import { describe, it, expect } from "vitest";
import {
  BRIDGE_SKIPPED_ACTIONS,
  BRIDGE_SKIPPED_PREFIXES,
  diffBridgeAudit,
  formatHandlerSideDrops,
  isBridgeSkippedAction,
  type BridgeAuditEntry,
  type WsAccountSnapshotLike,
} from "./bridge-audit-diff";
import {
  BRIDGE_SKIPPED_ACTIONS as BRIDGE_RUNTIME_SKIPPED_ACTIONS,
  BRIDGE_SKIPPED_PREFIXES as BRIDGE_RUNTIME_SKIPPED_PREFIXES,
} from "./bridge";

// Shared literals — sonarjs/no-duplicate-string flags any string repeated
// 4+ times. Extracting to constants both pacifies the lint and makes it
// obvious which scenarios are exercising the same canonical event.
const ACT_MSG_ADDED = "session.message.added";
const ACT_MSG_UPDATED = "session.message.updated";
const ACT_STATE_CHANGED = "session.state_changed";
const ACT_QUEUE_STATUS = "message.queue.status_changed";
const ACT_TASK_UPDATED = "task.updated";
const SESS_1 = "sess-1";
const REASON_NO_ENTRY = "no-bridge-entry" as const;

function snapshot(
  events: Array<{ seq?: number; action: string; sessionId: string | null; type?: string }>,
): WsAccountSnapshotLike {
  return {
    receivedEvents: events.map((e, i) => ({
      seq: e.seq ?? i + 1,
      action: e.action,
      sessionId: e.sessionId,
      type: e.type,
    })),
  };
}

function audit(
  overrides: Partial<BridgeAuditEntry> & Pick<BridgeAuditEntry, "action">,
): BridgeAuditEntry {
  return {
    action: overrides.action,
    sessionId: overrides.sessionId ?? null,
    taskId: overrides.taskId ?? null,
    cacheChanged: overrides.cacheChanged ?? true,
    mutationCount: overrides.mutationCount ?? (overrides.cacheChanged === false ? 0 : 1),
    timestamp: overrides.timestamp ?? 0,
  };
}

