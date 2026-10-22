/* eslint-env jest */
/**
 * @module tag-integration.test
 * @description Exercises computeSelectedTag from ../alarm-utils.js against a real JSDOM
 * location, localStorage and <select>, mimicking how the dashboard would apply it.
 * The dashboard script itself is not loaded here.
 */
const { JSDOM } = require('jsdom');
const path = require('path');
/** The alarm-utils module under test. */
const alarmUtils = require(path.resolve(__dirname, '../alarm-utils.js'));

/**
 * Tag precedence between URL and localStorage, applied to a DOM select.
 */
describe('alarm tag select integration using alarm-utils', () => {
  /**
   * ?tag=indoor wins over a persisted 'outdoor'; no URL rewrite is requested.
   */
  test('URL tag takes precedence over persisted tag', () => {
    const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/?tag=indoor' });
    const { window } = dom;

    // Persisted tag in localStorage
    window.localStorage.setItem('alarm-selected-tag', 'outdoor');

    const availableTags = ['indoor', 'outdoor'];
    const { selectedTag, newSearch } = alarmUtils.computeSelectedTag(window.location.search, window.localStorage.getItem('alarm-selected-tag'), availableTags);

    // URL wins, so selectedTag should be 'indoor' and no URL update required
    expect(selectedTag).toBe('indoor');
    expect(newSearch).toBeNull();

    // Build a DOM select as the real UI would and set the selected value
    const select = window.document.createElement('select');
    select.className = 'alarm-tag-select';
    availableTags.forEach(t => {
      const opt = window.document.createElement('option');
      opt.value = t;
      opt.textContent = t;
      select.appendChild(opt);
    });
    if (selectedTag) select.value = selectedTag;

    expect(select.value).toBe('indoor');
  });

  /**
   * With no tag param, a valid persisted tag is selected and a new query string
   * containing it is returned and can be pushed to history.
   */
  test('Persisted tag applied and URL updated when no tag param', () => {
    const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/' });
    const { window } = dom;

    // Persisted tag in localStorage
    window.localStorage.setItem('alarm-selected-tag', 'garage');

    const availableTags = ['garage', 'other'];
    const { selectedTag, newSearch } = alarmUtils.computeSelectedTag(window.location.search, window.localStorage.getItem('alarm-selected-tag'), availableTags);

    // Since URL has no tag but persisted tag exists and is valid, selectedTag should be persisted and newSearch should be provided
    expect(selectedTag).toBe('garage');
    expect(typeof newSearch).toBe('string');
    expect(newSearch).toMatch(/tag=garage/);

    // Simulate updating the browser URL (history.pushState) to reflect newSearch
    window.history.pushState({}, '', `${window.location.pathname}?${newSearch}`);
    expect(window.location.search).toContain('tag=garage');

    // Build select element and ensure value matches persisted selection
    const select = window.document.createElement('select');
    select.className = 'alarm-tag-select';
    availableTags.forEach(t => {
      const opt = window.document.createElement('option');
      opt.value = t;
      opt.textContent = t;
      select.appendChild(opt);
    });
    if (selectedTag) select.value = selectedTag;

    expect(select.value).toBe('garage');
  });
});

