/**
 * @module alarm-compact-persistence.test
 * @description Integration test for the dashboard alarm list's compact mode: loads the
 * real ../script.js into a fresh JSDOM page with a mocked localStorage and checks that
 * an alarm expanded by the user stays expanded across re-renders, via the
 * 'alarm-expanded-set' localStorage key.
 */
const fs = require('fs');
const path = require('path');

// Load the browser-like environment
const { JSDOM } = require('jsdom');

// Load the script module (it exports updateAlarmStatus for tests)
/** Absolute path of the dashboard script under test. @type {string} */
const scriptPath = path.resolve(__dirname, '..', 'script.js');
/** updateAlarmStatus from the freshly required script; reset after each test. @type {?Function} */
let updateAlarmStatus;

/**
 * Compact-mode expand state survives re-rendering.
 */
describe('Alarm compact persistence', () => {
    let dom;

    /**
     * Builds a new JSDOM page and mock localStorage, exposes them as globals, and
     * re-requires script.js so it binds to them.
     */
    beforeEach(() => {
        // Create a fresh DOM for each test
        dom = new JSDOM(`<!doctype html><html><body>
            <div id="tempest-card"></div>
            <div id="alarm-card">
                <div id="alarm-list"><div class="alarm-list-header">Active Alarms</div></div>
            </div>
        </body></html>`, { runScripts: 'dangerously', resources: 'usable' });

        // Mock localStorage
        const mockLocalStorage = {
            data: {},
            getItem(key) { return this.data[key] || null; },
            setItem(key, value) { this.data[key] = value; },
            removeItem(key) { delete this.data[key]; },
            clear() { this.data = {}; }
        };
        dom.window.localStorage = mockLocalStorage;
        global.localStorage = mockLocalStorage;

        // Provide global window and document for script
        global.window = dom.window;
        global.document = dom.window.document;

    // Require the script module (it will use the JSDOM globals we set)
    jest.resetModules();
    const script = require(scriptPath);
    updateAlarmStatus = script.updateAlarmStatus || dom.window.updateAlarmStatus;

    // Ensure localStorage is clean
    global.localStorage.clear();
    });

    /**
     * Removes the globals installed by beforeEach.
     */
    afterEach(() => {
        // Cleanup globals
        delete global.window;
        delete global.document;
        delete global.localStorage;
        updateAlarmStatus = null;
    });

    /**
     * Clicking an item's expand button in compact mode adds 'expanded', records the
     * alarm name in 'alarm-expanded-set', and a second render keeps it expanded.
     */
    test('persists expanded state across render', () => {
        const sample = {
            enabled: true,
            configPath: '/tmp/alarm.conf',
            lastReadTime: 'now',
            totalAlarms: 1,
            enabledAlarms: 1,
            alarms: [
                {
                    name: 'Test Alarm',
                    enabled: true,
                    condition: 'temp > 30',
                    lastTriggered: 'a moment ago',
                    channels: ['email'],
                    tags: ['outdoor'],
                    inCooldown: false,
                    cooldown: 60,
                    cooldownRemaining: 0,
                    triggeredCount: 0
                }
            ]
        };

    // First render with compact mode enabled in storage
        global.localStorage.setItem('alarm-compact-mode', 'true');
    updateAlarmStatus(sample, { window: dom.window, document: dom.window.document });

        const list = dom.window.document.getElementById('alarm-list');
        const item = list.querySelector('.alarm-item');
        expect(item).toBeTruthy();
        expect(item.classList.contains('compact')).toBe(true);

        // Simulate user clicking to expand
        const button = item.querySelector('.alarm-expand-button');
        button.click();
        expect(item.classList.contains('expanded')).toBe(true);

        // Check localStorage set updated
        const raw = global.localStorage.getItem('alarm-expanded-set');
        expect(raw).toBeTruthy();
        const arr = JSON.parse(raw);
        expect(Array.isArray(arr)).toBe(true);
        expect(arr).toContain('Test Alarm');

        // Re-render and ensure the item remains expanded
    updateAlarmStatus(sample, { window: dom.window, document: dom.window.document });
        const item2 = dom.window.document.getElementById('alarm-list').querySelector('.alarm-item');
        expect(item2.classList.contains('expanded')).toBe(true);
    });
});
