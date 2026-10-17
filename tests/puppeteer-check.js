/**
 * @module puppeteer-check
 * @description tests/puppeteer-check.js
 * Usage: node tests/puppeteer-check.js --url=http://localhost:8086/chart/temperature --duration=10000
 * Requires: Node 14+ and puppeteer installed
 *
 * Smoke test against a running server: opens the page in headless Chrome, waits for
 * the chart canvas or dashboard, collects console messages, page errors and failed
 * requests for --duration ms, saves a full-page screenshot to
 * ./puppeteer-popout.png and writes the collected results as JSON to --out
 * (default puppeteer-console.json).
 *
 * Options: --url=, --duration= (ms), --out=, --headed (show the browser).
 * Env: PUPPETEER_EXECUTABLE_PATH overrides the Chrome binary.
 * Exit codes: 0 clean; 2 any page error, failed request, or console message of type
 * error/assert or whose text contains "error"; 3 the run itself failed.
 */

const fs = require('fs');
const path = require('path');
const puppeteer = require('puppeteer');

/**
 * Collected run options.
 * @typedef {Object} CheckOptions
 * @property {string} url - Page to load.
 * @property {number} duration - Milliseconds to keep collecting after load.
 * @property {boolean} headless - false when --headed is given.
 * @property {string} out - Path of the JSON results file.
 */

/**
 * Parses --url=, --duration=, --out= and --headed from process.argv over defaults.
 * Unknown arguments are ignored.
 * @returns {CheckOptions}
 */
function parseArgs() {
  const args = process.argv.slice(2);
  const opts = { url: 'http://localhost:8086/chart/temperature', duration: 10000, headless: true, out: 'puppeteer-console.json' };
  args.forEach(a => {
    if (a.startsWith('--url=')) opts.url = a.split('=')[1];
    if (a.startsWith('--duration=')) opts.duration = Number(a.split('=')[1]);
    if (a === '--headed') opts.headless = false;
    if (a.startsWith('--out=')) opts.out = a.split('=')[1];
  });
  return opts;
}

/**
 * Main: launches the browser, runs the check and exits with the status described in
 * the file header.
 */
(async () => {
  const opts = parseArgs();
  const results = { url: opts.url, start: new Date().toISOString(), console: [], pageErrors: [], requestsFailed: [] };

  // Launch Puppeteer
  const launchOptions = {
    headless: opts.headless,
    args: [
      '--no-sandbox',
      '--disable-setuid-sandbox',
      '--disable-features=IsolateOrigins,site-per-process'
    ],
  };
  if (process.env.PUPPETEER_EXECUTABLE_PATH) {
    launchOptions.executablePath = process.env.PUPPETEER_EXECUTABLE_PATH;
    console.log('Using PUPPETEER_EXECUTABLE_PATH:', launchOptions.executablePath);
  }
  const browser = await puppeteer.launch(launchOptions);

  try {
    const page = await browser.newPage();

    // Collect console messages
    page.on('console', msg => {
        let args = [];
        if (Array.isArray(msg.args)) {
          args = msg.args.map(a => {
            try { return a._remoteObject ? a._remoteObject.value : String(a); } catch (e) { return String(a); }
          });
        } else if (msg.args) {
          // Some puppeteer versions emit args as a non-array; try best-effort stringify
          try { args = [JSON.stringify(msg.args)]; } catch (e) { args = [String(msg.args)]; }
        }
      results.console.push({
        type: msg.type(),
        text: msg.text(),
        args,
        location: msg.location()
      });
      console.log(`[console:${msg.type()}] ${msg.text()}`);
    });

    // Page errors
    page.on('pageerror', err => {
      results.pageErrors.push({ message: err.message, stack: err.stack, time: new Date().toISOString() });
      console.error('[pageerror]', err && err.message);
    });

    // Failed requests
    page.on('requestfailed', req => {
      results.requestsFailed.push({
        url: req.url(),
        method: req.method(),
        failure: req.failure() && req.failure().errorText,
        time: new Date().toISOString()
      });
      console.warn('[requestfailed]', req.url(), (req.failure() && req.failure().errorText) || '');
    });

    await page.setViewport({ width: 1600, height: 900 });

    console.log('Navigating to', opts.url);
    const resp = await page.goto(opts.url, { waitUntil: 'domcontentloaded', timeout: 15000 });
    if (resp) {
      console.log('HTTP', resp.status(), resp.statusText());
      results.httpStatus = { status: resp.status(), statusText: resp.statusText() };
    }

    try {
      await page.waitForSelector('#chart-canvas, #temperature', { timeout: 5000 });
      console.log('Found chart or dashboard selector');
    } catch (e) {
      console.warn('chart-canvas or temperature element not found within 5s');
    }

    console.log(`Waiting ${opts.duration}ms to collect console logs...`);
    await new Promise(r => setTimeout(r, opts.duration));

    const screenshotPath = path.join(process.cwd(), 'puppeteer-popout.png');
    try {
      await page.screenshot({ path: screenshotPath, fullPage: true });
      console.log('Saved screenshot to', screenshotPath);
      results.screenshot = screenshotPath;
    } catch (e) {
      console.warn('Screenshot failed:', e && e.message);
    }

    results.end = new Date().toISOString();
    fs.writeFileSync(opts.out, JSON.stringify(results, null, 2));
    console.log('Wrote results to', opts.out);

    const hasPageErrors = results.pageErrors.length > 0;
    const hasConsoleErrors = results.console.some(c => c.type === 'error' || c.type === 'assert' || (c.text && c.text.toLowerCase().includes('error')));
    const hasFailedRequests = results.requestsFailed.length > 0;

    if (hasPageErrors || hasConsoleErrors || hasFailedRequests) {
      console.error('Detected issues:', { pageErrors: results.pageErrors.length, consoleErrors: results.console.filter(c => c.type === 'error').length, failedRequests: results.requestsFailed.length });
      await browser.close();
      process.exit(2);
    } else {
      console.log('No critical console/page errors detected.');
      await browser.close();
      process.exit(0);
    }
  } catch (err) {
    console.error('Puppeteer run failed:', err);
    try { await browser.close(); } catch(e) {}
    process.exit(3);
  }
})();
