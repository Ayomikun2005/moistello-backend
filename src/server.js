/**
 * src/server.js
 *
 * Node.js HTTP server with graceful shutdown support.
 *
 * Mirrors the pattern already used on the Go side (internal/api/server.go):
 *   - Listens for SIGTERM / SIGINT
 *   - Stops accepting new connections
 *   - Waits for active connections to drain
 *   - Closes Redis cleanly
 *   - Honours a configurable timeout (SHUTDOWN_TIMEOUT_MS env var, default 30 s)
 *
 * Usage:
 *   import { startServer } from './server.js';
 *   const server = await startServer(3000);
 */

import http from 'node:http';
import redisService from './services/redisService.js';
import { openapiHandler } from './middleware/openapi.js';

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

/** Shutdown timeout in milliseconds (default: 30 000 ms). */
const SHUTDOWN_TIMEOUT_MS = parseInt(process.env.SHUTDOWN_TIMEOUT_MS ?? '30000', 10);

// ---------------------------------------------------------------------------
// Connection tracking
// ---------------------------------------------------------------------------

/** Set of all currently open sockets so we can forcibly destroy them on timeout. */
const activeConnections = new Set();

/**
 * Attach connection tracking to a server instance.
 * @param {http.Server} server
 */
function trackConnections(server) {
  server.on('connection', (socket) => {
    activeConnections.add(socket);
    socket.once('close', () => activeConnections.delete(socket));
  });
}

// ---------------------------------------------------------------------------
// Graceful shutdown
// ---------------------------------------------------------------------------

/**
 * Perform a graceful shutdown of the given HTTP server.
 *
 * Steps:
 *  1. Log that shutdown has started.
 *  2. Call server.close() to stop accepting new connections.
 *  3. Wait for all active connections to drain (up to SHUTDOWN_TIMEOUT_MS).
 *  4. Close the Redis connection.
 *  5. Log completion and resolve.
 *
 * @param {http.Server} server - The HTTP server to shut down.
 * @returns {Promise<void>}
 */
export async function gracefulShutdown(server) {
  console.log('[server] graceful shutdown initiated');

  // Resolve once server.close() callback fires (all connections drained)
  // or reject when the timeout fires, whichever comes first.
  const drainPromise = new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      console.warn('[server] shutdown timeout reached — forcibly destroying remaining connections');
      for (const socket of activeConnections) {
        socket.destroy();
      }
      reject(new Error(`Graceful shutdown timed out after ${SHUTDOWN_TIMEOUT_MS} ms`));
    }, SHUTDOWN_TIMEOUT_MS);

    // Make the timer non-blocking so the process can exit normally.
    if (timer.unref) timer.unref();

    console.log('[server] draining active connections…');
    server.close((err) => {
      clearTimeout(timer);
      if (err) {
        reject(err);
      } else {
        resolve();
      }
    });
  });

  try {
    await drainPromise;
    console.log('[server] all connections drained');
  } catch (err) {
    console.error('[server] connection drain error:', err.message);
    // Continue shutdown even if drain timed out — we already destroyed sockets above.
  }

  // Close Redis connection
  console.log('[server] closing Redis connection…');
  try {
    if (redisService && typeof redisService.disconnect === 'function') {
      await redisService.disconnect();
    } else if (redisService && redisService.client && typeof redisService.client.quit === 'function') {
      await redisService.client.quit();
    }
    console.log('[server] Redis connection closed');
  } catch (err) {
    console.error('[server] Redis close error:', err.message);
  }

  console.log('[server] graceful shutdown complete');
}

// ---------------------------------------------------------------------------
// Server factory
// ---------------------------------------------------------------------------

/**
 * Create and start the HTTP server on the specified port.
 *
 * Registers SIGTERM and SIGINT handlers that trigger graceful shutdown.
 *
 * @param {number} port - TCP port to listen on.
 * @param {http.RequestListener} [requestListener] - Optional request handler.
 *   Pass your Express/Fastify app here; if omitted a minimal handler is used.
 * @returns {Promise<http.Server>} Resolves once the server is listening.
 */
export function startServer(port, requestListener) {
  const handler = requestListener ?? ((req, res) => {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ status: 'ok' }));
  });

  const server = http.createServer((req, res) => {
    const pathname = new URL(req.url ?? '/', `http://${req.headers.host || 'localhost'}`).pathname;
    if (pathname === '/api/docs' || pathname === '/api/docs/' || pathname.startsWith('/api/docs/')) {
      return openapiHandler(req, res);
    }
    return handler(req, res);
  });

  // Track every socket that opens so we can drain / destroy them on shutdown.
  trackConnections(server);

  // Register signal handlers exactly once per server instance.
  const shutdown = async (signal) => {
    console.log(`[server] received ${signal} — starting graceful shutdown`);
    try {
      await gracefulShutdown(server);
      process.exit(0);
    } catch (err) {
      console.error('[server] shutdown failed:', err.message);
      process.exit(1);
    }
  };

  process.once('SIGTERM', () => shutdown('SIGTERM'));
  process.once('SIGINT',  () => shutdown('SIGINT'));

  return new Promise((resolve, reject) => {
    server.listen(port, () => {
      const addr = server.address();
      console.log(`[server] listening on port ${addr?.port ?? port}`);
      resolve(server);
    });

    server.once('error', reject);
  });
}
