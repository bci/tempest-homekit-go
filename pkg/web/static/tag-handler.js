/**
 * @module tag-handler
 * @description Helper for applying tag selection effects (URL + storage) in a
 * testable way. It is pure: it returns the new query string and a description of
 * the storage write instead of touching location or localStorage, so the caller
 * performs the side effects. Exported via CommonJS for Jest, or as a window global
 * in the browser.
 */

/**
 * Describes the side effects of choosing a tag in the alarm tag dropdown.
 * @param {string} selectedValue - Chosen tag; '' (or any falsy value) means "all tags".
 * @param {string} [currentSearch=''] - Current query string, with or without '?'.
 * @param {string} [storageKey='alarm-selected-tag'] - localStorage key to update.
 * @returns {{newSearch: string, storage: {action: ('set'|'remove'), key: string, value: (string|undefined)}}}
 *     newSearch is the query string (no leading '?') with `tag` set or removed and
 *     other parameters preserved; storage says whether to set or remove the key.
 */
function applyTagSelection(selectedValue, currentSearch = '', storageKey = 'alarm-selected-tag') {
  const params = new URLSearchParams(currentSearch || '');
  const result = { newSearch: null, storage: null };

  if (!selectedValue) {
    // clear tag
    params.delete('tag');
    result.newSearch = params.toString();
    result.storage = { action: 'remove', key: storageKey };
  } else {
    params.set('tag', selectedValue);
    result.newSearch = params.toString();
    result.storage = { action: 'set', key: storageKey, value: selectedValue };
  }

  return result;
}

// Export for Jest (CommonJS) or as a browser global.
if (typeof module !== 'undefined' && module.exports) {
  module.exports = { applyTagSelection };
} else {
  window.applyTagSelection = applyTagSelection;
}
