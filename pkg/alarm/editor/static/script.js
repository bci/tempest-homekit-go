/**
 * @module alarm-editor
 * @description Browser-side logic for the standalone alarm editor page served by
 * pkg/alarm/editor (the HTML lives in html.go and loads this file from
 * /alarm-editor/static/script.js at the end of <body>).
 *
 * The page lists alarms from GET /api/config, lets the user create/edit/delete them
 * through a modal form, and talks to the editor server's JSON endpoints
 * (/api/tags, /api/contacts, /api/env-defaults, /api/validate, /api/validate-json,
 * /api/alarms/create|update|delete, /api/contacts/save, /api/tags/save).
 * Most functions are invoked from inline onclick/onchange attributes in html.go, so
 * they must stay global (no module system, no bundler).
 *
 * State is kept in the module-level `let` variables below; the modal form is the
 * source of truth while it is open and is serialized back to JSON in handleSubmit.
 */

/**
 * A notification channel on an alarm, as stored in the alarm config JSON.
 * Exactly one of the type-specific sub-objects is present, matching `type`.
 * @typedef {Object} AlarmChannel
 * @property {string} type - One of console, syslog, oslog, eventlog, email, sms, webhook, csv, json.
 * @property {string} [template] - Message template for console/syslog/oslog/eventlog.
 * @property {{to: string[], subject: string, body: string, html: boolean}} [email]
 * @property {{to: string[], message: string}} [sms]
 * @property {{url: string, method: string, headers: Object<string,string>, body: string, content_type: string}} [webhook]
 * @property {{path: string, max_days: number, message: string}} [csv]
 * @property {{path: string, max_days: number, message: string}} [json]
 */

/**
 * Optional activation window for an alarm. Absent/null means "always active".
 * @typedef {Object} AlarmSchedule
 * @property {string} type - 'time', 'daily', 'weekly' or 'sun'.
 * @property {string} [timezone] - IANA zone name; omitted when blank.
 * @property {string} [start_time] - HH:MM start (time/daily, optional for weekly).
 * @property {string} [end_time] - HH:MM end (time/daily, optional for weekly).
 * @property {number[]} [days_of_week] - Weekly only; 0 = Sunday ... 6 = Saturday.
 * @property {string} [sun_event] - Sun only; e.g. 'sunrise' or 'sunset'.
 * @property {number} [sun_offset] - Sun only; offset applied to sun_event.
 * @property {string} [sun_event_end] - Sun only; optional end event.
 * @property {number} [sun_offset_end] - Sun only; offset applied to sun_event_end.
 * @property {boolean} [use_station_location] - Sun only; use the station's coordinates.
 * @property {number} [latitude] - Sun only; custom location latitude.
 * @property {number} [longitude] - Sun only; custom location longitude.
 */

/**
 * One alarm as returned by /api/config and posted to /api/alarms/create|update.
 * @typedef {Object} Alarm
 * @property {string} name - Unique name; also the key used by edit/delete.
 * @property {string} [description]
 * @property {string} condition - Alarm condition expression (validated by /api/validate).
 * @property {string[]} tags
 * @property {number} cooldown - Seconds between repeat notifications.
 * @property {boolean} enabled
 * @property {AlarmChannel[]} channels
 * @property {AlarmSchedule} [schedule]
 */

/**
 * An address-book entry from /api/contacts.
 * @typedef {Object} Contact
 * @property {string} name
 * @property {string} [email]
 * @property {string} [sms] - Phone number; the server requires a leading '+'.
 */

/**
 * All alarms loaded from the server; re-fetched after every save/delete.
 * @type {Alarm[]}
 */
let alarms = [];
/**
 * The alarm open in the edit modal, or null when the modal is creating a new alarm.
 * handleSubmit uses it to choose between the create and update endpoints.
 * @type {?Alarm}
 */
let currentAlarm = null;
/**
 * Every known tag (from /api/tags plus any added in this session), kept sorted when
 * addTag appends a new one.
 * @type {string[]}
 */
let allTags = [];
/**
 * Tags chosen for the alarm in the edit modal.
 * @type {string[]}
 */
let selectedTags = [];
/**
 * Address book loaded from /api/contacts, used to populate the contact dropdowns.
 * @type {Contact[]}
 */
let contacts = [];
/**
 * Email recipients chosen for the alarm in the edit modal.
 * @type {string[]}
 */
let selectedEmailContacts = [];
/**
 * SMS recipients (phone numbers) chosen for the alarm in the edit modal.
 * @type {string[]}
 */
let selectedSMSContacts = [];

// ============================================
// Theme Switching System
// ============================================

/**
 * Load saved theme on page load: applies the theme stored under the
 * 'selected-theme' localStorage key (default 'default'), syncs the #theme-select
 * dropdown to it, and persists any later change the user makes there.
 */
document.addEventListener('DOMContentLoaded', function() {
    const savedTheme = localStorage.getItem('selected-theme') || 'default';
    applyTheme(savedTheme);
    
    const themeSelect = document.getElementById('theme-select');
    if (themeSelect) {
        themeSelect.value = savedTheme;
        
        // Listen for theme changes
        themeSelect.addEventListener('change', function() {
            const newTheme = this.value;
            applyTheme(newTheme);
            localStorage.setItem('selected-theme', newTheme);
        });
    }
});

/**
 * Apply theme to document by setting `data-theme` on <body>. The 'default' theme
 * is represented by the attribute being absent, so themes.css only needs rules for
 * the named themes.
 * @param {string} themeName - Theme id from #theme-select, or 'default'.
 * @returns {void}
 */
function applyTheme(themeName) {
    const body = document.body;
    
    // Remove existing theme
    body.removeAttribute('data-theme');
    
    // Apply new theme (except for default)
    if (themeName !== 'default') {
        body.setAttribute('data-theme', themeName);
    }
}

/**
 * Page bootstrap: loads alarms, tags and contacts (sequentially, because
 * updateTagFilter and the contact dropdowns read the loaded data), wires the search,
 * tag-filter and form-submit listeners, and initialises the tag/contact pickers.
 * Called once at the bottom of this file.
 * @returns {Promise<void>}
 */
async function init() {
    await loadAlarms();
    await loadTags();
    await loadContacts();
    document.getElementById('searchName').addEventListener('input', filterAlarms);
    document.getElementById('filterTag').addEventListener('change', filterAlarms);
    document.getElementById('alarmForm').addEventListener('submit', handleSubmit);
    initContactSelectors();
    initTagSelectors();
    
    // Update last update timestamp
    updateLastUpdateTimestamp();
}

/**
 * Fetches the full config from /api/config and re-renders the alarm list. A
 * timestamp query parameter defeats browser caching so a reload right after a save
 * sees the new data.
 * @returns {Promise<void>}
 * @throws {Error} If the fetch or JSON parse fails (not caught here).
 */
async function loadAlarms() {
    const response = await fetch('/api/config?_=' + Date.now());
    const config = await response.json();
    alarms = config.alarms || [];
    renderAlarms();
}

/**
 * Fetches the tag list from /api/tags into allTags and rebuilds the tag filter.
 * @returns {Promise<void>}
 * @throws {Error} If the fetch or JSON parse fails (not caught here).
 */
async function loadTags() {
    const response = await fetch('/api/tags');
    allTags = await response.json();
    updateTagFilter();
}

/**
 * Fetches the address book from /api/contacts into `contacts`. Failure is logged and
 * leaves an empty list, because contacts are optional for using the editor.
 * @returns {Promise<void>}
 */
async function loadContacts() {
    try {
        const response = await fetch('/api/contacts');
        contacts = await response.json();
    } catch (error) {
        console.warn('Failed to load contacts:', error);
        contacts = [];
    }
}

/**
 * onchange handler for the hidden legacy #emailContactSelect: adds the chosen
 * contact's email (the option value is an index into `contacts`) and resets the
 * select. Superseded by the searchable dropdown in initContactSelectors but still
 * wired in html.go.
 * @returns {void}
 */
function addContactEmail() {
    const select = document.getElementById('emailContactSelect');
    const contactIndex = select.value;
    
    if (!contactIndex) return;
    
    const contact = contacts[parseInt(contactIndex)];
    if (!contact || !contact.email) return;
    
    addContact('email', contact.email);
    
    // Reset dropdown
    select.value = '';
}

/**
 * onchange handler for the hidden legacy #smsContactSelect; SMS counterpart of
 * addContactEmail.
 * @returns {void}
 */
function addContactSMS() {
    const select = document.getElementById('smsContactSelect');
    const contactIndex = select.value;
    
    if (!contactIndex) return;
    
    const contact = contacts[parseInt(contactIndex)];
    if (!contact || !contact.sms) return;
    
    addContact('sms', contact.sms);
    
    // Reset dropdown
    select.value = '';
}

/**
 * Rebuilds the #filterTag dropdown from allTags, preserving the current selection.
 * @returns {void}
 */
function updateTagFilter() {
    const select = document.getElementById('filterTag');
    const currentValue = select.value;
    select.innerHTML = '<option value="">All Tags</option>';
    allTags.forEach(tag => {
        const option = document.createElement('option');
        option.value = tag;
        option.textContent = tag;
        if (tag === currentValue) option.selected = true;
        select.appendChild(option);
    });
}

/**
 * Wires the searchable email and SMS recipient pickers: focus opens the dropdown,
 * typing filters it, Enter adds the typed text as a free-form recipient, and a click
 * outside either picker closes both dropdowns.
 * @returns {void}
 */
