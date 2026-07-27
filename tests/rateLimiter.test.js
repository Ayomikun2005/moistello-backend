import assert from 'node:assert';
import { describe, it, beforeEach } from 'node:test';
import { rateLimitMiddleware } from '../src/middleware/rateLimiter.js';
import redisService from '../src/services/redisService.js';

/**
 * Helper: builds a minimal Express-style req/res/next triple.
 * Returns { req, res, next, state } where state tracks calls.
 */
function makeContext({ ip, headers = {} } = {}) {
  const state = { statusCode: null, body: null, nextCalled: false };

  const req = {
    ip: ip || null,
    headers,
  };

  const res = {
    status(code) {
      state.statusCode = code;
      return res;
    },
    json(data) {
      state.body = data;
      return res;
    },
  };

  const next = () => {
    state.nextCalled = true;
  };

  return { req, res, next, state };
}

/**
 * Helper: creates a mock Redis client whose incr() returns the supplied count
 * and whose expire() is a no-op (both async).
 */
function makeMockRedis({ incrValue = 1, throwError = null } = {}) {
  const calls = { incr: [], expire: [] };

  return {
    calls,
    incr: async (key) => {
      calls.incr.push(key);
      if (throwError) throw throwError;
      return incrValue;
    },
    expire: async (key, ttl) => {
      calls.expire.push({ key, ttl });
      return 1;
    },
  };
}

