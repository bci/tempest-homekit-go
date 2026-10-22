/**
 * @module alarm-utils.test
 * @description Unit tests for getTriggeredBadgeClass in ../alarm-utils.js.
 */
const { getTriggeredBadgeClass } = require('../alarm-utils');

/**
 * Badge class thresholds: 0/invalid -> ok, 1-5 -> warn, >5 -> critical.
 */
describe('getTriggeredBadgeClass', () => {
  /**
   * 0, undefined and null all map to the ok class.
   */
  test('returns ok for 0 or invalid', () => {
    expect(getTriggeredBadgeClass(0)).toBe('alarm-triggered-ok');
    expect(getTriggeredBadgeClass()).toBe('alarm-triggered-ok');
    expect(getTriggeredBadgeClass(null)).toBe('alarm-triggered-ok');
  });

  /**
   * A count within 1-5 maps to the warn class.
   */
  test('returns warn for 3', () => {
    expect(getTriggeredBadgeClass(3)).toBe('alarm-triggered-warn');
  });

  /**
   * A count above 5 maps to the critical class.
   */
  test('returns critical for 10', () => {
    expect(getTriggeredBadgeClass(10)).toBe('alarm-triggered-critical');
  });
});