function initContactSelectors() {
    // Email contact selector
    const emailSearchInput = document.getElementById('emailContactSearch');
    const emailDropdown = document.getElementById('emailContactDropdown');
    
    emailSearchInput.addEventListener('focus', () => {
        updateContactDropdown('email', '');
        emailDropdown.classList.add('active');
    });
    
    emailSearchInput.addEventListener('input', (e) => {
        updateContactDropdown('email', e.target.value);
    });
    
    emailSearchInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            const value = emailSearchInput.value.trim();
            if (value) {
                addContact('email', value);
                emailSearchInput.value = '';
                updateContactDropdown('email', '');
            }
        }
    });
    
    // SMS contact selector
    const smsSearchInput = document.getElementById('smsContactSearch');
    const smsDropdown = document.getElementById('smsContactDropdown');
    
    smsSearchInput.addEventListener('focus', () => {
        updateContactDropdown('sms', '');
        smsDropdown.classList.add('active');
    });
    
    smsSearchInput.addEventListener('input', (e) => {
        updateContactDropdown('sms', e.target.value);
    });
    
    smsSearchInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            const value = smsSearchInput.value.trim();
            if (value) {
                addContact('sms', value);
                smsSearchInput.value = '';
                updateContactDropdown('sms', '');
            }
        }
    });
    
    // Close dropdowns when clicking outside
    document.addEventListener('click', (e) => {
        if (!e.target.closest('.contact-dropdown-wrapper') && !e.target.closest('.contact-dropdown')) {
            emailDropdown.classList.remove('active');
            smsDropdown.classList.remove('active');
        }
    });
}

/**
 * Wires the searchable tag picker the same way initContactSelectors wires the
 * recipient pickers (focus, filter-as-you-type, Enter to add, click-outside to close).
 * @returns {void}
 */
function initTagSelectors() {
    // Tag selector
    const tagSearchInput = document.getElementById('tagSearchInput');
    const tagDropdown = document.getElementById('tagDropdown');
    
    tagSearchInput.addEventListener('focus', () => {
        updateTagDropdown('');
        tagDropdown.classList.add('active');
    });
    
    tagSearchInput.addEventListener('input', (e) => {
        updateTagDropdown(e.target.value);
    });
    
    tagSearchInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            const value = tagSearchInput.value.trim();
            if (value) {
                addTag(value);
                tagSearchInput.value = '';
                updateTagDropdown('');
            }
        }
    });
    
    // Close dropdown when clicking outside
    document.addEventListener('click', (e) => {
        if (!e.target.closest('.tag-dropdown-wrapper') && !e.target.closest('.tag-dropdown')) {
            tagDropdown.classList.remove('active');
        }
    });
}

/**
 * Rebuilds a recipient dropdown: lists address-book contacts of that type that are
 * not already selected and whose name or address contains the search term, plus a
 * "+ Add new" entry when the term is not an existing contact or selection.
 * @param {('email'|'sms')} type - Which picker to rebuild.
 * @param {string} searchTerm - Current text in the picker's search box.
 * @returns {void}
 */
function updateContactDropdown(type, searchTerm) {
    const dropdown = document.getElementById(type + 'ContactDropdown');
    const searchLower = searchTerm.toLowerCase();
    
    // Get already selected contacts for this type
    const selectedContacts = type === 'email' ? selectedEmailContacts : selectedSMSContacts;
    
    // Filter available contacts (not already selected)
    const availableContacts = contacts.filter(contact => {
        const contactValue = type === 'email' ? contact.email : contact.sms;
        return contactValue && !selectedContacts.includes(contactValue) && 
               (contact.name.toLowerCase().includes(searchLower) || contactValue.toLowerCase().includes(searchLower));
    });
    
    dropdown.innerHTML = '';
    
    if (availableContacts.length === 0 && !searchTerm) {
        dropdown.innerHTML = '<div class="contact-dropdown-empty">No contacts available</div>';
        return;
    }
    
    // Show matching existing contacts
    availableContacts.forEach((contact, index) => {
        const contactValue = type === 'email' ? contact.email : contact.sms;
        const item = document.createElement('div');
        item.className = 'contact-dropdown-item';
        item.textContent = `${contact.name} (${contactValue})`;
        item.addEventListener('click', () => {
            addContact(type, contactValue);
            document.getElementById(type + 'ContactSearch').value = '';
            updateContactDropdown(type, '');
        });
        dropdown.appendChild(item);
    });
    
    // Show "add new contact" option if searching and not already a contact
    if (searchTerm && !contacts.some(c => {
        const contactValue = type === 'email' ? c.email : c.sms;
        return contactValue === searchTerm;
    }) && !selectedContacts.includes(searchTerm)) {
        const newContactItem = document.createElement('div');
        newContactItem.className = 'contact-dropdown-item new-contact';
        newContactItem.textContent = `+ Add new ${type}: "${searchTerm}"`;
        newContactItem.addEventListener('click', () => {
            addContact(type, searchTerm);
            document.getElementById(type + 'ContactSearch').value = '';
            updateContactDropdown(type, '');
        });
        dropdown.appendChild(newContactItem);
    }
    
    if (dropdown.children.length === 0 && searchTerm) {
        dropdown.innerHTML = '<div class="contact-dropdown-empty">No matching contacts</div>';
    }
}

/**
 * Adds a recipient (trimmed, de-duplicated) to the selected list for `type` and
 * re-renders the chips.
 * @param {('email'|'sms')} type
 * @param {string} contactValue - Email address or phone number.
 * @returns {void}
 */
function addContact(type, contactValue) {
    const trimmedContact = contactValue.trim();
    if (!trimmedContact) return;
    
    const selectedContacts = type === 'email' ? selectedEmailContacts : selectedSMSContacts;
    
    if (!selectedContacts.includes(trimmedContact)) {
        selectedContacts.push(trimmedContact);
        renderSelectedContacts(type);
    }
}

/**
 * Removes a recipient chip; called from the chip's inline onclick. Refreshes the
 * dropdown so the removed contact becomes selectable again.
 * @param {('email'|'sms')} type
 * @param {string} contact - The exact recipient string to remove.
 * @returns {void}
 */
function removeContact(type, contact) {
    const selectedContacts = type === 'email' ? selectedEmailContacts : selectedSMSContacts;
    const index = selectedContacts.indexOf(contact);
    if (index > -1) {
        selectedContacts.splice(index, 1);
        renderSelectedContacts(type);
        updateContactDropdown(type, '');
    }
}

/**
 * Renders the selected recipients for `type` as removable chips into
 * #selectedEmailContacts or #selectedSMSContacts. Values are inserted as raw HTML;
 * only single quotes are escaped (for the inline onclick).
 * @param {('email'|'sms')} type
 * @returns {void}
 */
