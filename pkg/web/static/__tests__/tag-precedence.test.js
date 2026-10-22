/**
 * @module tag-precedence.test
 * @description Unit tests for the tag persistence helpers and computeSelectedTag in
 * ../alarm-utils.js. Relies on the global localStorage that the jsdom test
 * environment provides (jest.web.config.json).
 */
const { computeSelectedTag, persistSelectedTag, restorePersistedTag } = require('../alarm-utils');

/**
 * URL vs persisted tag precedence, and validation against the tag list.
 */
describe('Tag precedence and persistence', () => {
  /**
   * Starts each test with empty localStorage.
   */
  beforeEach(() => {
    // jsdom provides a simple localStorage
    localStorage.clear();
  });

  /**
   * A URL tag beats a persisted one and needs no URL update.
   */
  test('URL param overrides persisted tag', () => {
    // Persist a tag 'outdoor'
    persistSelectedTag('alarm-selected-tag', 'outdoor');
    // Provide a URL search with ?tag=indoor
    const urlSearch = '?tag=indoor';
    const tagList = ['indoor','outdoor','garage'];

    const result = computeSelectedTag(urlSearch, restorePersistedTag('alarm-selected-tag'), tagList);
    expect(result.selectedTag).toBe('indoor');
    expect(result.newSearch).toBeNull();
  });

  /**
   * A valid persisted tag is used and written into the returned query string.
   */
  test('Persisted tag applied when URL param absent and tag valid', () => {
    // Persist a tag 'garage'
    persistSelectedTag('alarm-selected-tag', 'garage');
    const urlSearch = '';
    const tagList = ['indoor','outdoor','garage'];

    const result = computeSelectedTag(urlSearch, restorePersistedTag('alarm-selected-tag'), tagList);
    expect(result.selectedTag).toBe('garage');
    // newSearch should include tag=garage
    expect(result.newSearch).toMatch(/tag=garage/);
  });

  /**
   * A persisted tag missing from the tag list yields no selection and no URL change.
   */
  test('Invalid persisted tag is ignored', () => {
    persistSelectedTag('alarm-selected-tag', 'invalid-tag');
    const urlSearch = '';
    const tagList = ['indoor','outdoor','garage'];

    const result = computeSelectedTag(urlSearch, restorePersistedTag('alarm-selected-tag'), tagList);
    expect(result.selectedTag).toBe('');
    expect(result.newSearch).toBeNull();
  });
});