describe("isBridgeSkippedAction", () => {
  it("matches an exact allowlist entry", () => {
    expect(isBridgeSkippedAction(ACT_QUEUE_STATUS)).toBe(true);
  });

  it("no longer skips session.state_changed (now bridged into the taskSession caches)", () => {
    expect(isBridgeSkippedAction(ACT_STATE_CHANGED)).toBe(false);
  });

  it("matches an allowlist prefix", () => {
    expect(isBridgeSkippedAction("agentctl_status")).toBe(true);
    expect(isBridgeSkippedAction("agentctl_log_appended")).toBe(true);
  });

  it("no longer skips session.agentctl_* (now bridged by bridge/session-state.ts)", () => {
    // The agentctl handshake family (starting / ready / error) is now mirrored
    // into the TQ taskSession caches, so the dedicated `session.agentctl_`
    // prefix was removed from the allowlist. The bare `agentctl_` prefix (the
    // in-container log/status channels) still does NOT match these because they
    // start with `session.`.
    expect(isBridgeSkippedAction("session.agentctl_starting")).toBe(false);
    expect(isBridgeSkippedAction("session.agentctl_ready")).toBe(false);
    expect(isBridgeSkippedAction("session.agentctl_error")).toBe(false);
  });

  it("matches the user_shell. prefix (Workstream 3 expansion)", () => {
    expect(isBridgeSkippedAction("user_shell.list")).toBe(true);
    expect(isBridgeSkippedAction("user_shell.create")).toBe(true);
    expect(isBridgeSkippedAction("user_shell.destroy")).toBe(true);
    expect(isBridgeSkippedAction("user_shell.park")).toBe(true);
  });

  it("allowlists subscription / focus lifecycle acks", () => {
    expect(isBridgeSkippedAction("session.subscribe")).toBe(true);
    expect(isBridgeSkippedAction("session.unsubscribe")).toBe(true);
    expect(isBridgeSkippedAction("session.focus")).toBe(true);
    expect(isBridgeSkippedAction("session.unfocus")).toBe(true);
    expect(isBridgeSkippedAction("task.subscribe")).toBe(true);
    expect(isBridgeSkippedAction("run.subscribe")).toBe(true);
    expect(isBridgeSkippedAction("user.subscribe")).toBe(true);
  });

  it("allowlists session lifecycle operation acks", () => {
    expect(isBridgeSkippedAction("session.launch")).toBe(true);
    expect(isBridgeSkippedAction("session.ensure")).toBe(true);
    expect(isBridgeSkippedAction("session.stop")).toBe(true);
    expect(isBridgeSkippedAction("session.delete")).toBe(true);
    expect(isBridgeSkippedAction("session.reset_context")).toBe(true);
    expect(isBridgeSkippedAction("session.set_mode")).toBe(true);
  });

  it("allowlists task.session.* polling acks", () => {
    expect(isBridgeSkippedAction("task.session.status")).toBe(true);
    expect(isBridgeSkippedAction("task.session.list")).toBe(true);
    expect(isBridgeSkippedAction("task.session")).toBe(true);
  });

  it("allowlists agent operation acks", () => {
    expect(isBridgeSkippedAction("agent.prompt")).toBe(true);
    expect(isBridgeSkippedAction("agent.cancel")).toBe(true);
    expect(isBridgeSkippedAction("agent.stop")).toBe(true);
    expect(isBridgeSkippedAction("permission.respond")).toBe(true);
  });

  it("allowlists Zustand-only notifications beyond state_changed", () => {
    expect(isBridgeSkippedAction("session.waiting_for_input")).toBe(true);
    expect(isBridgeSkippedAction("input.requested")).toBe(true);
    expect(isBridgeSkippedAction("permission.requested")).toBe(true);
  });

  it("allowlists message-queue request acks (notification stays separate)", () => {
    expect(isBridgeSkippedAction("message.queue.add")).toBe(true);
    expect(isBridgeSkippedAction("message.queue.cancel")).toBe(true);
    expect(isBridgeSkippedAction("message.queue.remove")).toBe(true);
    // The notification is also allowlisted (Zustand-only path).
    expect(isBridgeSkippedAction(ACT_QUEUE_STATUS)).toBe(true);
  });

  it("allowlists task.plan request acks but NOT the bridged notifications", () => {
    // Request acks → allowlisted.
    expect(isBridgeSkippedAction("task.plan.get")).toBe(true);
    expect(isBridgeSkippedAction("task.plan.create")).toBe(true);
    expect(isBridgeSkippedAction("task.plan.revisions.list")).toBe(true);
    // Notifications that ARE bridged in session.ts → NOT allowlisted.
    expect(isBridgeSkippedAction("task.plan.created")).toBe(false);
    expect(isBridgeSkippedAction("task.plan.updated")).toBe(false);
    expect(isBridgeSkippedAction("task.plan.deleted")).toBe(false);
  });

  it("allowlists session git / file-review query acks", () => {
    expect(isBridgeSkippedAction("session.git.snapshots")).toBe(true);
    expect(isBridgeSkippedAction("session.git.commits")).toBe(true);
    expect(isBridgeSkippedAction("session.cumulative_diff")).toBe(true);
    expect(isBridgeSkippedAction("session.file_review.get")).toBe(true);
    expect(isBridgeSkippedAction("session.file_review.update")).toBe(true);
  });

  it("allowlists shell / vscode operation acks", () => {
    expect(isBridgeSkippedAction("session.shell.status")).toBe(true);
    expect(isBridgeSkippedAction("shell.subscribe")).toBe(true);
    expect(isBridgeSkippedAction("shell.input")).toBe(true);
    expect(isBridgeSkippedAction("vscode.start")).toBe(true);
    expect(isBridgeSkippedAction("vscode.openFile")).toBe(true);
  });
});