describe('rateLimitMiddleware', () => {
  // Reset redisService.client before each test so tests are independent
  beforeEach(() => {
    redisService.client = null;
  });

  // ──────────────────────────────────────────────────────────────
  // 1. Fail-closed: deny 503 when Redis client is null
  // ──────────────────────────────────────────────────────────────
  it('denies 503 when Redis client is null (fail-closed)', async () => {
    redisService.client = null; // Explicitly null

    const { req, res, next, state } = makeContext({ ip: '10.0.0.1' });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(state.statusCode, 503, 'Expected HTTP 503');
    assert.strictEqual(state.body.error, 'Service Temporarily Unavailable');
    assert.strictEqual(state.nextCalled, false, 'next() must not be called');
  });

  // ──────────────────────────────────────────────────────────────
  // 2. Allow request when count is below threshold (incr returns 1)
  // ──────────────────────────────────────────────────────────────
  it('allows request when count is below threshold (incr returns 1)', async () => {
    const mockRedis = makeMockRedis({ incrValue: 1 });
    redisService.client = mockRedis;

    const { req, res, next, state } = makeContext({ ip: '10.0.0.2' });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(state.nextCalled, true, 'next() should be called');
    assert.strictEqual(state.statusCode, null, 'No HTTP status set on success');
  });

  // ──────────────────────────────────────────────────────────────
  // 3. Allow request when count is at maximum (incr returns 100)
  // ──────────────────────────────────────────────────────────────
  it('allows request when count equals MAX_REQUESTS_PER_WINDOW (100)', async () => {
    const mockRedis = makeMockRedis({ incrValue: 100 });
    redisService.client = mockRedis;

    const { req, res, next, state } = makeContext({ ip: '10.0.0.3' });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(state.nextCalled, true, 'Exactly at limit should still pass');
    assert.strictEqual(state.statusCode, null);
  });

  // ──────────────────────────────────────────────────────────────
  // 4. Deny 429 when count exceeds MAX_REQUESTS_PER_WINDOW (101)
  // ──────────────────────────────────────────────────────────────
  it('denies 429 when count exceeds MAX_REQUESTS_PER_WINDOW (101)', async () => {
    const mockRedis = makeMockRedis({ incrValue: 101 });
    redisService.client = mockRedis;

    const { req, res, next, state } = makeContext({ ip: '10.0.0.4' });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(state.statusCode, 429, 'Expected HTTP 429');
    assert.strictEqual(state.body.error, 'Too Many Requests');
    assert.strictEqual(state.nextCalled, false, 'next() must not be called');
  });

  // ──────────────────────────────────────────────────────────────
  // 5. Deny 429 for higher counts well above the limit
  // ──────────────────────────────────────────────────────────────
  it('denies 429 when count is well above limit (e.g. 500)', async () => {
    const mockRedis = makeMockRedis({ incrValue: 500 });
    redisService.client = mockRedis;

    const { req, res, next, state } = makeContext({ ip: '10.0.0.5' });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(state.statusCode, 429);
    assert.ok(state.body.message, 'Response body should contain a message');
    assert.strictEqual(state.nextCalled, false);
  });

  // ──────────────────────────────────────────────────────────────
  // 6. Sets TTL on first request (count === 1, verify expire called)
  // ──────────────────────────────────────────────────────────────
  it('sets TTL on first request (count === 1, expire is called)', async () => {
    const mockRedis = makeMockRedis({ incrValue: 1 });
    redisService.client = mockRedis;

    const ip = '10.0.0.6';
    const { req, res, next } = makeContext({ ip });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(mockRedis.calls.expire.length, 1, 'expire() should be called once');
    assert.strictEqual(
      mockRedis.calls.expire[0].key,
      `ratelimit:${ip}`,
      'expire key should match the rate limit key for this IP'
    );
    assert.ok(
      mockRedis.calls.expire[0].ttl > 0,
      'TTL value should be positive'
    );
  });

  // ──────────────────────────────────────────────────────────────
  // 7. Does NOT call expire on subsequent requests (count > 1)
  // ──────────────────────────────────────────────────────────────
  it('does not set TTL when count is greater than 1 (key already has TTL)', async () => {
    const mockRedis = makeMockRedis({ incrValue: 50 });
    redisService.client = mockRedis;

    const { req, res, next, state } = makeContext({ ip: '10.0.0.7' });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(mockRedis.calls.expire.length, 0, 'expire() must not be called after first request');
    assert.strictEqual(state.nextCalled, true);
  });

  // ──────────────────────────────────────────────────────────────
  // 8. Deny 503 on Redis operation error (throw from incr)
  // ──────────────────────────────────────────────────────────────
  it('denies 503 on Redis operation error (incr throws)', async () => {
    const redisError = new Error('ECONNREFUSED: Redis connection refused');
    const mockRedis = makeMockRedis({ throwError: redisError });
    redisService.client = mockRedis;

    const { req, res, next, state } = makeContext({ ip: '10.0.0.8' });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(state.statusCode, 503, 'Expected HTTP 503 on Redis error');
    assert.strictEqual(state.body.error, 'Service Temporarily Unavailable');
    assert.strictEqual(state.nextCalled, false, 'next() must not be called on error');
  });

  // ──────────────────────────────────────────────────────────────
  // 9. Handles x-forwarded-for IP header
  // ──────────────────────────────────────────────────────────────
  it('uses x-forwarded-for header when req.ip is not set', async () => {
    const mockRedis = makeMockRedis({ incrValue: 1 });
    redisService.client = mockRedis;

    const forwardedIp = '203.0.113.42';
    const { req, res, next, state } = makeContext({
      ip: null, // no direct IP
      headers: { 'x-forwarded-for': forwardedIp },
    });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(state.nextCalled, true, 'next() should be called');
    // The incr key should be built from the forwarded IP
    assert.ok(
      mockRedis.calls.incr[0].includes(forwardedIp),
      `incr key "${mockRedis.calls.incr[0]}" should contain the forwarded IP "${forwardedIp}"`
    );
  });

  // ──────────────────────────────────────────────────────────────
  // 10. Falls back to 'unknown_ip' when no IP is available
  // ──────────────────────────────────────────────────────────────
  it('falls back to unknown_ip when no IP information is present', async () => {
    const mockRedis = makeMockRedis({ incrValue: 1 });
    redisService.client = mockRedis;

    const { req, res, next, state } = makeContext({
      ip: null,
      headers: {}, // no x-forwarded-for
    });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(state.nextCalled, true);
    assert.ok(
      mockRedis.calls.incr[0].includes('unknown_ip'),
      'Key should fall back to unknown_ip'
    );
  });

  // ──────────────────────────────────────────────────────────────
  // 11. incr is called with the correct Redis key format
  // ──────────────────────────────────────────────────────────────
  it('calls incr with the correct ratelimit key format', async () => {
    const mockRedis = makeMockRedis({ incrValue: 5 });
    redisService.client = mockRedis;

    const ip = '192.168.1.100';
    const { req, res, next } = makeContext({ ip });
    await rateLimitMiddleware(req, res, next);

    assert.strictEqual(mockRedis.calls.incr.length, 1);
    assert.strictEqual(mockRedis.calls.incr[0], `ratelimit:${ip}`);
  });
});
