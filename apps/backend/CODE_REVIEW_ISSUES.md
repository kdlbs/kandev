# Backend Code Review Issues

Generated: 2026-01-11

## High Priority

### 1. Security TODOs
- [ ] `internal/gateway/websocket/handler.go:20` - TODO: In production, validate origin
- [ ] `internal/gateway/websocket/handler.go:46` - TODO: Implement proper JWT validation

### 2. Deprecated API
- [ ] `internal/agent/docker/client.go:300-301` - `inspect.NetworkSettings.IPAddress` is deprecated (removed in Docker v29). Use `NetworkSettings.Networks` instead.

## Medium Priority

### 3. Unused Functions
- [ ] `internal/agent/lifecycle/manager.go:641` - `updateInstanceComplete` is unused
- [ ] `internal/agent/streaming/reader.go:232` - `marshalMessage` is unused
- [ ] `internal/task/repository/sqlite.go:617` - `scanTasks` is unused

### 4. Unused Field / Race Condition
- [ ] `internal/gateway/websocket/client.go:36` - `mu sync.RWMutex` field is unused. Either use it to protect `subscriptions` or remove it.

### 5. Outdated Comment
- [ ] `internal/agent/agentctl/acp.go:108` - Comment references `needs_input` which was removed. Update to: `// The StopReason indicates why the agent stopped (e.g., "end_turn")`

## Low Priority

### 6. Unused Test Helpers
- [ ] `internal/agent/lifecycle/manager_test.go:130` - `testManager` type unused
- [ ] `internal/agent/lifecycle/manager_test.go:135` - `newTestManager` function unused

### 7. Code Style (staticcheck S1000)
- [ ] `internal/agentctl/api/acp.go:215` - Use `for range ch` instead of `for { select {} }`
- [ ] `internal/agentctl/api/acp.go:336` - Use `for range ch` instead of `for { select {} }`

### 8. Hardcoded Sleep
- [ ] `internal/agent/lifecycle/manager.go:253` - `time.Sleep(500 * time.Millisecond)` - Consider proper ready-check mechanism

### 9. Ignored Error
- [ ] `internal/task/repository/sqlite.go:626` - `_ = json.Unmarshal(...)` ignores error

## Questions / Review Needed

### 10. InputRequestHandler Redundancy
Files: `internal/orchestrator/service.go` (lines 44-45, 71, 387-389, 507-512), `cmd/kandev/main.go` (lines 305-363)

The `InputRequestHandler` and `handleInputRequired` functionality still exists using `input_required` message type. Is this still needed given the new comment-based conversation flow?

### 11. Other TODOs
- [ ] `internal/gateway/websocket/hub.go:127` - TODO: Add topic-based routing for task-specific notifications

