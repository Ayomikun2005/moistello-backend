/**
 * tests/gracefulShutdown.test.js
 *
 * Tests for the Node.js graceful-shutdown implementation in src/server.js.
 *
 * Run with:
 *   node --test tests/gracefulShutdown.test.js
 *
 * Uses node:test and node:assert only (no external test framework).
 * Redis is stubbed — no real Redis instance is required.
 */

import assert from 'node:assert/strict';
import http from 'node:http';
import test from 'node:test';
import { startServer, gracefulShutdown } from '../src/server.js';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Find a free port by letting the OS assign one, then close that server.
 * @returns {Promise<number>}
 */
function getFreePort() {
  return new Promise((resolve, reject) => {
    const tmp = http.createServer();
    tmp.listen(0, '127.0.0.1', () => {
      const port = tmp.address().port;
      tmp.close(() => resolve(port));
    });
    tmp.once('error', reject);
  });
}

/**
 * Wait for `ms` milliseconds.
 * @param {number} ms
 */
function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// ---------------------------------------------------------------------------
// Test 1 — server starts and listens on the requested port
// ---------------------------------------------------------------------------

test('startServer() listens on the given port', async () => {
  const port = await getFreePort();
  const server = await startServer(port);

  try {
    assert.ok(server.listening, 'server should be in listening state');
    const addr = server.address();
    assert.strictEqual(addr.port, port, 'server should bind to the requested port');
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});

// ---------------------------------------------------------------------------
// Test 2 — gracefulShutdown() closes the server
// ---------------------------------------------------------------------------

test('gracefulShutdown() stops the server from accepting new connections', async (t) => {
  // Stub redisService.disconnect so gracefulShutdown does not need real Redis
  const redisServiceModule = await import('../src/services/redisService.js');
  const redisService = redisServiceModule.default;

  let disconnectCalled = false;
  redisService.disconnect = async () => { disconnectCalled = true; };

  const port = await getFreePort();
  const server = await startServer(port);

  assert.ok(server.listening, 'server should be listening before shutdown');

  await gracefulShutdown(server);

  assert.ok(!server.listening, 'server should no longer be listening after shutdown');

  // Reset stub
  t.after(() => { delete redisService.disconnect; });
});

// ---------------------------------------------------------------------------
// Test 3 — active connections are drained before shutdown resolves
// ---------------------------------------------------------------------------

test('gracefulShutdown() drains active connections before resolving', async (t) => {
  const redisServiceModule = await import('../src/services/redisService.js');
  const redisService = redisServiceModule.default;
  redisService.disconnect = async () => {};

  const port = await getFreePort();

  // Use a slow request handler to simulate an in-flight connection
  let requestReceived = false;
  let respondFn;
  const respondPromise = new Promise((resolve) => { respondFn = resolve; });

  const server = await startServer(port, (req, res) => {
    requestReceived = true;
    // Hold the response open until the test explicitly releases it
    respondPromise.then(() => {
      res.writeHead(200);
      res.end('done');
    });
  });

  // Open a connection and start a request (but do NOT wait for the response yet)
  const reqPromise = new Promise((resolve, reject) => {
    const req = http.request({ host: '127.0.0.1', port, path: '/' }, (res) => {
      res.on('data', () => {});
      res.on('end', resolve);
    });
    req.on('error', reject);
    req.end();
  });

  // Give the server time to receive the request
  await delay(50);
  assert.ok(requestReceived, 'server should have received the in-flight request');

  // Start shutdown in the background — it must NOT resolve until we release the response
  let shutdownDone = false;
  const shutdownPromise = gracefulShutdown(server).then(() => { shutdownDone = true; });

  // A short delay — shutdown should still be waiting for the connection to drain
  await delay(100);
  assert.strictEqual(shutdownDone, false, 'shutdown should still be waiting for the active connection');

  // Release the in-flight request
  respondFn();
  await reqPromise;

  // Now shutdown should complete
  await shutdownPromise;
  assert.strictEqual(shutdownDone, true, 'shutdown should complete once all connections are drained');

  t.after(() => { delete redisService.disconnect; });
});

// ---------------------------------------------------------------------------
// Test 4 — Redis disconnect is called during shutdown
// ---------------------------------------------------------------------------

test('gracefulShutdown() calls redisService.disconnect()', async (t) => {
  const redisServiceModule = await import('../src/services/redisService.js');
  const redisService = redisServiceModule.default;

  let disconnectCallCount = 0;
  redisService.disconnect = async () => { disconnectCallCount += 1; };

  const port = await getFreePort();
  const server = await startServer(port);

  await gracefulShutdown(server);

  assert.strictEqual(disconnectCallCount, 1, 'redisService.disconnect should be called exactly once');

  t.after(() => { delete redisService.disconnect; });
});

// ---------------------------------------------------------------------------
// Test 5 — shutdown completes within the timeout window
// ---------------------------------------------------------------------------

test('gracefulShutdown() completes within the configured timeout', async (t) => {
  const redisServiceModule = await import('../src/services/redisService.js');
  const redisService = redisServiceModule.default;
  redisService.disconnect = async () => {};

  const port = await getFreePort();
  const server = await startServer(port);

  const start = Date.now();
  await gracefulShutdown(server);
  const elapsed = Date.now() - start;

  // Default SHUTDOWN_TIMEOUT_MS is 30 000 ms.
  // A clean shutdown with no active connections should finish well under 2 s.
  assert.ok(
    elapsed < 2000,
    `shutdown should complete quickly when there are no active connections (took ${elapsed} ms)`,
  );

  t.after(() => { delete redisService.disconnect; });
});

// ---------------------------------------------------------------------------
// Test 6 — SIGTERM signal triggers graceful shutdown
// ---------------------------------------------------------------------------

test('process receives SIGTERM and calls gracefulShutdown()', async () => {
  // We test signal wiring without actually sending SIGTERM to the current
  // process (which would terminate the test runner).  Instead we verify that
  // startServer() registers a SIGTERM handler and that invoking it shuts the
  // server down cleanly.

  const redisServiceModule = await import('../src/services/redisService.js');
  const redisService = redisServiceModule.default;

  let disconnectCalled = false;
  redisService.disconnect = async () => { disconnectCalled = true; };

  const port = await getFreePort();

  // Prevent process.exit() from actually killing the test runner
  const originalExit = process.exit;
  let exitCode;
  process.exit = (code) => { exitCode = code; };

  const server = await startServer(port);

  assert.ok(server.listening, 'server must be listening before signal test');

  // Retrieve and invoke the SIGTERM listener that startServer() registered
  const sigtermListeners = process.listeners('SIGTERM');
  assert.ok(sigtermListeners.length > 0, 'startServer() should register a SIGTERM listener');

  const sigtermHandler = sigtermListeners[sigtermListeners.length - 1];

  // Remove our listener so we can safely call it without re-entry issues
  process.removeListener('SIGTERM', sigtermHandler);

  await sigtermHandler();

  assert.ok(!server.listening, 'server should stop listening after SIGTERM handler runs');
  assert.strictEqual(exitCode, 0, 'process.exit(0) should be called on clean shutdown');
  assert.ok(disconnectCalled, 'Redis disconnect should be called during SIGTERM shutdown');

  // Restore process.exit
  process.exit = originalExit;
  delete redisService.disconnect;
});
