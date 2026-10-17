/**
 * @module qr_code_test
 * @description Unit tests for HomeKit QR code generation
 * Run with: node qr_code_test.js
 *
 * A standalone Node script (not a Jest test: the filename does not match the Jest
 * testMatch patterns). It tests a local copy of calculateHomekitPayload, so it only
 * catches regressions if the copy is kept in sync with script.js by hand. Exits 0
 * when every assertion passes and 1 otherwise.
 */

const fs = require('fs');
const path = require('path');

// Simple test implementations of the QR code functions
// These mirror the implementations in script.js

/**
 * Copy of script.js's calculateHomekitPayload: packs the 8-digit setup code into a
 * BigInt bit field (category 2 = bridge) and returns it as a decimal string, which
 * the UI prefixes with "X-HM://" for the pairing QR code.
 * @param {string} setupCode - Setup code; every non-digit (e.g. dashes) is stripped first.
 * @returns {string} The payload as a base-10 string.
 * @throws {Error} If fewer or more than 8 digits remain after stripping.
 */
function calculateHomekitPayload(setupCode) {
    // Extract digits from setup code (remove dashes and non-digits)
    const digits = setupCode.replace(/[^0-9]/g, '');
    if (digits.length !== 8) {
        throw new Error('Invalid setup code: must be 8 digits');
    }

    const setupValue = parseInt(digits, 10);
    if (setupValue < 0 || setupValue > 99999999) {
        throw new Error('Invalid setup code range');
    }

    // HomeKit setup payload format:
    // Version (3 bits): 0
    // Reserved (4 bits): 0
    // Category (8 bits): 2 (bridge)
    // Flags (8 bits): 0
    // Setup code (27 bits): encoded from 8 digits
    // Reserved (4 bits): 0
    // Reserved (10 bits): 0

    let payload = 0n;

    // Version (3 bits): 0
    payload |= 0n;

    // Reserved (4 bits): 0
    payload <<= 4n;

    // Category (8 bits): 2 (bridge)
    payload |= 2n;
    payload <<= 8n;

    // Flags (8 bits): 0
    payload |= 0n;
    payload <<= 27n; // FIXED: was <<= 8n

    // Setup code (27 bits)
    payload |= BigInt(setupValue);
    payload <<= 4n; // Reserved 4 bits at end

    return payload.toString(10);
}

/**
 * Test suite: runs ten numbered checks of calculateHomekitPayload, prints a pass/fail
 * line per assertion and a summary, then exits the process.
 * @returns {void} Always calls process.exit (0 on success, 1 on any failure).
 */
function runTests() {
    console.log('🧪 Running HomeKit QR Code Tests...\n');

    let passed = 0;
    let failed = 0;

    /**
     * Records and prints one pass/fail result.
     * @param {boolean} condition
     * @param {string} message
     * @returns {void}
     */
    function assert(condition, message) {
        if (condition) {
            console.log('✅ ' + message);
            passed++;
        } else {
            console.log('❌ ' + message);
            failed++;
        }
    }

    /**
     * Passes if fn throws (any error), fails otherwise.
     * @param {Function} fn
     * @param {string} message
     * @returns {void}
     */
    function assertThrows(fn, message) {
        try {
            fn();
            console.log('❌ ' + message + ' (expected to throw)');
            failed++;
        } catch (e) {
            console.log('✅ ' + message);
            passed++;
        }
    }

    // Test 1: Valid setup code acceptance
    try {
        const result = calculateHomekitPayload('12345678');
        assert(typeof result === 'string', 'Valid setup code produces string payload');
        assert(/^\d+$/.test(result), 'Payload is numeric string');
        assert(result.length > 0, 'Payload is not empty');
    } catch (e) {
        assert(false, 'Valid setup code should not throw: ' + e.message);
    }

    // Test 2: Invalid setup code rejection - too short
    assertThrows(() => calculateHomekitPayload('1234567'), 'Rejects setup codes shorter than 8 digits');

    // Test 3: Invalid setup code rejection - too long
    assertThrows(() => calculateHomekitPayload('123456789'), 'Rejects setup codes longer than 8 digits');

    // Test 4: Invalid setup code rejection - non-digits
    assertThrows(() => calculateHomekitPayload('1234567a'), 'Rejects setup codes with non-digit characters');

    // Test 5: Setup code with dashes (should be cleaned)
    try {
        const result = calculateHomekitPayload('123-456-78');
        assert(typeof result === 'string', 'Setup code with dashes produces valid payload');
    } catch (e) {
        assert(false, 'Setup code with dashes should be accepted: ' + e.message);
    }

    // Test 6: Payload is reasonable size
    const testPayload = calculateHomekitPayload('12345678');
    assert(testPayload.length >= 8 && testPayload.length <= 20, 'Payload has reasonable length for 64-bit number');

    // Test 7: Different setup codes produce different payloads
    const payload1 = calculateHomekitPayload('11111111');
    const payload2 = calculateHomekitPayload('22222222');
    assert(payload1 !== payload2, 'Different setup codes produce different payloads');

    // Test 8: Same setup code produces identical payloads
    const payload1Again = calculateHomekitPayload('11111111');
    assert(payload1 === payload1Again, 'Same setup code produces identical payloads');

    // Test 9: Boundary setup codes
    try {
        const minPayload = calculateHomekitPayload('00000000');
        assert(typeof minPayload === 'string', 'Minimum setup code (00000000) accepted');
    } catch (e) {
        assert(false, 'Minimum setup code should be accepted: ' + e.message);
    }

    try {
        const maxPayload = calculateHomekitPayload('99999999');
        assert(typeof maxPayload === 'string', 'Maximum setup code (99999999) accepted');
    } catch (e) {
        assert(false, 'Maximum setup code should be accepted: ' + e.message);
    }

    // Test 10: QR content format
    const setupCode = '12345678';
    const payload = calculateHomekitPayload(setupCode);
    const qrContent = `X-HM://${payload}`;
    assert(qrContent.startsWith('X-HM://'), 'QR content has correct format');
    assert(qrContent.length > 8, 'QR content is properly formatted');

    console.log(`\n📊 Test Results: ${passed} passed, ${failed} failed`);

    if (failed === 0) {
        console.log('🎉 All tests passed!');
        process.exit(0);
    } else {
        console.log('💥 Some tests failed!');
        process.exit(1);
    }
}

// Run the tests
if (require.main === module) {
    runTests();
}

// Exported so other scripts can reuse the payload helper or run the suite.
module.exports = { runTests, calculateHomekitPayload };