function renderSelectedContacts(type) {
    const container = document.getElementById('selected' + (type === 'email' ? 'Email' : 'SMS') + 'Contacts');
    const selectedContacts = type === 'email' ? selectedEmailContacts : selectedSMSContacts;
    
    if (selectedContacts.length === 0) {
        container.innerHTML = '';
        return;
    }
    
    container.innerHTML = selectedContacts.map(contact => 
        '<div class="selected-contact">' +
            '<span>' + contact + '</span>' +
            '<span class="remove-contact" onclick="removeContact(\'' + type + '\', \'' + contact.replace(/'/g, "\\'") + '\')">×</span>' +
        '</div>'
    ).join('');
}

/**
 * Rebuilds the tag dropdown: unselected tags containing the search term (case
 * insensitive) plus a "+ Add new tag" entry when the term is new.
 * @param {string} [searchTerm=''] - Current text in #tagSearchInput.
 * @returns {void}
 */
function updateTagDropdown(searchTerm = '') {
    const dropdown = document.getElementById('tagDropdown');
    const searchLower = searchTerm.toLowerCase();
    
    // Filter available tags (not already selected)
    const availableTags = allTags.filter(tag => 
        !selectedTags.includes(tag) && 
        tag.toLowerCase().includes(searchLower)
    );
    
    dropdown.innerHTML = '';
    
    if (availableTags.length === 0 && !searchTerm) {
        dropdown.innerHTML = '<div class="tag-dropdown-empty">No tags available</div>';
        return;
    }
    
    // Show matching existing tags
    availableTags.forEach(tag => {
        const item = document.createElement('div');
        item.className = 'tag-dropdown-item';
        item.textContent = tag;
        item.addEventListener('click', () => {
            addTag(tag);
            document.getElementById('tagSearchInput').value = '';
            updateTagDropdown();
        });
        dropdown.appendChild(item);
    });
    
    // Show "add new tag" option if searching
    if (searchTerm && !allTags.includes(searchTerm) && !selectedTags.includes(searchTerm)) {
        const newTagItem = document.createElement('div');
        newTagItem.className = 'tag-dropdown-item new-tag';
        newTagItem.textContent = '+ Add new tag: "' + searchTerm + '"';
        newTagItem.addEventListener('click', () => {
            addTag(searchTerm);
            document.getElementById('tagSearchInput').value = '';
            updateTagDropdown();
        });
        dropdown.appendChild(newTagItem);
    }
    
    if (dropdown.children.length === 0 && searchTerm) {
        dropdown.innerHTML = '<div class="tag-dropdown-empty">No matching tags</div>';
    }
}

/**
 * Selects a tag for the alarm being edited. A tag not yet in allTags is added there
 * too (kept sorted) and the filter dropdown is rebuilt, so new tags appear at once.
 * @param {string} tag - Tag text; trimmed, ignored if empty or already selected.
 * @returns {void}
 */
function addTag(tag) {
    const trimmedTag = tag.trim();
    if (!trimmedTag || selectedTags.includes(trimmedTag)) return;
    
    selectedTags.push(trimmedTag);
    
    // Add to allTags if it's new
    if (!allTags.includes(trimmedTag)) {
        allTags.push(trimmedTag);
        allTags.sort();
        updateTagFilter();
    }
    
    renderSelectedTags();
}

/**
 * Deselects a tag; called from the tag chip's inline onclick.
 * @param {string} tag
 * @returns {void}
 */
function removeTag(tag) {
    selectedTags = selectedTags.filter(t => t !== tag);
    renderSelectedTags();
    updateTagDropdown();
}

/**
 * Renders selectedTags as removable chips into #selectedTags (raw HTML; only single
 * quotes are escaped for the inline onclick).
 * @returns {void}
 */
function renderSelectedTags() {
    const container = document.getElementById('selectedTags');
    
    if (selectedTags.length === 0) {
        container.innerHTML = '';
        return;
    }
    
    container.innerHTML = selectedTags.map(tag => 
        '<div class="selected-tag">' +
            '<span>' + tag + '</span>' +
            '<span class="remove-tag" onclick="removeTag(\'' + tag.replace(/'/g, "\\'") + '\')">×</span>' +
        '</div>'
    ).join('');
}

/**
 * Listener for the name search box and tag filter; simply re-renders the list.
 * @returns {void}
 */
function filterAlarms() {
    renderAlarms();
}

/**
 * Renders the alarm cards into #alarmList, applying the name (case-insensitive
 * substring) and tag filters, and toggles #emptyState when nothing matches. Card
 * buttons call editAlarm/showAlarmJSON/deleteAlarm by alarm name via inline onclick.
 * Alarm fields are concatenated into HTML without escaping.
 * @returns {void}
 */
function renderAlarms() {
    const nameFilter = document.getElementById('searchName').value.toLowerCase();
    const tagFilter = document.getElementById('filterTag').value;
    
    const filtered = alarms.filter(alarm => {
        if (nameFilter && !alarm.name.toLowerCase().includes(nameFilter)) return false;
        if (tagFilter && !alarm.tags.includes(tagFilter)) return false;
        return true;
    });
    
    const container = document.getElementById('alarmList');
    const emptyState = document.getElementById('emptyState');
    
    if (filtered.length === 0) {
        container.innerHTML = '';
        emptyState.style.display = 'block';
        return;
    }
    
    emptyState.style.display = 'none';
    container.innerHTML = filtered.map(alarm => {
        const enabledClass = alarm.enabled ? '' : 'disabled';
        const statusClass = alarm.enabled ? 'status-enabled' : 'status-disabled';
        const description = alarm.description ? '<div class="alarm-description">' + alarm.description + '</div>' : '';
        const tags = alarm.tags && alarm.tags.length ? '<div class="alarm-tags">' + alarm.tags.map(tag => '<span class="tag">' + tag + '</span>').join('') + '</div>' : '';
        const channels = alarm.channels ? alarm.channels.map(ch => ch.type).join(', ') : 'No channels';
        
        return '<div class="alarm-card ' + enabledClass + '">' +
            '<div class="alarm-header">' +
                '<div>' +
                    '<div class="alarm-name">' +
                        '<span class="status-indicator ' + statusClass + '"></span>' +
                        alarm.name +
                    '</div>' +
                    description +
                '</div>' +
            '</div>' +
            '<div class="alarm-condition">' + alarm.condition + '</div>' +
            tags +
            '<div class="alarm-channels">📢 ' + channels + '</div>' +
            '<div class="alarm-actions">' +
                '<button class="btn btn-primary" onclick="editAlarm(\'' + alarm.name + '\')">Edit</button>' +
                '<button class="btn btn-info btn-sm" onclick="showAlarmJSON(\'' + alarm.name + '\')">📄 JSON</button>' +
                '<button class="btn btn-danger" onclick="deleteAlarm(\'' + alarm.name + '\')">Delete</button>' +
            '</div>' +
        '</div>';
    }).join('');
}

/**
 * Inserts a sensor field name into the condition textarea at the cursor, adding a
 * leading space unless the cursor follows a space or '(' so expressions stay
 * tokenisable. Called by the sensor-field buttons in html.go.
 * @param {string} fieldName - e.g. 'humidity', 'lux'.
 * @returns {void}
 */
function insertField(fieldName) {
    const textarea = document.getElementById('alarmCondition');
    const cursorPos = textarea.selectionStart;
    const textBefore = textarea.value.substring(0, cursorPos);
    const textAfter = textarea.value.substring(cursorPos);
    
    // Add space before if needed
    const needsSpaceBefore = textBefore.length > 0 && !textBefore.endsWith(' ') && !textBefore.endsWith('(');
    const prefix = needsSpaceBefore ? ' ' : '';
    
    textarea.value = textBefore + prefix + fieldName + textAfter;
    textarea.focus();
    textarea.setSelectionRange(cursorPos + prefix.length + fieldName.length, cursorPos + prefix.length + fieldName.length);
}

/**
 * onchange handler for the "Insert Variable" dropdowns: inserts the chosen
 * {{placeholder}} at the cursor and resets the dropdown. Reads the triggering
 * <select> from the global `event` object, so it only works as an inline handler.
 * @param {string} textareaId - Target textarea id.
 * @param {string} [alternateId] - Second candidate (e.g. email subject vs body); used
 *     instead of textareaId when it currently has focus.
 * @returns {void}
 */
function insertVariable(textareaId, alternateId) {
    const select = event.target;
    const variable = select.value;
    if (!variable) return;
    
    // Determine which textarea to use
    let targetId = textareaId;
    if (alternateId) {
        // For email, determine if we should insert into subject or body based on focus
        const subject = document.getElementById(textareaId);
        const body = document.getElementById(alternateId);
        if (document.activeElement === body) {
            targetId = alternateId;
        }
    }
    
    const textarea = document.getElementById(targetId);
    const cursorPos = textarea.selectionStart;
    const textBefore = textarea.value.substring(0, cursorPos);
    const textAfter = textarea.value.substring(cursorPos);
    
    textarea.value = textBefore + variable + textAfter;
    textarea.focus();
    const newPos = cursorPos + variable.length;
    textarea.setSelectionRange(newPos, newPos);
    
    // Reset dropdown
    select.selectedIndex = 0;
}

/**
 * Show/hide each delivery method's message section according to its checkbox.
 * @returns {void}
 */
function toggleMessageSections() {
    // Show/hide message sections based on selected delivery methods
    const consoleChecked = document.getElementById('deliveryConsole').checked;
    const syslogChecked = document.getElementById('deliverySyslog').checked;
    const oslogChecked = document.getElementById('deliveryOslog').checked;
    const eventlogChecked = document.getElementById('deliveryEventlog').checked;
    const emailChecked = document.getElementById('deliveryEmail').checked;
    const smsChecked = document.getElementById('deliverySMS').checked;
    const webhookChecked = document.getElementById('deliveryWebhook').checked;
    const csvChecked = document.getElementById('deliveryCSV').checked;
    const jsonChecked = document.getElementById('deliveryJSON').checked;
    
    // Message sections for each delivery method
    document.getElementById('consoleMessageSection').style.display = consoleChecked ? 'block' : 'none';
    document.getElementById('syslogMessageSection').style.display = syslogChecked ? 'block' : 'none';
    document.getElementById('oslogMessageSection').style.display = oslogChecked ? 'block' : 'none';
    document.getElementById('eventlogMessageSection').style.display = eventlogChecked ? 'block' : 'none';
    document.getElementById('emailMessageSection').style.display = emailChecked ? 'block' : 'none';
    document.getElementById('smsMessageSection').style.display = smsChecked ? 'block' : 'none';
    document.getElementById('webhookMessageSection').style.display = webhookChecked ? 'block' : 'none';
    document.getElementById('csvMessageSection').style.display = csvChecked ? 'block' : 'none';
    document.getElementById('jsonMessageSection').style.display = jsonChecked ? 'block' : 'none';
}

/**
 * Shows the schedule sub-section matching #scheduleType (time/daily share one
 * section) and shows the timezone field for any non-empty type. Every element is
 * null-checked so the form works on pages without the schedule UI.
 * @returns {void}
 */
function toggleScheduleFields() {
    const scheduleTypeEl = document.getElementById('scheduleType');
    if (!scheduleTypeEl) return;
    
    const scheduleType = scheduleTypeEl.value;
    
    // Hide all schedule sections
    const timeSection = document.getElementById('timeScheduleSection');
    const weeklySection = document.getElementById('weeklyScheduleSection');
    const sunSection = document.getElementById('sunScheduleSection');
    const timezoneSection = document.getElementById('timezoneSection');
    
    if (timeSection) timeSection.style.display = 'none';
    if (weeklySection) weeklySection.style.display = 'none';
    if (sunSection) sunSection.style.display = 'none';
    
    // Show timezone field for all schedule types except empty (always active)
    if (timezoneSection) {
        timezoneSection.style.display = scheduleType ? 'block' : 'none';
    }
    
    // Show relevant section based on type
    if (scheduleType === 'time' || scheduleType === 'daily') {
        if (timeSection) timeSection.style.display = 'block';
    } else if (scheduleType === 'weekly') {
        if (weeklySection) weeklySection.style.display = 'block';
    } else if (scheduleType === 'sun') {
        if (sunSection) sunSection.style.display = 'block';
    }
}

/**
 * Shows the optional weekly start/end time fields when #weeklyTimeRange is checked.
 * @returns {void}
 */
function toggleWeeklyTimeRange() {
    const checkbox = document.getElementById('weeklyTimeRange');
    const fields = document.getElementById('weeklyTimeRangeFields');
    if (!checkbox || !fields) return;
    
    fields.style.display = checkbox.checked ? 'block' : 'none';
}

/**
 * Shows the optional sun end-event fields when #sunHasEndEvent is checked.
 * @returns {void}
 */
function toggleSunEndEvent() {
    const checkbox = document.getElementById('sunHasEndEvent');
    const fields = document.getElementById('sunEndEventFields');
    if (!checkbox || !fields) return;
    
    fields.style.display = checkbox.checked ? 'block' : 'none';
}

/**
 * Shows the latitude/longitude inputs when #sunHasCustomLocation is checked.
 * @returns {void}
 */
function toggleCustomLocation() {
    const checkbox = document.getElementById('sunHasCustomLocation');
    const inputs = document.getElementById('customLocationInputs');
    if (!checkbox || !inputs) return;
    
    inputs.style.display = checkbox.checked ? 'block' : 'none';
}

/**
 * Resets every schedule input to its default (no schedule, sunrise/sunset with zero
 * offsets, no days, no custom location) and re-runs the visibility toggles.
 * @returns {void}
 */
function clearScheduleForm() {
    // Helper to safely clear an element if it exists
    /**
     * Sets an element's value if the element exists.
     * @param {string} id - Element id.
     * @param {string} value
     * @returns {void}
     */
    const safeSet = (id, value) => {
        const el = document.getElementById(id);
        if (el) el.value = value;
    };
    
    /**
     * Sets a checkbox's checked state if the element exists.
     * @param {string} id - Element id.
     * @param {boolean} checked
     * @returns {void}
     */
    const safeCheck = (id, checked) => {
        const el = document.getElementById(id);
        if (el) el.checked = checked;
    };
    
    // Reset schedule type
    safeSet('scheduleType', '');
    
    // Clear timezone
    safeSet('scheduleTimezone', '');
    
    // Clear time/daily fields
    safeSet('scheduleStartTime', '');
    safeSet('scheduleEndTime', '');
    
    // Clear weekly fields
    document.querySelectorAll('.schedule-day').forEach(cb => cb.checked = false);
    safeCheck('weeklyTimeRange', false);
    safeSet('weeklyStartTime', '');
    safeSet('weeklyEndTime', '');
    
    // Clear sun fields
    safeSet('scheduleSunEvent', 'sunrise');
    safeSet('scheduleSunOffset', '0');
    safeCheck('sunHasEndEvent', false);
    safeSet('scheduleSunEventEnd', 'sunset');
    safeSet('scheduleSunOffsetEnd', '0');
    safeCheck('scheduleUseStationLocation', false);
    safeCheck('sunHasCustomLocation', false);
    safeSet('scheduleLatitude', '');
    safeSet('scheduleLongitude', '');
    
    // Hide all sections (only if elements exist)
    if (document.getElementById('scheduleType')) {
        toggleScheduleFields();
        toggleWeeklyTimeRange();
        toggleSunEndEvent();
        toggleCustomLocation();
    }
}

/**
 * Fills the schedule inputs from an alarm's schedule, or clears them when the alarm
 * has none. Only sets fields present in the schedule; it does not clear the others
 * first (see editAlarm).
 * @param {?AlarmSchedule} schedule
 * @returns {void}
 */
function loadScheduleIntoForm(schedule) {
    if (!schedule || !schedule.type) {
        clearScheduleForm();
        return;
    }
    
    // Set schedule type
    document.getElementById('scheduleType').value = schedule.type;
    
    // Set timezone
    const timezoneEl = document.getElementById('scheduleTimezone');
    if (timezoneEl && schedule.timezone) {
        timezoneEl.value = schedule.timezone;
    }
    
    // Load type-specific fields
    if (schedule.type === 'time' || schedule.type === 'daily') {
        document.getElementById('scheduleStartTime').value = schedule.start_time || '';
        document.getElementById('scheduleEndTime').value = schedule.end_time || '';
    } else if (schedule.type === 'weekly') {
        // Set day checkboxes
        if (schedule.days_of_week && Array.isArray(schedule.days_of_week)) {
            schedule.days_of_week.forEach(day => {
                const checkbox = document.querySelector(`.schedule-day[value="${day}"]`);
                if (checkbox) checkbox.checked = true;
            });
        }
        
        // Set time range if present
        if (schedule.start_time && schedule.end_time) {
            document.getElementById('weeklyTimeRange').checked = true;
            document.getElementById('weeklyStartTime').value = schedule.start_time;
            document.getElementById('weeklyEndTime').value = schedule.end_time;
        }
    } else if (schedule.type === 'sun') {
        document.getElementById('scheduleSunEvent').value = schedule.sun_event || 'sunrise';
        document.getElementById('scheduleSunOffset').value = schedule.sun_offset || 0;
        
        // Set end event if present
        if (schedule.sun_event_end) {
            document.getElementById('sunHasEndEvent').checked = true;
            document.getElementById('scheduleSunEventEnd').value = schedule.sun_event_end;
            document.getElementById('scheduleSunOffsetEnd').value = schedule.sun_offset_end || 0;
        }
        
        // Set location options
        document.getElementById('scheduleUseStationLocation').checked = schedule.use_station_location || false;
        
        if (schedule.latitude !== undefined && schedule.longitude !== undefined) {
            document.getElementById('sunHasCustomLocation').checked = true;
            document.getElementById('scheduleLatitude').value = schedule.latitude;
            document.getElementById('scheduleLongitude').value = schedule.longitude;
        }
    }
    
    // Trigger UI updates
    toggleScheduleFields();
    toggleWeeklyTimeRange();
    toggleSunEndEvent();
    toggleCustomLocation();
}

/**
 * Builds an AlarmSchedule from the schedule inputs. Offsets are parsed as integers
 * (NaN becomes 0); a custom location is included only when both coordinates parse.
 * @returns {?AlarmSchedule} null when no schedule type is selected (always active).
 */
function serializeScheduleFromForm() {
    const scheduleType = document.getElementById('scheduleType').value;
    
    // If no type selected, return null (always active)
    if (!scheduleType) {
        return null;
    }
    
    const schedule = {
        type: scheduleType
    };
    
    // Add timezone if specified
    const timezoneEl = document.getElementById('scheduleTimezone');
    if (timezoneEl && timezoneEl.value.trim()) {
        schedule.timezone = timezoneEl.value.trim();
    }
    
    if (scheduleType === 'time' || scheduleType === 'daily') {
        schedule.start_time = document.getElementById('scheduleStartTime').value;
        schedule.end_time = document.getElementById('scheduleEndTime').value;
    } else if (scheduleType === 'weekly') {
        // Collect selected days
        const selectedDays = [];
        document.querySelectorAll('.schedule-day:checked').forEach(cb => {
            selectedDays.push(parseInt(cb.value));
        });
        schedule.days_of_week = selectedDays;
        
        // Add time range if specified
        if (document.getElementById('weeklyTimeRange').checked) {
            schedule.start_time = document.getElementById('weeklyStartTime').value;
            schedule.end_time = document.getElementById('weeklyEndTime').value;
        }
    } else if (scheduleType === 'sun') {
        schedule.sun_event = document.getElementById('scheduleSunEvent').value;
        schedule.sun_offset = parseInt(document.getElementById('scheduleSunOffset').value) || 0;
        
        // Add end event if specified
        if (document.getElementById('sunHasEndEvent').checked) {
            schedule.sun_event_end = document.getElementById('scheduleSunEventEnd').value;
            schedule.sun_offset_end = parseInt(document.getElementById('scheduleSunOffsetEnd').value) || 0;
        }
        
        // Add location options
        schedule.use_station_location = document.getElementById('scheduleUseStationLocation').checked;
        
        if (document.getElementById('sunHasCustomLocation').checked) {
            const lat = parseFloat(document.getElementById('scheduleLatitude').value);
            const lon = parseFloat(document.getElementById('scheduleLongitude').value);
            if (!isNaN(lat) && !isNaN(lon)) {
                schedule.latitude = lat;
                schedule.longitude = lon;
            }
        }
    }
    
    return schedule;
}

/**
 * Opens the edit modal in "create" mode: clears currentAlarm and every field,
 * selects console delivery only, fills each delivery method with a ready-made
 * default template, then asynchronously pulls default recipients via
 * loadEnvDefaults.
 * @returns {void}
 */
function showCreateModal() {
    currentAlarm = null;
    document.getElementById('alarmName').value = '';
    document.getElementById('alarmName').readOnly = false;
    document.getElementById('alarmDescription').value = '';
    document.getElementById('alarmCondition').value = '';
    document.getElementById('alarmCooldown').value = '1800';
    document.getElementById('alarmEnabled').checked = true;
    
    // Reset validation result
    const validationResult = document.getElementById('validationResult');
    if (validationResult) {
        validationResult.style.display = 'none';
        validationResult.innerHTML = '';
    }
    
    // Reset delivery methods to console only
    document.getElementById('deliveryConsole').checked = true;
    document.getElementById('deliverySyslog').checked = false;
    document.getElementById('deliveryOslog').checked = false;
    document.getElementById('deliveryEventlog').checked = false;
    document.getElementById('deliveryEmail').checked = false;
    document.getElementById('deliverySMS').checked = false;
    document.getElementById('deliveryWebhook').checked = false;
    document.getElementById('deliveryCSV').checked = false;
    document.getElementById('deliveryJSON').checked = false;
    
    // Set default messages with nice formatting
    // Console: Simple, clean terminal output
    document.getElementById('consoleMessage').value = `🚨 WEATHER ALARM TRIGGERED
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Alarm:       {{alarm_name}}
Description: {{alarm_description}}
Station:     {{station}}
Time:        {{timestamp}}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
{{sensor_info}}`;
    
    // Syslog: Compact, structured format
    document.getElementById('syslogMessage').value = `tempest-alarm: {{alarm_name}} triggered - {{alarm_description}} | station={{station}} time={{timestamp}} | {{sensor_info}}`;
    
    // OSLog: Clean macOS logging format
    document.getElementById('oslogMessage').value = `[ALARM] {{alarm_name}}: {{alarm_description}} at {{station}} ({{timestamp}})`;
    
    // Event Log: Windows Event Viewer style
    document.getElementById('eventlogMessage').value = `Event Type: Warning
Source: Tempest HomeKit
Event ID: 1001
Description: Weather alarm condition detected

{{alarm_info}}

Station: {{station}}
Date/Time: {{timestamp}}

Sensor Data:
{{sensor_info}}

{{app_info}}`;
    
    // Email: Professional HTML-ready format
    // Will be populated from env defaults after modal opens
    document.getElementById('emailSubject').value = '⚠️ Weather Alert: {{alarm_name}}';
    document.getElementById('emailHtml').checked = true;
    document.getElementById('emailBody').value = `<h2 style="color: #d9534f;">⚠️ Weather Alarm Triggered</h2>

<div style="background: #f5f5f5; padding: 15px; border-left: 4px solid #d9534f; margin: 20px 0;">
    <h3 style="margin-top: 0;">{{alarm_name}}</h3>
    <p>{{alarm_description}}</p>
</div>

<table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
    <tr style="background: #f9f9f9;">
        <td style="padding: 10px; border: 1px solid #ddd;"><strong>Station</strong></td>
        <td style="padding: 10px; border: 1px solid #ddd;">{{station}}</td>
    </tr>
    <tr>
        <td style="padding: 10px; border: 1px solid #ddd;"><strong>Time</strong></td>
        <td style="padding: 10px; border: 1px solid #ddd;">{{timestamp}}</td>
    </tr>
    <tr style="background: #f9f9f9;">
        <td style="padding: 10px; border: 1px solid #ddd;"><strong>Condition</strong></td>
        <td style="padding: 10px; border: 1px solid #ddd;">{{alarm_condition}}</td>
    </tr>
</table>

<h4>Current Sensor Readings:</h4>
<div style="background: #fff; padding: 15px; border: 1px solid #ddd;">
    {{sensor_info}}
</div>

<hr style="margin: 20px 0; border: none; border-top: 1px solid #ddd;">
<p style="color: #666; font-size: 12px;">{{app_info}}</p>`;
    
    // SMS: Very concise
    // Will be populated from env defaults after modal opens
    document.getElementById('smsMessage').value = `⚠️ {{alarm_name}} at {{station}} - {{timestamp}}. {{alarm_description}}`;
    
    // Webhook: JSON payload with alarm and sensor data
    document.getElementById('webhookUrl').value = '';
    document.getElementById('webhookMethod').value = 'POST';
    document.getElementById('webhookHeaders').value = `{
  "Content-Type": "application/json",
  "User-Agent": "Tempest-HomeKit-Alarm"
}`;
    document.getElementById('webhookBody').value = `{
  "alarm": {
    "name": "{{alarm_name}}",
    "description": "{{alarm_description}}",
    "condition": "{{alarm_condition}}",
    "tags": "{{alarm_tags}}"
  },
  "station": "{{station}}",
  "timestamp": "{{timestamp}}",
  "sensors": {
    "temperature_c": {{temperature}},
    "temperature_f": {{temperature_f}},
    "humidity": {{humidity}},
    "pressure_mb": {{pressure}},
    "wind_speed_ms": {{wind_speed}},
    "wind_gust_ms": {{wind_gust}},
    "wind_direction_deg": {{wind_direction}},
    "illuminance_lux": {{lux}},
    "uv_index": {{uv}},
    "rain_rate_mmh": {{rain_rate}},
    "rain_daily_mm": {{rain_daily}},
    "lightning_count": {{lightning_count}},
    "lightning_distance_km": {{lightning_distance}}
  },
  "app_info": "{{app_info}}"
}`;
    document.getElementById('webhookContentType').value = 'application/json';
    
    // CSV: Default path and message with timestamp, alarm info, and sensor data
    document.getElementById('csvPath').value = '/tmp/tempest-alarms.csv';
    document.getElementById('csvMaxDays').value = 30;
    document.getElementById('csvMessage').value = '{{alarm_name}},{{alarm_description}},{{temperature}},{{humidity}},{{pressure}},{{wind_speed}},{{lux}},{{uv}},{{rain_daily}},{{message}}';
    
    // JSON: Default path and message with timestamp, message, alarm info, and sensor info
    document.getElementById('jsonPath').value = '/tmp/tempest-alarms.json';
    document.getElementById('jsonMaxDays').value = 30;
    document.getElementById('jsonMessage').value = '{"timestamp": "{{timestamp}}", "message": "ALARM: {{alarm_name}} triggered", "alarm": {{alarm_info}}, "sensors": {{sensor_info}}}';
    
    selectedTags = [];
    renderSelectedTags();
    document.getElementById('tagSearchInput').value = '';
    updateTagDropdown('');
    
    // Clear contacts
    selectedEmailContacts = [];
    selectedSMSContacts = [];
    renderSelectedContacts('email');
    renderSelectedContacts('sms');
    document.getElementById('emailContactSearch').value = '';
    document.getElementById('smsContactSearch').value = '';
    
    // Clear schedule
    clearScheduleForm();
    
    toggleMessageSections();
    
    document.getElementById('editModal').classList.add('active');
    
    // Load environment defaults for email/SMS addresses
    loadEnvDefaults();
    
    // Populate contact dropdowns
    // populateContactDropdowns(); // No longer needed with dynamic dropdowns
}

/**
 * Fetches /api/env-defaults ({emailTo, smsTo}: comma-separated values from the
 * server's MS365_TO_ADDRESS and SMS_TO_NUMBER environment variables) and uses them
 * as the recipient lists, but only when the user has not already chosen any.
 * Failures are logged and ignored.
 * @returns {Promise<void>}
 */
async function loadEnvDefaults() {
    try {
        const response = await fetch('/api/env-defaults');
        const defaults = await response.json();
        
        // Only set defaults if contact arrays are empty (don't override user selections)
        if (defaults.emailTo && selectedEmailContacts.length === 0) {
            // Parse comma-separated emails and add them
            const emails = defaults.emailTo.split(',').map(e => e.trim()).filter(e => e);
            selectedEmailContacts = emails;
            renderSelectedContacts('email');
        }
        if (defaults.smsTo && selectedSMSContacts.length === 0) {
            // Parse comma-separated numbers and add them
            const numbers = defaults.smsTo.split(',').map(n => n.trim()).filter(n => n);
            selectedSMSContacts = numbers;
            renderSelectedContacts('sms');
        }
    } catch (error) {
        console.warn('Failed to load environment defaults:', error);
    }
}

/**
 * Opens the edit modal for an existing alarm: clears every field (so values from a
 * previous edit do not leak through), then loads the alarm's tags, cooldown,
 * delivery checkboxes, per-channel templates/settings, recipients and schedule.
 * @param {string} name - Name of the alarm in `alarms`; does nothing if not found.
 * @returns {void}
 */
function editAlarm(name) {
    currentAlarm = alarms.find(a => a.name === name);
    if (!currentAlarm) return;
    
    // Clear all form fields first to prevent values from previous edits
    document.getElementById('alarmName').value = '';
    document.getElementById('alarmName').readOnly = false;
    document.getElementById('alarmDescription').value = '';
    document.getElementById('alarmCondition').value = '';
    document.getElementById('alarmCooldown').value = '1800';
    document.getElementById('alarmEnabled').checked = true;
    
    // Reset validation result
    const validationResult = document.getElementById('validationResult');
    if (validationResult) {
        validationResult.style.display = 'none';
        validationResult.innerHTML = '';
    }
    
    // Clear all delivery method checkboxes
    document.getElementById('deliveryConsole').checked = false;
    document.getElementById('deliverySyslog').checked = false;
    document.getElementById('deliveryOslog').checked = false;
    document.getElementById('deliveryEventlog').checked = false;
    document.getElementById('deliveryEmail').checked = false;
    document.getElementById('deliverySMS').checked = false;
    document.getElementById('deliveryWebhook').checked = false;
    document.getElementById('deliveryCSV').checked = false;
    document.getElementById('deliveryJSON').checked = false;
    
    // Clear all message fields
    document.getElementById('consoleMessage').value = '';
    document.getElementById('syslogMessage').value = '';
    document.getElementById('oslogMessage').value = '';
    document.getElementById('eventlogMessage').value = '';
    document.getElementById('emailSubject').value = '';
    document.getElementById('emailBody').value = '';
    document.getElementById('emailHtml').checked = true;
    document.getElementById('smsMessage').value = '';
    document.getElementById('webhookUrl').value = '';
    document.getElementById('webhookMethod').value = 'POST';
    document.getElementById('webhookHeaders').value = '';
    document.getElementById('webhookBody').value = '';
    document.getElementById('webhookContentType').value = 'application/json';
    document.getElementById('csvPath').value = '';
    document.getElementById('csvMaxDays').value = 30;
    document.getElementById('csvMessage').value = '';
    document.getElementById('jsonPath').value = '';
    document.getElementById('jsonMaxDays').value = 30;
    document.getElementById('jsonMessage').value = '';
    
    // Clear tags
    selectedTags = [];
    renderSelectedTags();
    document.getElementById('tagSearchInput').value = '';
    updateTagDropdown('');
    
    // Clear contacts
    selectedEmailContacts = [];
    selectedSMSContacts = [];
    renderSelectedContacts('email');
    renderSelectedContacts('sms');
    document.getElementById('emailContactSearch').value = '';
    document.getElementById('smsContactSearch').value = '';
    document.getElementById('alarmName').value = currentAlarm.name;
    document.getElementById('alarmDescription').value = currentAlarm.description || '';
    document.getElementById('alarmCondition').value = currentAlarm.condition;
    
    selectedTags = currentAlarm.tags || [];
    renderSelectedTags();
    updateTagDropdown('');
    
    document.getElementById('alarmCooldown').value = currentAlarm.cooldown || 1800;
    document.getElementById('alarmEnabled').checked = currentAlarm.enabled;
    
    // Load delivery methods and messages from channels
    const channels = currentAlarm.channels || [];
    const channelTypes = channels.map(ch => ch.type);
    
    document.getElementById('deliveryConsole').checked = channelTypes.includes('console');
    document.getElementById('deliverySyslog').checked = channelTypes.includes('syslog');
    document.getElementById('deliveryOslog').checked = channelTypes.includes('oslog');
    document.getElementById('deliveryEventlog').checked = channelTypes.includes('eventlog');
    document.getElementById('deliveryEmail').checked = channelTypes.includes('email');
    document.getElementById('deliverySMS').checked = channelTypes.includes('sms');
    document.getElementById('deliveryWebhook').checked = channelTypes.includes('webhook');
    document.getElementById('deliveryCSV').checked = channelTypes.includes('csv');
    document.getElementById('deliveryJSON').checked = channelTypes.includes('json');
    
    // Load messages from channels
    channels.forEach(channel => {
        if (channel.type === 'console' && channel.template) {
            document.getElementById('consoleMessage').value = channel.template;
        } else if (channel.type === 'syslog' && channel.template) {
            document.getElementById('syslogMessage').value = channel.template;
        } else if (channel.type === 'oslog' && channel.template) {
            document.getElementById('oslogMessage').value = channel.template;
        } else if (channel.type === 'eventlog' && channel.template) {
            document.getElementById('eventlogMessage').value = channel.template;
        } else if (channel.type === 'email' && channel.email) {
            selectedEmailContacts = channel.email.to || [];
            document.getElementById('emailSubject').value = channel.email.subject || '';
            document.getElementById('emailBody').value = channel.email.body || '';
            document.getElementById('emailHtml').checked = channel.email.html || false;
        } else if (channel.type === 'sms' && channel.sms) {
            selectedSMSContacts = channel.sms.to || [];
            document.getElementById('smsMessage').value = channel.sms.message || '';
        } else if (channel.type === 'webhook' && channel.webhook) {
            document.getElementById('webhookUrl').value = channel.webhook.url || '';
            document.getElementById('webhookMethod').value = channel.webhook.method || 'POST';
            document.getElementById('webhookHeaders').value = channel.webhook.headers ? JSON.stringify(channel.webhook.headers, null, 2) : '';
            document.getElementById('webhookBody').value = channel.webhook.body || '';
            document.getElementById('webhookContentType').value = channel.webhook.content_type || 'application/json';
        } else if (channel.type === 'csv' && channel.csv) {
            document.getElementById('csvPath').value = channel.csv.path || '';
            document.getElementById('csvMaxDays').value = channel.csv.max_days || 30;
            document.getElementById('csvMessage').value = channel.csv.message || '';
        } else if (channel.type === 'json' && channel.json) {
            document.getElementById('jsonPath').value = channel.json.path || '';
            document.getElementById('jsonMaxDays').value = channel.json.max_days || 30;
            document.getElementById('jsonMessage').value = channel.json.message || '';
        }
    });
    
    // Render contacts
    renderSelectedContacts('email');
    renderSelectedContacts('sms');
    
    // Load schedule
    loadScheduleIntoForm(currentAlarm.schedule);
    
    toggleMessageSections();
    
    document.getElementById('editModal').classList.add('active');
    
    // Populate contact dropdowns
    // populateContactDropdowns(); // No longer needed with dynamic dropdowns
}

/**
 * Hides the alarm edit modal (unsaved changes are discarded).
 * @returns {void}
 */
function closeModal() {
    document.getElementById('editModal').classList.remove('active');
}

/**
 * Hides the JSON viewer modal.
 * @returns {void}
 */
function closeJSONModal() {
    document.getElementById('jsonModal').classList.remove('active');
}

/**
 * Validates the condition textarea via POST /api/validate and shows the result
 * (with the server's plain-English paraphrase on success) in #validationResult.
 * Used by the Validate button and as a gate in handleSubmit.
 * @returns {Promise<boolean>} true only if the server reports the condition valid.
 */
async function validateCondition() {
    const condition = document.getElementById('alarmCondition').value;
    const resultDiv = document.getElementById('validationResult');
    
    if (!condition || condition.trim() === '') {
        resultDiv.style.display = 'block';
        resultDiv.style.backgroundColor = '#fff3cd';
        resultDiv.style.color = '#856404';
        resultDiv.innerHTML = '⚠️ Please enter a condition to validate';
        return false;
    }
    
    try {
        const response = await fetch('/api/validate', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ condition: condition })
        });
        
        const result = await response.json();
        resultDiv.style.display = 'block';
        
        if (result.valid) {
            resultDiv.style.backgroundColor = '#d4edda';
            resultDiv.style.color = '#155724';
            resultDiv.innerHTML = `✓ Valid condition!<br><strong>Meaning:</strong> ${result.paraphrase}`;
            return true;
        } else {
            resultDiv.style.backgroundColor = '#f8d7da';
            resultDiv.style.color = '#721c24';
            resultDiv.innerHTML = `✗ Invalid condition: ${result.error}`;
            return false;
        }
    } catch (error) {
        resultDiv.style.display = 'block';
        resultDiv.style.backgroundColor = '#f8d7da';
        resultDiv.style.color = '#721c24';
        resultDiv.innerHTML = `✗ Validation error: ${error.message}`;
        return false;
    }
}

