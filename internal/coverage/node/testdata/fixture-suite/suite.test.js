const { describe, it, test } = require('node:test');
const assert = require('node:assert');
const { alpha, beta, gamma } = require('./slib.js');

describe('outer suite', () => {
  it('covers alpha', () => { assert.strictEqual(alpha(), 'a'); });
  it('covers beta', () => { assert.strictEqual(beta(), 'b'); });
});

test('covers gamma', () => { assert.strictEqual(gamma(), 'g'); });