describe("isBridgeSkippedAction — negative cases", () => {
  it("does not match unrelated actions", () => {
    expect(isBridgeSkippedAction(ACT_MSG_ADDED)).toBe(false);
    expect(isBridgeSkippedAction(ACT_TASK_UPDATED)).toBe(false);
    // Bridged notifications must NOT be allowlisted.
    expect(isBridgeSkippedAction("session.message.added")).toBe(false);
    expect(isBridgeSkippedAction("session.turn.started")).toBe(false);
    expect(isBridgeSkippedAction("session.git.event")).toBe(false);
    expect(isBridgeSkippedAction("session.mode_changed")).toBe(false);
  });

  it("uses caller-supplied allowlists when provided", () => {
    const customActions = new Set<string>(["custom.action"]);
    const customPrefixes = ["custom_"];
    expect(isBridgeSkippedAction("custom.action", customActions, customPrefixes)).toBe(true);
    expect(isBridgeSkippedAction("custom_foo", customActions, customPrefixes)).toBe(true);
    expect(isBridgeSkippedAction(ACT_STATE_CHANGED, customActions, customPrefixes)).toBe(false);
  });
});

describe("diffBridgeAudit", () => {
  it("returns no-bridge-entry when no audit entry exists for the receipt", () => {
    const snap = snapshot([{ action: ACT_MSG_ADDED, sessionId: SESS_1 }]);
    const drops = diffBridgeAudit(snap, []);
    expect(drops).toEqual([{ action: ACT_MSG_ADDED, sessionId: SESS_1, reason: REASON_NO_ENTRY }]);
  });

  it("returns cache-unchanged when audit entry exists but cache did not change", () => {
    const snap = snapshot([{ action: ACT_MSG_ADDED, sessionId: SESS_1 }]);
    const drops = diffBridgeAudit(snap, [
      audit({ action: ACT_MSG_ADDED, sessionId: SESS_1, cacheChanged: false }),
    ]);
    expect(drops).toEqual([
      { action: ACT_MSG_ADDED, sessionId: SESS_1, reason: "cache-unchanged" },
    ]);
  });

  it("returns no drop when audit entry exists and cache changed", () => {
    const snap = snapshot([{ action: ACT_MSG_ADDED, sessionId: SESS_1 }]);
    const drops = diffBridgeAudit(snap, [
      audit({ action: ACT_MSG_ADDED, sessionId: SESS_1, cacheChanged: true }),
    ]);
    expect(drops).toEqual([]);
  });

  it("returns no drop when at least one matching audit entry mutated the cache", () => {
    // The bridge can record multiple entries per (action, session) — e.g. two
    // domain registrars for the same event. As long as one of them mutated
    // the cache the receipt is considered applied.
    const snap = snapshot([{ action: ACT_MSG_ADDED, sessionId: SESS_1 }]);
    const drops = diffBridgeAudit(snap, [
      audit({ action: ACT_MSG_ADDED, sessionId: SESS_1, cacheChanged: false }),
      audit({ action: ACT_MSG_ADDED, sessionId: SESS_1, cacheChanged: true }),
    ]);
    expect(drops).toEqual([]);
  });

  it("returns [] for an empty receipts list", () => {
    expect(diffBridgeAudit(snapshot([]), [])).toEqual([]);
  });
});