/**
 * Validates the JSON-delivery template via POST /api/validate-json and shows the
 * expanded sample output (pretty-printed when it parses) in #jsonValidationResult.
 * @returns {Promise<boolean>} true only if the server reports the template valid.
 */
async function validateJSONMessage() {
    const template = document.getElementById('jsonMessage').value;
    const resultDiv = document.getElementById('jsonValidationResult');
    
    if (!template || template.trim() === '') {
        resultDiv.style.display = 'block';
        resultDiv.style.backgroundColor = '#fff3cd';
        resultDiv.style.color = '#856404';
        resultDiv.innerHTML = '⚠️ Please enter a JSON template to validate';
        return false;
    }
    
    try {
        const response = await fetch('/api/validate-json', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ template: template })
        });
        
        const result = await response.json();
        resultDiv.style.display = 'block';
        
        if (result.valid) {
            resultDiv.style.backgroundColor = '#d4edda';
            resultDiv.style.color = '#155724';
            resultDiv.innerHTML = `✓ Valid JSON template!<br><strong>Sample output:</strong><br><pre style="font-size: 11px; margin-top: 4px; word-wrap: break-word; white-space: pre-wrap;">${JSON.stringify(JSON.parse(result.expanded), null, 2)}</pre>`;
            return true;
        } else {
            resultDiv.style.backgroundColor = '#f8d7da';
            resultDiv.style.color = '#721c24';
            
            // Try to pretty-print even invalid JSON for better readability
            let formattedExpanded;
            try {
                formattedExpanded = JSON.stringify(JSON.parse(result.expanded), null, 2);
            } catch (parseError) {
                // If we can't parse it as JSON, show the raw text with some formatting
                formattedExpanded = result.expanded.replace(/\\n/g, '\n').replace(/\\t/g, '\t');
            }
            
            resultDiv.innerHTML = `✗ Invalid JSON template: ${result.error}<br><strong>Expanded result:</strong><br><pre style="font-size: 11px; margin-top: 4px; word-wrap: break-word; white-space: pre-wrap;">${formattedExpanded}</pre>`;
            return false;
        }
    } catch (error) {
        resultDiv.style.display = 'block';
        resultDiv.style.backgroundColor = '#f8d7da';
        resultDiv.style.color = '#721c24';
        resultDiv.innerHTML = `✗ Validation error: ${error.message}`;
        return false;
    }
}

