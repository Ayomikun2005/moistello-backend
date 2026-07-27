import assert from 'node:assert/strict';
import test from 'node:test';
import redisService from '../src/services/redisService.js';

function makeClient(overrides = {}) {
  return {
    get: async () => 'value',
    set: async () => 'OK',
    del: async () => 1,
    quit: async () => 'OK',
    ...overrides,
  };
}

test('connect stores a client and marks the service alive', async () => {
  const client = makeClient();
  await redisService.connect(client);

  assert.equal(redisService.isAlive(), true);
  assert.equal(redisService.client, client);

  await redisService.disconnect();
});

test('disconnect clears the client reference', async () => {
  const client = makeClient();
  await redisService.connect(client);
  await redisService.disconnect();

  assert.equal(redisService.client, null);
  assert.equal(redisService.isAlive(), false);
});

test('get/set/del delegate to the underlying Redis client', async () => {
  const calls = [];
  const client = makeClient({
    get: async (key) => {
      calls.push(['get', key]);
      return 'value';
    },
    set: async (key, value, ...rest) => {
      calls.push(['set', key, value, ...rest]);
      return 'OK';
    },
    del: async (...keys) => {
      calls.push(['del', ...keys]);
      return keys.length;
    },
  });

  await redisService.connect(client);

  assert.equal(await redisService.get('foo'), 'value');
  assert.equal(await redisService.set('foo', 'bar', 30), 'OK');
  assert.equal(await redisService.del('foo', 'bar'), 2);
  assert.deepEqual(calls, [
    ['get', 'foo'],
    ['set', 'foo', 'bar', 'EX', 30],
    ['del', 'foo', 'bar'],
  ]);

  await redisService.disconnect();
});

test('operations fail with descriptive errors when the service is disconnected', async () => {
  await redisService.disconnect();

  await assert.rejects(() => redisService.get('foo'), /RedisService.get/);
  await assert.rejects(() => redisService.set('foo', 'bar'), /RedisService.set/);
  await assert.rejects(() => redisService.del('foo'), /RedisService.del/);
});

test('connect wraps factory errors and leaves the client unset', async () => {
  await assert.rejects(() => redisService.connect(async () => {
    throw new Error('boom');
  }), /RedisService.connect/);

  assert.equal(redisService.client, null);
});
