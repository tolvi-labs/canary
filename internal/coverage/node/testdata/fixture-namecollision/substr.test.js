const { test } = require('node:test');
const assert = require('node:assert');
const { sendsOnce, sendsTwice } = require('./substr.js');

test('sends', () => {
  assert.strictEqual(sendsOnce(), 'once');
});

test('sends twice', () => {
  assert.strictEqual(sendsTwice(), 'twice');
});
