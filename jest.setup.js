/**
 * @module jest.setup
 * @description Jest setup: polyfills missing globals for JSDOM/whatwg-url.
 * Listed in setupFiles of both jest.config.js and jest.web.config.json, so it runs
 * before each test file. It fills in TextEncoder/TextDecoder and crypto when the
 * environment lacks them, and sets global.__JEST__.
 */

// TextEncoder/TextDecoder are provided by Node's util in many versions
try {
    const util = require('util');
    if (typeof global.TextEncoder === 'undefined' && util.TextEncoder) {
        global.TextEncoder = util.TextEncoder;
    }
    if (typeof global.TextDecoder === 'undefined' && util.TextDecoder) {
        global.TextDecoder = util.TextDecoder;
    }
} catch (e) {
    // ignore if util is unavailable
}

// Ensure crypto.subtle exists for libraries that expect Web Crypto (optional)
if (typeof global.crypto === 'undefined') {
    try {
        const { webcrypto } = require('crypto');
        global.crypto = webcrypto || {};
    } catch (e) {
        global.crypto = {};
    }
}

// Fallback minimal TextEncoder/TextDecoder implementations for older Node versions
if (typeof global.TextEncoder === 'undefined') {
    /**
     * Minimal UTF-8-only TextEncoder built on Buffer.
     */
    class TextEncoderPolyfill {
        /**
         * Encodes a string as UTF-8.
         * @param {string} [str='']
         * @returns {Uint8Array}
         */
        encode(str = '') {
            // Buffer.from produces a Node Buffer, convert to Uint8Array
            return new Uint8Array(Buffer.from(String(str), 'utf8'));
        }
    }
    global.TextEncoder = TextEncoderPolyfill;
}

if (typeof global.TextDecoder === 'undefined') {
    /**
     * Minimal TextDecoder built on Buffer; always decodes as UTF-8 whatever the
     * encoding argument says.
     */
    class TextDecoderPolyfill {
        /**
         * @param {string} [encoding='utf-8'] - Stored only; not used for decoding.
         */
        constructor(encoding = 'utf-8') { this.encoding = encoding; }
        /**
         * Decodes bytes as UTF-8.
         * @param {(Uint8Array|number[]|*)} buf - Anything else is passed through String().
         * @returns {string}
         */
        decode(buf) {
            if (buf instanceof Uint8Array || Array.isArray(buf)) {
                return Buffer.from(buf).toString('utf8');
            }
            return String(buf);
        }
    }
    global.TextDecoder = TextDecoderPolyfill;
}

// Mark that we're running under Jest so the app can short-circuit DOM-heavy
// initialization during unit/integration tests.
global.__JEST__ = true;
