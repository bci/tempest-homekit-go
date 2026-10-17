/**
 * @module run_update_alarm_sim
 * @description Manual debugging aid: loads pkg/web/static/script.js into a JSDOM page
 * whose URL carries ?tag=indoor, calls updateAlarmStatus with two sample alarms
 * (tags outdoor and indoor), and prints the resulting #alarm-list HTML, the tag
 * <select> and each rendered .alarm-item, so the tag-filter rendering can be
 * inspected without a browser or server.
 * Usage: node tools/run_update_alarm_sim.js (needs the jsdom dev dependency).
 * Errors from updateAlarmStatus are printed, not rethrown.
 */
const { JSDOM } = require('jsdom');
const path = require('path');
/** Absolute path of the dashboard script under test. @type {string} */
const scriptPath = path.resolve(__dirname, '..', 'pkg', 'web', 'static', 'script.js');

/** Minimal page with the elements updateAlarmStatus renders into. @type {JSDOM} */
const dom = new JSDOM(`<!doctype html><html><body>
  <div id="tempest-card"></div>
  <div id="alarm-list"><div class="alarm-list-header">Active Alarms</div></div>
</body></html>`, { url: 'http://localhost/?tag=indoor' });

global.window = dom.window;
global.document = dom.window.document;
global.localStorage = dom.window.localStorage;

// Drop any cached copy so script.js binds to the JSDOM globals set above.
delete require.cache[require.resolve(scriptPath)];
/** The script.js module exports (CommonJS when run under Node). */
const script = require(scriptPath);
/** Function under test, from the exports or the window global. @type {Function} */
const updateAlarmStatus = script.updateAlarmStatus || dom.window.updateAlarmStatus;

/** Sample /api/alarm-status style payload with one outdoor and one indoor alarm. */
const data = {
  enabled: true,
  totalAlarms: 2,
  enabledAlarms: 2,
  alarms: [
    { enabled: true, name: 'A', tags: ['outdoor'], condition: 'x', lastTriggered: '-', channels: ['email'], inCooldown: false, cooldownRemaining: 0, cooldown: 0, triggeredCount: 0 },
    { enabled: true, name: 'B', tags: ['indoor'], condition: 'y', lastTriggered: '-', channels: ['sms'], inCooldown: false, cooldownRemaining: 0, cooldown: 0, triggeredCount: 0 }
  ]
};

try {
  updateAlarmStatus(data);
  console.log('alarm-list innerHTML:\n', global.document.getElementById('alarm-list').innerHTML);
  const sel = global.document.querySelector('select.alarm-tag-select');
  console.log('select exists?', !!sel, sel && sel.outerHTML);
  const items = Array.from(global.document.querySelectorAll('.alarm-item')).map(el => el.outerHTML);
  console.log('alarm items:', items.length);
  items.forEach((it, i) => console.log(i, it));
} catch (e) {
  console.error('ERROR during updateAlarmStatus simulation:', e);
}