describe("diffBridgeAudit — allowlist filtering", () => {
  it("skips bridge-allowlisted actions even without an audit entry", () => {
    const snap = snapshot([{ action: ACT_QUEUE_STATUS, sessionId: SESS_1 }]);
    expect(diffBridgeAudit(snap, [])).toEqual([]);
  });

  it("skips actions matched by an allowlist prefix (e.g. agentctl_*)", () => {
    const snap = snapshot([
      { action: "agentctl_status", sessionId: SESS_1 },
      { action: "agentctl_log_appended", sessionId: SESS_1 },
    ]);
    expect(diffBridgeAudit(snap, [])).toEqual([]);
  });

  it("skips user_shell.* receipts (prefix allowlist)", () => {
    const snap = snapshot([
      { action: "user_shell.list", sessionId: SESS_1 },
      { action: "user_shell.create", sessionId: SESS_1 },
    ]);
    expect(diffBridgeAudit(snap, [])).toEqual([]);
  });

  it("requires a bridge entry for session.agentctl_* (no longer skipped)", () => {
    // These are now bridged by bridge/session-state.ts, so a receipt with no
    // matching audit entry is a real no-bridge-entry drop; with a cache-mutating
    // entry it is applied.
    const snap = snapshot([
      { action: "session.agentctl_starting", sessionId: SESS_1 },
      { action: "session.agentctl_ready", sessionId: SESS_1 },
    ]);
    const drops = diffBridgeAudit(snap, [
      audit({ action: "session.agentctl_ready", sessionId: SESS_1, cacheChanged: true }),
    ]);
    expect(drops).toEqual([
      { action: "session.agentctl_starting", sessionId: SESS_1, reason: REASON_NO_ENTRY },
    ]);
  });

  it("skips control-plane request/response acks (the no-bridge-entry source)", () => {
    // Representative entries from each control-plane group. Their responses
    // come back with the same action + session_id but never run through a
    // bridge handler (responses are dispatched via pendingRequests).
    const snap = snapshot([
      { action: "session.subscribe", sessionId: SESS_1 },
      { action: "session.focus", sessionId: SESS_1 },
      { action: "task.session.status", sessionId: SESS_1 },
      { action: "session.launch", sessionId: SESS_1 },
      { action: "agent.prompt", sessionId: SESS_1 },
      { action: "message.queue.add", sessionId: SESS_1 },
      { action: "task.plan.get", sessionId: SESS_1 },
      { action: "session.git.snapshots", sessionId: SESS_1 },
    ]);
    expect(diffBridgeAudit(snap, [])).toEqual([]);
  });

  it("skips events with no sessionId (not bridge-scoped)", () => {
    const snap = snapshot([
      { action: ACT_TASK_UPDATED, sessionId: null },
      { action: "workspace.changed", sessionId: null },
    ]);
    expect(diffBridgeAudit(snap, [])).toEqual([]);
  });

  it("does not cross-match audit entries from a different session", () => {
    const snap = snapshot([{ action: ACT_MSG_ADDED, sessionId: SESS_1 }]);
    const drops = diffBridgeAudit(snap, [
      audit({ action: ACT_MSG_ADDED, sessionId: "sess-OTHER", cacheChanged: true }),
    ]);
    expect(drops).toEqual([{ action: ACT_MSG_ADDED, sessionId: SESS_1, reason: REASON_NO_ENTRY }]);
  });

  it("reports drops independently for multiple receipts", () => {
    const snap = snapshot([
      { action: ACT_MSG_ADDED, sessionId: SESS_1 },
      { action: ACT_MSG_UPDATED, sessionId: SESS_1 },
      { action: ACT_TASK_UPDATED, sessionId: "sess-2" },
    ]);
    const drops = diffBridgeAudit(snap, [
      // sess-1 message.added — applied
      audit({ action: ACT_MSG_ADDED, sessionId: SESS_1, cacheChanged: true }),
      // sess-1 message.updated — ran but didn't mutate
      audit({ action: ACT_MSG_UPDATED, sessionId: SESS_1, cacheChanged: false }),
      // sess-2 task.updated — no entry
    ]);
    expect(drops).toEqual([
      { action: ACT_MSG_UPDATED, sessionId: SESS_1, reason: "cache-unchanged" },
      { action: ACT_TASK_UPDATED, sessionId: "sess-2", reason: REASON_NO_ENTRY },
    ]);
  });

  it("exports the documented allowlist constants verbatim", () => {
    expect(BRIDGE_SKIPPED_ACTIONS.has(ACT_STATE_CHANGED)).toBe(false);
    expect(BRIDGE_SKIPPED_ACTIONS.has(ACT_QUEUE_STATUS)).toBe(true);
    expect(BRIDGE_SKIPPED_ACTIONS.has("session.subscribe")).toBe(true);
    expect(BRIDGE_SKIPPED_ACTIONS.has("task.session.status")).toBe(true);
    expect(BRIDGE_SKIPPED_PREFIXES).toContain("agentctl_");
    expect(BRIDGE_SKIPPED_PREFIXES).not.toContain("session.agentctl_");
    expect(BRIDGE_SKIPPED_PREFIXES).toContain("user_shell.");
  });
});

