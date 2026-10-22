/**
 * @module jest.config
 * @description Default Jest configuration (used by a bare `jest` run). Runs the web
 * UI tests in pkg/web/static/__tests__/ under the plain Node environment, with
 * jest.setup.js polyfilling globals. Most DOM tests build their own JSDOM instance,
 * but tag-precedence.test.js needs a global localStorage and so only passes under
 * jest.web.config.json, the jsdom-environment alternative.
 * @type {import('jest').Config}
 */
module.exports = {
  testEnvironment: 'node',
  setupFiles: ['<rootDir>/jest.setup.js'],
  testMatch: [
    '<rootDir>/pkg/web/static/__tests__/**/*.test.js'
  ]
};
