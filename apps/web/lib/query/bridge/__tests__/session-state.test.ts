import { describe, it, expect, beforeEach } from "vitest";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { registerSessionStateBridge } from "../session-state";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "@/lib/query/keys";
import type { TaskSession, TaskSessionsResponse } from "@/lib/types/http";

// ---------------------------------------------------------------------------
// WS event name constants
// ---------------------------------------------------------------------------

const STATE_CHANGED = "session.state_changed";
const AGENTCTL_STARTING = "session.agentctl_starting";
const AGENTCTL_READY = "session.agentctl_ready";

const TASK_ID = "task-1";
const SESSION_ID = "sess-1";
const ENV_ID = "env-1";

// ---------------------------------------------------------------------------
// Fake WebSocket client
// ---------------------------------------------------------------------------
type Handler = (message: { payload: Record<string, unknown>; timestamp?: string }) => void;

function makeFakeWs() {
  const handlers = new Map<string, Set<Handler>>();
  return {
    on(event: string, handler: Handler) {
      let set = handlers.get(event);
      if (!set) {
        set = new Set();
        handlers.set(event, set);
      }
      set.add(handler);
      return () => set?.delete(handler);
    },
    emit(event: string, payload: Record<string, unknown>, timestamp?: string) {
      const set = handlers.get(event);
      if (!set) return;
      for (const h of set) h({ payload, timestamp });
    },
  };
}

// ---------------------------------------------------------------------------
// Shared setup
// ---------------------------------------------------------------------------

type TestContext = {
  ws: ReturnType<typeof makeFakeWs>;
  qc: ReturnType<typeof createTestQueryClient>;
  envMappings: Array<[string, string]>;
  cleanup: () => void;
};

function makeContext(): TestContext {
  const ws = makeFakeWs();
  const qc = createTestQueryClient();
  const envMappings: Array<[string, string]> = [];
  const cleanup = registerSessionStateBridge(ws as unknown as WebSocketClient, qc, {
    setEnvMapping: (sessionId, environmentId) => envMappings.push([sessionId, environmentId]),
  });
  return { ws, qc, envMappings, cleanup };
}

describe("session-state bridge — agentctl snapshot path", () => {
  let ctx: TestContext;
  beforeEach(() => {
    ctx = makeContext();
    return () => ctx.cleanup();
  });

  it("state_changed seeds the by-id + by-task caches with task_id", () => {
    ctx.ws.emit(STATE_CHANGED, {
      task_id: TASK_ID,
      session_id: SESSION_ID,
      new_state: "RUNNING",
    });

    const byId = ctx.qc.getQueryData<TaskSession | null>(qk.taskSession.byId(SESSION_ID));
    expect(byId?.state).toBe("RUNNING");
    expect(byId?.task_id).toBe(TASK_ID);

    const byTask = ctx.qc.getQueryData<TaskSessionsResponse>(qk.taskSession.byTask(TASK_ID));
    expect(byTask?.sessions.map((s) => s.id)).toContain(SESSION_ID);
  });

  // Regression for the WS-accounting `cache-unchanged` drop: the snapshot/replay
  // agentctl event (sent on subscribe/focus) omits task_id. The bridge must
  // derive it from the already-cached session (seeded by the preceding
  // state_changed in the same snapshot batch) and still write the cache — or the
  // bridge audit flags it as "received but never mutated the TQ cache".
  it("agentctl_ready WITHOUT task_id derives it from the cached session and writes env mapping", () => {
    // Pre-seed via state_changed (carries task_id), as the backend snapshot does.
    ctx.ws.emit(STATE_CHANGED, {
      task_id: TASK_ID,
      session_id: SESSION_ID,
      new_state: "RUNNING",
    });

    let writes = 0;
    const unsub = ctx.qc.getQueryCache().subscribe((e) => {
      if (e.type === "updated") writes += 1;
    });

    // Snapshot agentctl event: session_id + env/worktree fields, NO task_id.
    ctx.ws.emit(AGENTCTL_READY, {
      session_id: SESSION_ID,
      task_environment_id: ENV_ID,
      worktree_path: "/work/wt",
      worktree_branch: "feat/x",
    });
    unsub();

    // The merge landed in the by-id cache (task_id preserved, worktree applied).
    const byId = ctx.qc.getQueryData<TaskSession | null>(qk.taskSession.byId(SESSION_ID));
    expect(byId?.task_id).toBe(TASK_ID);
    expect(byId?.worktree_path).toBe("/work/wt");
    expect(byId?.worktree_branch).toBe("feat/x");
    expect(byId?.task_environment_id).toBe(ENV_ID);

    // D6 env mapping side-effect fired (read by getEnvKey for git keys).
    expect(ctx.envMappings).toContainEqual([SESSION_ID, ENV_ID]);

    // It actually mutated the cache (would be 0 if the handler early-returned).
    expect(writes).toBeGreaterThan(0);
  });

  it("agentctl_starting WITHOUT task_id and WITHOUT a cached session is a clean no-op", () => {
    // No prior state_changed → session unknown, no task_id to derive.
    ctx.ws.emit(AGENTCTL_STARTING, {
      session_id: "unknown-sess",
      task_environment_id: ENV_ID,
    });
    expect(ctx.qc.getQueryData(qk.taskSession.byId("unknown-sess"))).toBeUndefined();
  });
});