describe("diffBridgeAudit — envelope type filtering", () => {
  it("does not flag non-notification envelopes as drops", () => {
    // Responses / errors / requests are dispatched via pendingRequests in
    // client.ts and never reach ws.on, so the bridge never sees them. Even with
    // a sessionId and no bridge entry, they must NOT read as a drop — the
    // receipt layer skips `type !== "notification"` structurally.
    const snap = snapshot([
      { action: ACT_MSG_ADDED, sessionId: SESS_1, type: "response" },
      { action: ACT_TASK_UPDATED, sessionId: SESS_1, type: "error" },
      { action: ACT_MSG_UPDATED, sessionId: SESS_1, type: "request" },
    ]);
    expect(diffBridgeAudit(snap, [])).toEqual([]);
  });

  it("still flags a notification (or untyped, back-compat) envelope with no entry", () => {
    const snap = snapshot([
      { action: ACT_MSG_ADDED, sessionId: SESS_1, type: "notification" },
      // Untyped (pre-Phase-2 bundle) is treated as a notification too.
      { action: ACT_MSG_UPDATED, sessionId: SESS_1 },
    ]);
    expect(diffBridgeAudit(snap, [])).toEqual([
      { action: ACT_MSG_ADDED, sessionId: SESS_1, reason: REASON_NO_ENTRY },
      { action: ACT_MSG_UPDATED, sessionId: SESS_1, reason: REASON_NO_ENTRY },
    ]);
  });
});

describe("BRIDGE_SKIPPED_* duplication guard", () => {
  // The runtime copy in `lib/query/bridge/index.ts` is the source of truth; the
  // pure copy in `bridge-audit-diff.ts` is duplicated so the diff helper stays
  // free of the QueryClient runtime. This test fails the moment the two drift,
  // forcing both copies to be updated together.
  it("the actions allowlist matches the bridge-runtime copy exactly", () => {
    expect([...BRIDGE_SKIPPED_ACTIONS].sort()).toEqual([...BRIDGE_RUNTIME_SKIPPED_ACTIONS].sort());
  });

  it("the prefixes allowlist matches the bridge-runtime copy exactly", () => {
    expect([...BRIDGE_SKIPPED_PREFIXES].sort()).toEqual(
      [...BRIDGE_RUNTIME_SKIPPED_PREFIXES].sort(),
    );
  });
});

describe("formatHandlerSideDrops", () => {
  it("returns empty string for no drops", () => {
    expect(formatHandlerSideDrops([])).toBe("");
  });

  it("formats a small list verbatim", () => {
    const msg = formatHandlerSideDrops([
      { action: ACT_MSG_ADDED, sessionId: SESS_1, reason: REASON_NO_ENTRY },
      { action: ACT_TASK_UPDATED, sessionId: null, reason: "cache-unchanged" },
    ]);
    expect(msg).toContain("2 WS event(s)");
    expect(msg).toContain("action=session.message.added");
    expect(msg).toContain("session=sess-1");
    expect(msg).toContain("reason=no-bridge-entry");
    expect(msg).toContain("session=<none>");
  });

  it("caps at 20 entries with a trailing '... and N more'", () => {
    const drops = Array.from({ length: 25 }, (_, i) => ({
      action: `act-${i}`,
      sessionId: `sess-${i}`,
      reason: REASON_NO_ENTRY,
    }));
    const msg = formatHandlerSideDrops(drops);
    expect(msg).toContain("25 WS event(s)");
    expect(msg).toContain("... and 5 more");
  });
});