/**
 * Expands the JSON-delivery template via /api/validate-json and shows the sample
 * output in the JSON modal, valid or not.
 * @returns {Promise<void>}
 */
async function showSampleJSON() {
    const template = document.getElementById('jsonMessage').value;
    
    if (!template || template.trim() === '') {
        showNotification('Please enter a JSON template first', 'error');
        return;
    }
    
    try {
        const response = await fetch('/api/validate-json', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ template: template })
        });
        
        const result = await response.json();
        
        // Format the expanded result for display
        let formattedExpanded;
        let title;
        
        if (result.valid) {
            formattedExpanded = JSON.stringify(JSON.parse(result.expanded), null, 2);
            title = 'Sample JSON Output (Valid)';
        } else {
            title = 'Sample JSON Output (Invalid)';
            try {
                formattedExpanded = JSON.stringify(JSON.parse(result.expanded), null, 2);
            } catch (parseError) {
                formattedExpanded = result.expanded.replace(/\\n/g, '\n').replace(/\\t/g, '\t');
            }
        }
        
        displayJSON(formattedExpanded, title);
    } catch (error) {
        showNotification('Error generating sample JSON: ' + error.message, 'error');
    }
}

/**
 * Shows the whole in-memory alarm config in the JSON modal.
 * @returns {void}
 */
