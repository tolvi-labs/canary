const { test } = require('node:test');
const { add } = require('./no-such-module.js');

test('this test never runs', () => {
  add(1, 2);
});
