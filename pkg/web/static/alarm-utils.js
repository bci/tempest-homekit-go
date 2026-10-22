/**
 * @module alarm-utils
 * @description Small utility functions for alarm UI logic (pure functions - safe to
 * test). Loaded as a plain <script> by the dashboard (server.go) and chart.html,
 * where the functions become window globals, and required as CommonJS by the Jest
 * tests in __tests__/.
 */

/**
 * Picks the CSS class for an alarm's "triggered N times" badge.
 * @param {(number|string|null|undefined)} count - Trigger count; coerced with Number().
 * @returns {string} 'alarm-triggered-ok' for 0, negative or non-numeric input,
 *     'alarm-triggered-warn' for 1-5, 'alarm-triggered-critical' above 5.
 */
function getTriggeredBadgeClass(count) {
    const n = Number(count || 0);
    if (!Number.isFinite(n) || n <= 0) return 'alarm-triggered-ok';
    if (n > 5) return 'alarm-triggered-critical';
    return 'alarm-triggered-warn';
}

/**
 * Saves the selected alarm tag to localStorage, or removes the key when the value is
 * empty. Storage errors (private mode, quota, disabled storage) are swallowed so a
 * failed save never breaks the UI.
 * @param {string} key - localStorage key (the app uses 'alarm-selected-tag').
 * @param {?string} val - Tag to persist; falsy clears it.
 * @returns {void}
 */
function persistSelectedTag(key, val) {
    try {
        if (!val) {
            localStorage.removeItem(key);
        } else {
            localStorage.setItem(key, val);
        }
    } catch (e) {
        // ignore storage errors
    }
}

/**
 * Reads a persisted tag from localStorage.
 * @param {string} key - localStorage key.
 * @returns {?string} The stored tag, or null if absent or storage is unavailable.
 */
function restorePersistedTag(key) {
    try {
        return localStorage.getItem(key);
    } catch (e) {
        return null;
    }
}

/**
 * Decide which tag should be selected based on URL and persisted value.
 * Returns an object { selectedTag, newSearch } where newSearch is null
 * when no URL update is required, or a string (without leading '?') when
 * the URL should be updated to include the selected tag.
 *
 * Precedence: a `tag` URL parameter always wins (it is not checked against
 * tagList); otherwise a persisted tag is used only if it is still in tagList, so a
 * tag that no longer exists is silently dropped.
 *
 * urlSearch: string (window.location.search)
 * persistedTag: string|null
 * tagList: array of available tag strings
 *
 * @param {string} urlSearch - Query string, with or without leading '?'.
 * @param {?string} persistedTag - Value from restorePersistedTag.
 * @param {string[]} tagList - Tags currently available; non-arrays are treated as empty.
 * @returns {{selectedTag: string, newSearch: ?string}} selectedTag is '' when no tag applies.
 */
function computeSelectedTag(urlSearch, persistedTag, tagList) {
    const params = new URLSearchParams(urlSearch || '');
    const urlTag = params.get('tag');
    let selectedTag = urlTag || '';
    let newSearch = null;

    const tagSet = new Set(Array.isArray(tagList) ? tagList : []);

    if (urlTag) {
        // URL takes precedence, nothing to change
        selectedTag = urlTag;
        newSearch = null;
    } else if (!urlTag && persistedTag && tagSet.has(persistedTag)) {
        // No URL tag but persisted tag exists and is valid -> update URL
        selectedTag = persistedTag;
        params.set('tag', selectedTag);
        newSearch = params.toString();
    } else {
        selectedTag = '';
        newSearch = null;
    }

    return { selectedTag, newSearch };
}


// Export for test env (CommonJS & browser global): under Node/Jest `module` exists,
// in the browser the functions are attached to window instead.
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { getTriggeredBadgeClass, persistSelectedTag, restorePersistedTag, computeSelectedTag };
} else {
    window.getTriggeredBadgeClass = getTriggeredBadgeClass;
    window.persistSelectedTag = persistSelectedTag;
    window.restorePersistedTag = restorePersistedTag;
    window.computeSelectedTag = computeSelectedTag;
}