function showFullJSON() {
    const config = { alarms: alarms };
    displayJSON(config, 'Full Configuration JSON');
}

/**
 * Shows one alarm's JSON in the JSON modal.
 * @param {string} name - Alarm name; does nothing if not found.
 * @returns {void}
 */
function showAlarmJSON(name) {
    const alarm = alarms.find(a => a.name === name);
    if (!alarm) return;
    displayJSON(alarm, 'Alarm: ' + name);
}

/**
 * Opens the JSON modal with the given content (set as textContent, so it is safe to
 * display untrusted text).
 * @param {(Object|string)} data - Object to pretty-print, or an already formatted string.
 * @param {string} title - Modal heading.
 * @returns {void}
 */
function displayJSON(data, title) {
    document.getElementById('jsonModalTitle').textContent = title;
    
    let jsonString;
    if (typeof data === 'string') {
        // If data is already a formatted JSON string, use it directly
        jsonString = data;
    } else {
        // If data is an object, stringify it
        jsonString = JSON.stringify(data, null, 2);
    }
    
    document.getElementById('jsonContent').textContent = jsonString;
    document.getElementById('jsonModal').classList.add('active');
}

/**
 * Copies the JSON modal's content to the clipboard, falling back to a hidden
 * textarea plus document.execCommand('copy') where the async Clipboard API is
 * unavailable or denied (e.g. non-HTTPS origins).
 * @returns {Promise<void>}
 */
