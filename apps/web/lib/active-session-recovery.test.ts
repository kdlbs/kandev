import { describe, expect, it } from "vitest";
import { selectActiveSessionRecovery } from "./active-session-recovery";
const FAILED_AT = "2026-09-20T10:00:00Z";
const RESOLVED_AT = "2026-09-20T11:00:00Z";
const NEW_FAILURE_AT = "2026-09-20T12:00:00Z";
const error = { message: "Connection lost", stamp: "current", details: "diagnostic" };
const session = { id: "session", state: "FAILED", metadata: { last_agent_error: error } };
const message = (stamp: string, kind?: string) => ({
  id: stamp,
  session_id: "session",
  created_at: FAILED_AT,
  content: "Connection lost",
  metadata: {
    recovery_actions: true,
    error_stamp: stamp,
    failure_kind: kind,
    error_output: "safe detail",
  },
});
describe("resolved session recovery ownership", () => {
  it.each(["FAILED", "WAITING_FOR_INPUT"])(
    "does not reuse a resolved failure while %s still has an error string",
    (state) => {
      expect(
        selectActiveSessionRecovery(
          {
            ...session,
            state,
            error_message: error.message,
            metadata: { ...session.metadata, recovery_resolved_at: RESOLVED_AT },
          },
          [message("current", "provider_quota_limited")],
        ),
      ).toBeNull();
    },
  );
  it("does not reuse a failed recovery after a successful boot from the same session", () => {
    expect(
      selectActiveSessionRecovery(session, [
        message("current", "provider_quota_limited"),
        {
          id: "boot",
          session_id: session.id,
          type: "script_execution",
          created_at: RESOLVED_AT,
          metadata: { script_type: "agent_boot", status: "exited", exit_code: 0 },
        },
      ]),
    ).toBeNull();
  });
  it("keeps a new failure actionable after an earlier recovery", () => {
    const model = selectActiveSessionRecovery(
      {
        ...session,
        metadata: {
          last_agent_error: { ...error, occurred_at: NEW_FAILURE_AT },
          recovery_resolved_at: RESOLVED_AT,
        },
      },
      [{ ...message("current", "provider_quota_limited"), created_at: NEW_FAILURE_AT }],
    );
    expect(model?.kind).toBe("provider_quota_limited");
  });
  it("honors durable resolution before failed-session history loads", () => {
    expect(
      selectActiveSessionRecovery(
        {
          ...session,
          metadata: {
            last_agent_error: { ...error, occurred_at: FAILED_AT },
            recovery_resolved_at: RESOLVED_AT,
          },
        },
        [],
      ),
    ).toBeNull();
  });
  it("does not let another session's successful boot retire the current failure", () => {
    const model = selectActiveSessionRecovery(session, [
      message("current", "provider_quota_limited"),
      {
        id: "foreign-boot",
        session_id: "other",
        type: "script_execution",
        created_at: RESOLVED_AT,
        metadata: { script_type: "agent_boot", status: "exited", exit_code: 0 },
      },
    ]);
    expect(model?.kind).toBe("provider_quota_limited");
  });
});

describe("active session recovery ownership", () => {
  it("selects matching metadata rather than an older specialized cause", () => {
    const model = selectActiveSessionRecovery(session, [
      message("current"),
      message("old", "provider_quota_limited"),
    ]);
    expect(model?.kind).toBe("generic");
    expect(model?.stamp).toBe("current");
  });
  it("owns interrupted waiting with an error, but not healthy waiting with historical metadata", () => {
    expect(
      selectActiveSessionRecovery(
        { ...session, state: "WAITING_FOR_INPUT", error_message: "lost" },
        [],
      ),
    ).not.toBeNull();
    expect(selectActiveSessionRecovery({ ...session, state: "WAITING_FOR_INPUT" }, [])).toBeNull();
  });
  it.each(["managed_runtime_npm_resolution", "provider_quota_limited"])(
    "preserves %s recovery semantics",
    (kind) => {
      expect(selectActiveSessionRecovery(session, [message("current", kind)])?.kind).toBe(kind);
    },
  );
  it("does not adopt foreign-session or task-scoped failures", () => {
    expect(
      selectActiveSessionRecovery(session, [
        { ...message("current", "provider_quota_limited"), session_id: "other" },
      ])?.kind,
    ).toBe("generic");
    expect(
      selectActiveSessionRecovery(
        { ...session, metadata: { last_agent_error: { ...error, scope: "task" } } },
        [],
      ),
    ).toBeNull();
  });
  it.each(["RUNNING", "STARTING", "COMPLETED", "CANCELLED"])(
    "does not block %s from retained errors",
    (state) => {
      expect(selectActiveSessionRecovery({ ...session, state }, [])).toBeNull();
    },
  );
});

it("owns a current unresolved waiting failure without the legacy error string", () => {
  expect(
    selectActiveSessionRecovery({ ...session, state: "WAITING_FOR_INPUT" }, [message("current")]),
  ).not.toBeNull();
});
it("keeps the composer usable after durable recovery or a successful later boot", () => {
  const waiting = { ...session, state: "WAITING_FOR_INPUT" };
  expect(
    selectActiveSessionRecovery(
      {
        ...waiting,
        metadata: { ...session.metadata, recovery_resolved_at: RESOLVED_AT },
      },
      [message("current")],
    ),
  ).toBeNull();
  expect(
    selectActiveSessionRecovery(waiting, [
      message("current"),
      {
        id: "boot",
        session_id: "session",
        type: "script_execution",
        created_at: RESOLVED_AT,
        metadata: { script_type: "agent_boot", status: "exited", exit_code: 0 },
      },
    ]),
  ).toBeNull();
});

it("uses a matching live task error before session metadata catches up", () => {
  expect(
    selectActiveSessionRecovery(
      { id: "session", state: "WAITING_FOR_INPUT" },
      [message("current")],
      {
        scope: "session",
        session_id: "session",
        stamp: "current",
        occurred_at: FAILED_AT,
        preview: "Connection lost",
      },
    ),
  ).not.toBeNull();
});

it("reconstructs unresolved recovery on a fresh STARTING mount", () => {
  const starting = { ...session, state: "STARTING" };
  expect(selectActiveSessionRecovery(starting, [message("current")])?.stamp).toBe("current");
  expect(
    selectActiveSessionRecovery(
      {
        ...starting,
        metadata: { ...session.metadata, recovery_resolved_at: RESOLVED_AT },
      },
      [message("current")],
    ),
  ).toBeNull();
});

it("retains a durable unresolved startup failure before history finishes loading", () => {
  const starting = {
    ...session,
    state: "STARTING",
    metadata: { last_agent_error: { ...error, occurred_at: FAILED_AT } },
  };
  expect(selectActiveSessionRecovery(starting, [])?.stamp).toBe("current");
  expect(
    selectActiveSessionRecovery(
      {
        ...starting,
        metadata: { ...starting.metadata, recovery_resolved_at: RESOLVED_AT },
      },
      [],
    ),
  ).toBeNull();
});
