/* eslint-env jest */
/**
 * @module tag-handler.test
 * @description Unit tests for applyTagSelection in ../tag-handler.js.
 */
const { applyTagSelection } = require('../tag-handler.js');

/**
 * applyTagSelection returns the right query string and storage action.
 */
describe('applyTagSelection', () => {
  /**
   * A non-empty selection sets ?tag= and asks for a storage set.
   */
  test('sets tag and returns newSearch and storage set action', () => {
    const res = applyTagSelection('garage', '');
    expect(res).toBeDefined();
    expect(res.newSearch).toMatch(/tag=garage/);
    expect(res.storage).toEqual({ action: 'set', key: 'alarm-selected-tag', value: 'garage' });
  });

  /**
   * An empty selection removes the tag param and asks for a storage remove.
   */
  test('clears tag and returns newSearch without tag and storage remove action', () => {
    const res = applyTagSelection('', '?tag=garage&foo=1');
    expect(res).toBeDefined();
    expect(res.newSearch).not.toMatch(/tag=/);
    expect(res.storage).toEqual({ action: 'remove', key: 'alarm-selected-tag' });
  });
});