async function copyJSON() {
    const jsonText = document.getElementById('jsonContent').textContent;
    try {
        await navigator.clipboard.writeText(jsonText);
        showNotification('JSON copied to clipboard!', 'success');
    } catch (err) {
        // Fallback for older browsers
        const textarea = document.createElement('textarea');
        textarea.value = jsonText;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        try {
            document.execCommand('copy');
            showNotification('JSON copied to clipboard!', 'success');
        } catch (e) {
            showNotification('Failed to copy JSON', 'error');
        }
        document.body.removeChild(textarea);
    }
}

/**
 * Submit handler for #alarmForm. Validates the condition (and the JSON template when
 * JSON delivery is on), builds the channels array from the checked delivery methods,
 * substituting a default template for any blank one, and POSTs the alarm to
 * /api/alarms/update?oldName=... (editing, so renames work) or /api/alarms/create.
 * On success it closes the modal and reloads alarms and tags.
 *
 * Email/SMS channels with no recipients are saved with placeholder recipients
 * (admin@example.com / +1234567890).
 * @param {SubmitEvent} e
 * @returns {Promise<void>}
 */
async function handleSubmit(e) {
    e.preventDefault();
    
    // Validate condition before saving
    const isValid = await validateCondition();
    if (!isValid) {
        showNotification('Please fix the condition before saving', 'error');
        return;
    }
    
    // Validate JSON template if JSON delivery is selected
    if (document.getElementById('deliveryJSON').checked) {
        const jsonValid = await validateJSONMessage();
        if (!jsonValid) {
            showNotification('Please fix the JSON template before saving', 'error');
            return;
        }
    }
    
    // Build channels array from selected delivery methods
    const channels = [];
    
    if (document.getElementById('deliveryConsole').checked) {
        const template = document.getElementById('consoleMessage').value || '🚨 ALARM: {{alarm_name}}\nStation: {{station}}\nTime: {{timestamp}}';
        channels.push({ 
            type: 'console',
            template: template
        });
    }
    if (document.getElementById('deliverySyslog').checked) {
        const template = document.getElementById('syslogMessage').value || 'tempest-alarm: {{alarm_name}} - {{alarm_description}}';
        channels.push({ 
            type: 'syslog',
            template: template
        });
    }
    if (document.getElementById('deliveryOslog').checked) {
        const template = document.getElementById('oslogMessage').value || '[ALARM] {{alarm_name}}: {{alarm_description}}';
        channels.push({ 
            type: 'oslog',
            template: template
        });
    }
    if (document.getElementById('deliveryEventlog').checked) {
        const template = document.getElementById('eventlogMessage').value || 'Weather alarm: {{alarm_name}}';
        channels.push({ 
            type: 'eventlog',
            template: template
        });
    }
    if (document.getElementById('deliveryEmail').checked) {
        const emailSubject = document.getElementById('emailSubject').value || 'Tempest Alert: {{alarm_name}}';
        const emailBody = document.getElementById('emailBody').value || '{{alarm_info}}\n\n{{sensor_info}}';
        const emailHtml = document.getElementById('emailHtml').checked;
        
        channels.push({ 
            type: 'email',
            email: {
                to: selectedEmailContacts.length > 0 ? selectedEmailContacts : ['admin@example.com'],
                subject: emailSubject,
                body: emailBody,
                html: emailHtml
            }
        });
    }
    if (document.getElementById('deliverySMS').checked) {
        const smsMessage = document.getElementById('smsMessage').value;
        
        channels.push({ 
            type: 'sms',
            sms: {
                to: selectedSMSContacts.length > 0 ? selectedSMSContacts : ['+1234567890'],
                message: smsMessage || 'ALARM: {{alarm_name}} at {{timestamp}}'
            }
        });
    }
    if (document.getElementById('deliveryWebhook').checked) {
        const webhookUrl = document.getElementById('webhookUrl').value;
        const webhookMethod = document.getElementById('webhookMethod').value || 'POST';
        const webhookHeadersStr = document.getElementById('webhookHeaders').value;
        const webhookBody = document.getElementById('webhookBody').value;
        const webhookContentType = document.getElementById('webhookContentType').value || 'application/json';
        
        let webhookHeaders = {};
        if (webhookHeadersStr.trim()) {
            try {
                webhookHeaders = JSON.parse(webhookHeadersStr);
            } catch (e) {
                showNotification('Invalid JSON in webhook headers', 'error');
                return;
            }
        }
        
        channels.push({ 
            type: 'webhook',
            webhook: {
                url: webhookUrl,
                method: webhookMethod,
                headers: webhookHeaders,
                body: webhookBody,
                content_type: webhookContentType
            }
        });
    }
    
    if (document.getElementById('deliveryCSV').checked) {
        const csvPath = document.getElementById('csvPath').value;
        const csvMaxDays = parseInt(document.getElementById('csvMaxDays').value) || 30;
        const csvMessage = document.getElementById('csvMessage').value || '{{alarm_name}},{{alarm_description}},{{temperature}},{{humidity}},{{pressure}},{{wind_speed}},{{lux}},{{uv}},{{rain_daily}}';
        
        channels.push({ 
            type: 'csv',
            csv: {
                path: csvPath,
                max_days: csvMaxDays,
                message: csvMessage
            }
        });
    }
    
    if (document.getElementById('deliveryJSON').checked) {
        const jsonPath = document.getElementById('jsonPath').value;
        const jsonMaxDays = parseInt(document.getElementById('jsonMaxDays').value) || 30;
        const jsonMessage = document.getElementById('jsonMessage').value || '{"timestamp": "{{timestamp}}", "message": "ALARM: {{alarm_name}} triggered", "alarm": {{alarm_info}}, "sensors": {{sensor_info}}}';
        
        channels.push({ 
            type: 'json',
            json: {
                path: jsonPath,
                max_days: jsonMaxDays,
                message: jsonMessage
            }
        });
    }
    
    // Serialize schedule
    const schedule = serializeScheduleFromForm();
    
    const alarmData = {
        name: document.getElementById('alarmName').value,
        description: document.getElementById('alarmDescription').value,
        condition: document.getElementById('alarmCondition').value,
        tags: selectedTags,
        cooldown: parseInt(document.getElementById('alarmCooldown').value),
        enabled: document.getElementById('alarmEnabled').checked,
        channels: channels
    };
    
    // Only include schedule if it's not null (not always active)
    if (schedule !== null) {
        alarmData.schedule = schedule;
    }
    
    // Track original name for updates (in case name changed)
    const originalName = currentAlarm ? currentAlarm.name : null;
    const endpoint = currentAlarm ? `/api/alarms/update?oldName=${encodeURIComponent(originalName)}` : '/api/alarms/create';
    
    try {
        const response = await fetch(endpoint, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(alarmData)
        });
        
        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }
        
        showNotification(currentAlarm ? 'Alarm updated successfully' : 'Alarm created successfully', 'success');
        closeModal();
        await loadAlarms();
        await loadTags();
    } catch (error) {
        showNotification('Error: ' + error.message, 'error');
    }
}

/**
 * Deletes an alarm after a confirm() prompt via POST /api/alarms/delete. Closes any
 * modal showing it, removes it locally for instant feedback, then reloads alarms and
 * tags from the server for consistency.
 * @param {string} name
 * @returns {Promise<void>}
 */
async function deleteAlarm(name) {
    if (!confirm('Are you sure you want to delete alarm "' + name + '"?')) return;
    
    try {
        const response = await fetch('/api/alarms/delete?name=' + encodeURIComponent(name), {
            method: 'POST'
        });
        
        if (!response.ok) {
            throw new Error(await response.text());
        }
        
        // If the deleted alarm is currently being viewed/edited, close the modal
        if (currentAlarm && currentAlarm.name === name) {
            closeModal();
            closeJSONModal();
            currentAlarm = null;
        }
        
        // Remove the alarm from the local array immediately
        alarms = alarms.filter(a => a.name !== name);
        
        // Re-render the UI immediately
        renderAlarms();
        
        showNotification('Alarm deleted successfully', 'success');
        
        // Reload from server to ensure consistency and update tags
        await loadAlarms();
        await loadTags();
    } catch (error) {
        showNotification('Error: ' + error.message, 'error');
    }
}

/**
 * "Save All" button: downloads the server's current config (GET /api/config) as
 * alarms.json. It does not write anything on the server; edits are already persisted
 * by the create/update/delete endpoints.
 * @returns {Promise<void>}
 */
async function saveAll() {
    const response = await fetch('/api/config');
    const config = await response.json();
    
    const blob = new Blob([JSON.stringify(config, null, 2)], {type: 'application/json'});
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'alarms.json';
    a.click();
    
    showNotification('Configuration saved', 'success');
}

/**
 * Shows a toast in #notification for three seconds.
 * @param {string} message - Plain text (set via textContent).
 * @param {('success'|'error')} type - Added as a CSS class.
 * @returns {void}
 */
function showNotification(message, type) {
    const notification = document.getElementById('notification');
    notification.textContent = message;
    notification.className = 'notification ' + type + ' show';
    setTimeout(() => {
        notification.classList.remove('show');
    }, 3000);
}

/**
 * Writes the current local date/time (24-hour, en-US format) into #last-update.
 * @returns {void}
 */
function updateLastUpdateTimestamp() {
    const lastUpdateElement = document.getElementById('last-update');
    if (lastUpdateElement) {
        const now = new Date();
        const lastUpdateText = now.toLocaleString('en-US', {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            hour12: false
        });
        lastUpdateElement.textContent = lastUpdateText;
    }
}

// ============================================
// Contacts and Tags Editor Functions
// ============================================

/**
 * Opens the address-book editor and loads the contacts into it.
 * @returns {void}
 */
function showEditContactsModal() {
    loadContactsForEditor();
    document.getElementById('editContactsModal').classList.add('active');
}

/**
 * Hides the address-book editor.
 * @returns {void}
 */
function closeEditContactsModal() {
    document.getElementById('editContactsModal').classList.remove('active');
}

