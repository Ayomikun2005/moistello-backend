# Known Race Findings

This document tracks race-detector findings in the codebase. The CI pipeline
runs `go test -race ./...` on every PR and push to main.

## Fixed Races

### `Hub.Broadcast` map iteration race (fixed in #254)

**Location**: `internal/websocket/hub.go` — `Broadcast()` method

**Problem**: `Broadcast()` released `RLock` before iterating the `room` map,
creating a race with `LeaveRoom()` and `Unregister()` which modify the same
map under a write lock. The Go race detector flagged concurrent read-write
access to the room's `map[string]*Client`.

**Fix**: Copy the room's client slice while holding the read lock, then iterate
the copy after releasing the lock. This eliminates the shared-map contention
while keeping the lock duration minimal.

```go
// Before (racy):
h.mu.RLock()
room, ok := h.rooms[circleID]
h.mu.RUnlock()
for _, client := range room { ... }

// After (safe):
h.mu.RLock()
clients := make([]*Client, 0, len(room))
for _, c := range room { clients = append(clients, c) }
h.mu.RUnlock()
for _, client := range clients { ... }
```

## Known-Benign Patterns

### `Client.missedPings` — `sync/atomic.Int32`

The `missedPings` counter on `Client` uses `atomic.Int32`, which is inherently
race-free. Multiple goroutines (ReadPump, WritePump, pong handler) may read
and write it concurrently, but `atomic` operations guarantee correctness.

### Webhook `Dispatcher.deliverWithRetry` goroutines

The webhook dispatcher launches goroutines in `DispatchPayload` that share the
`body` byte slice. This is safe because `body` is never modified after
creation — all goroutines perform read-only access.

## Running the Race Detector Locally

```bash
# Run all tests with race detection
go test ./... -race -count=1

# Run a specific package
go test ./internal/websocket/... -race -v -count=1

# Run the concurrent stress tests
go test ./internal/websocket/... -race -run TestHub_Concurrent -v
```

> **Note**: The race detector significantly slows execution (typically 2-10x)
> and increases memory usage. It is enabled in CI but optional for local
> development.