/**
 * Opens the tag-list editor and loads the tags into it.
 * @returns {void}
 */
function showEditTagsModal() {
    loadTagsForEditor();
    document.getElementById('editTagsModal').classList.add('active');
}

/**
 * Hides the tag-list editor.
 * @returns {void}
 */
function closeEditTagsModal() {
    document.getElementById('editTagsModal').classList.remove('active');
}

/**
 * Fetches /api/contacts fresh and renders it into the address-book editor (an empty
 * editor on failure).
 * @returns {Promise<void>}
 */
async function loadContactsForEditor() {
    try {
        const response = await fetch('/api/contacts');
        const contacts = await response.json();
        renderContactsEditor(contacts);
    } catch (error) {
        console.error('Failed to load contacts for editor:', error);
        renderContactsEditor([]);
    }
}

/**
 * Fetches /api/tags fresh and renders it into the tag-list editor (an empty editor
 * on failure).
 * @returns {Promise<void>}
 */
async function loadTagsForEditor() {
    try {
        // Get all tags from the server (this includes both alarm tags and predefined tags)
        const response = await fetch('/api/tags');
        const tags = await response.json();
        renderTagsEditor(tags);
    } catch (error) {
        console.error('Failed to load tags for editor:', error);
        renderTagsEditor([]);
    }
}

/**
 * Renders one editable row (name, email, SMS, Remove) per contact into #contactsList.
 * Values are interpolated into HTML attributes without escaping.
 * @param {Contact[]} contacts
 * @returns {void}
 */
function renderContactsEditor(contacts) {
    const container = document.getElementById('contactsList');
    container.innerHTML = '';

    if (contacts.length === 0) {
        container.innerHTML = '<p style="text-align: center; color: var(--card-text-light); padding: 20px;">No contacts configured</p>';
        return;
    }

    contacts.forEach((contact, index) => {
        const item = document.createElement('div');
        item.className = 'contact-item';
        item.innerHTML = `
            <input type="text" placeholder="Name" value="${contact.name || ''}" data-field="name" data-index="${index}">
            <input type="email" placeholder="Email" value="${contact.email || ''}" data-field="email" data-index="${index}">
            <input type="tel" placeholder="SMS (+1234567890)" value="${contact.sms || ''}" data-field="sms" data-index="${index}">
            <button class="remove-btn" onclick="removeContactItem(${index})">Remove</button>
        `;
        container.appendChild(item);
    });
}

/**
 * Renders one editable row per tag into #tagsList (values not escaped).
 * @param {string[]} tags
 * @returns {void}
 */
function renderTagsEditor(tags) {
    const container = document.getElementById('tagsList');
    container.innerHTML = '';

    if (tags.length === 0) {
        container.innerHTML = '<p style="text-align: center; color: var(--card-text-light); padding: 20px;">No tags configured</p>';
        return;
    }

    tags.forEach((tag, index) => {
        const item = document.createElement('div');
        item.className = 'tag-item';
        item.innerHTML = `
            <input type="text" placeholder="Tag name" value="${tag}" data-index="${index}">
            <button class="remove-btn" onclick="removeTagItem(${index})">Remove</button>
        `;
        container.appendChild(item);
    });
}

/**
 * Appends a blank contact row to the address-book editor (data-index -1 marks it new).
 * @returns {void}
 */
function addNewContact() {
    const container = document.getElementById('contactsList');
    const item = document.createElement('div');
    item.className = 'contact-item';
    item.innerHTML = `
        <input type="text" placeholder="Name" data-field="name" data-index="-1">
        <input type="email" placeholder="Email" data-field="email" data-index="-1">
        <input type="tel" placeholder="SMS (+1234567890)" data-field="sms" data-index="-1">
        <button class="remove-btn" onclick="removeContactItem(-1)">Remove</button>
    `;
    container.appendChild(item);
}

/**
 * Appends a blank tag row to the tag-list editor (data-index -1 marks it new).
 * @returns {void}
 */
function addNewTag() {
    const container = document.getElementById('tagsList');
    const item = document.createElement('div');
    item.className = 'tag-item';
    item.innerHTML = `
        <input type="text" placeholder="Tag name" data-index="-1">
        <button class="remove-btn" onclick="removeTagItem(-1)">Remove</button>
    `;
    container.appendChild(item);
}

/**
 * Removes the first address-book editor row whose inputs carry `data-index=index`.
 * @param {number} index - Row index from renderContactsEditor, or -1 for new rows.
 * @returns {void}
 */
function removeContactItem(index) {
    const item = document.querySelector(`.contact-item input[data-index="${index}"]`);
    if (item) {
        item.closest('.contact-item').remove();
    }
}

/**
 * Removes the first tag editor row whose input carries `data-index=index`.
 * @param {number} index - Row index from renderTagsEditor, or -1 for new rows.
 * @returns {void}
 */
function removeTagItem(index) {
    const item = document.querySelector(`.tag-item input[data-index="${index}"]`);
    if (item) {
        item.closest('.tag-item').remove();
    }
}

/**
 * Reads the address-book editor rows, skipping rows where every field is blank.
 * @returns {Contact[]}
 */
function collectContactsFromEditor() {
    const contacts = [];
    const contactItems = document.querySelectorAll('.contact-item');

    contactItems.forEach(item => {
        const name = item.querySelector('input[data-field="name"]').value.trim();
        const email = item.querySelector('input[data-field="email"]').value.trim();
        const sms = item.querySelector('input[data-field="sms"]').value.trim();

        if (name || email || sms) {
            contacts.push({
                name: name,
                email: email,
                sms: sms
            });
        }
    });

    return contacts;
}

/**
 * Reads the non-blank, trimmed tag names from the tag-list editor.
 * @returns {string[]}
 */
function collectTagsFromEditor() {
    const tags = [];
    const tagItems = document.querySelectorAll('.tag-item input');

    tagItems.forEach(input => {
        const tag = input.value.trim();
        if (tag) {
            tags.push(tag);
        }
    });

    return tags;
}

/**
 * POSTs the edited address book to /api/contacts/save, then closes the editor and
 * reloads `contacts`. The server rejects contacts with no name, no email/SMS, an
 * email without '@' or an SMS number without a leading '+'.
 * @param {('json'|'env')} saveType - Persist to a JSON file or to the server's env file.
 * @returns {Promise<void>}
 */
async function saveContacts(saveType) {
    const contacts = collectContactsFromEditor();

    try {
        const response = await fetch('/api/contacts/save', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                contacts: contacts,
                saveType: saveType
            })
        });

        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }

        const result = await response.json();
        showNotification(result.message, 'success');
        closeEditContactsModal();

        // Reload contacts for the dropdowns
        await loadContacts();
    } catch (error) {
        showNotification('Error saving contacts: ' + error.message, 'error');
    }
}

/**
 * POSTs the edited tag list to /api/tags/save, then closes the editor and reloads tags.
 * @param {('json'|'env')} saveType - Persist to a JSON file or to the server's env file.
 * @returns {Promise<void>}
 */
async function saveTags(saveType) {
    const tags = collectTagsFromEditor();

    try {
        const response = await fetch('/api/tags/save', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                tags: tags,
                saveType: saveType
            })
        });

        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }

        const result = await response.json();
        showNotification(result.message, 'success');
        closeEditTagsModal();

        // Reload tags for the dropdowns
        await loadTags();
    } catch (error) {
        showNotification('Error saving tags: ' + error.message, 'error');
    }
}

// ============================================
// Emoji Picker Functions
// ============================================

/**
 * Id of the textarea the emoji picker was opened for, or null when closed.
 * @type {?string}
 */
let currentEmojiTarget = null;

/**
 * Opens the emoji picker modal and records which textarea it was opened for.
 * @param {string} targetId - Textarea id.
 * @returns {void}
 */
function showEmojiPicker(targetId) {
    currentEmojiTarget = targetId;
    document.getElementById('emojiModal').classList.add('active');
}

/**
 * Hides the emoji picker and forgets the target textarea.
 * @returns {void}
 */
function closeEmojiModal() {
    document.getElementById('emojiModal').classList.remove('active');
    currentEmojiTarget = null;
}

/**
 * Replaces the current selection in a textarea with an emoji, places the cursor
 * after it, and closes the picker.
 * @param {string} targetId - Textarea id (the emoji buttons in html.go hard-code
 *     'consoleMessage').
 * @param {string} emoji
 * @returns {void}
 */
function insertEmoji(targetId, emoji) {
    const textarea = document.getElementById(targetId);
    if (textarea) {
        const start = textarea.selectionStart;
        const end = textarea.selectionEnd;
        const text = textarea.value;
        const before = text.substring(0, start);
        const after = text.substring(end, text.length);
        textarea.value = before + emoji + after;
        textarea.selectionStart = textarea.selectionEnd = start + emoji.length;
        textarea.focus();
    }
    closeEmojiModal();
}

/**
 * Update emoji buttons to use the current target: rewrites the 'consoleMessage'
 * target in each emoji button's onclick to currentEmojiTarget. Note it runs once at
 * DOMContentLoaded, when currentEmojiTarget is still null, so the rewrite is a no-op
 * and every emoji is inserted into consoleMessage regardless of which picker opened the modal.
 */
document.addEventListener('DOMContentLoaded', function() {
    // Update emoji modal buttons to use current target
    const emojiModal = document.getElementById('emojiModal');
    if (emojiModal) {
        const emojiButtons = emojiModal.querySelectorAll('button[onclick*="insertEmoji"]');
        emojiButtons.forEach(button => {
            const onclick = button.getAttribute('onclick');
            button.setAttribute('onclick', onclick.replace('consoleMessage', currentEmojiTarget || 'consoleMessage'));
        });
    }
});

// Start immediately rather than on DOMContentLoaded: html.go loads this script at
// the end of <body>, so the elements init() touches already exist.
init();
