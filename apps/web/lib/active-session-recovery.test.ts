import { describe, expect, it } from "vitest";
import { selectActiveSessionRecovery } from "./active-session-recovery";
const FAILED_AT = "2026-09-20T10:00:00Z";
const RESOLVED_AT = "2026-09-20T11:00:00Z";
const NEW_FAILURE_AT = "2026-09-20T12:00:00Z";
const error = { message: "Connection lost", stamp: "current", details: "diagnostic" };
const session = { id: "session", state: "FAILED", metadata: { last_agent_error: error } };
const resolution = (stamp = "current") => ({
  recovery_resolved_at: RESOLVED_AT,
  recovery_resolutions: [{ error_stamp: stamp, resolved_at: RESOLVED_AT, attempt_id: "resume-1" }],
});
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
            metadata: { ...session.metadata, ...resolution() },
          },
          [message("current", "provider_quota_limited")],
        ),
      ).toBeNull();
    },
  );
  it("does not infer stamped recovery from an uncorrelated successful boot", () => {
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
    ).not.toBeNull();
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
            ...resolution(),
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

describe("selection failure recovery occurrence ownership", () => {
  const selectionError = {
    ...error,
    causes: [{ operation: "resume", code: "model_unavailable" }],
  };
  const selectionMessage = {
    ...message("current"),
    metadata: {
      ...message("current").metadata,
      causes: [{ operation: "resume", code: "model_unavailable" }],
    },
  };
  const success = (stamp: string) => ({
    id: `success-${stamp}`,
    session_id: "session",
    type: "status",
    created_at: RESOLVED_AT,
    metadata: {
      variant: "resume_settings_provider_restored",
      resolved_error_stamp: stamp,
    },
  });

  it("resolves only when the successful attempt names the same error stamp", () => {
    const failedSession = { ...session, metadata: { last_agent_error: selectionError } };
    expect(
      selectActiveSessionRecovery(failedSession, [selectionMessage, success("other")]),
    ).not.toBeNull();
    expect(
      selectActiveSessionRecovery(failedSession, [selectionMessage, success("current")]),
    ).not.toBeNull();
    expect(
      selectActiveSessionRecovery(
        {
          ...failedSession,
          metadata: { ...failedSession.metadata, ...resolution() },
        },
        [selectionMessage],
      ),
    ).toBeNull();
  });

  it("keeps a later selection failure active after an older success", () => {
    const laterFailure = {
      ...selectionMessage,
      id: "later-selection-failure",
      created_at: NEW_FAILURE_AT,
      metadata: { ...selectionMessage.metadata, error_stamp: "failure-later" },
    };
    const model = selectActiveSessionRecovery(
      {
        ...session,
        metadata: {
          last_agent_error: {
            ...selectionError,
            stamp: "failure-later",
            occurred_at: NEW_FAILURE_AT,
          },
          ...resolution(),
        },
      },
      [success("current"), laterFailure],
    );
    expect(model?.stamp).toBe("failure-later");
  });

  it("does not treat manual dismissal as successful recovery", () => {
    expect(
      selectActiveSessionRecovery(
        {
          ...session,
          state: "FAILED",
          metadata: {
            last_agent_error: { ...selectionError, dismissed_at: RESOLVED_AT },
          },
        },
        [selectionMessage, success("other")],
      ),
    ).toBeNull();
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
  it("keeps a canceled session actionable only for the typed managed clone repair", () => {
    const managedError = {
      message: "Workspace needs repair",
      occurred_at: FAILED_AT,
      stamp: "managed-clone-stamp",
      scope: "session",
      code: "managed_clone_relocation_required",
      recovery_actions: ["relocate_and_resume"],
    };
    const model = selectActiveSessionRecovery(
      { ...session, state: "CANCELLED", metadata: { last_agent_error: managedError } },
      [],
    );
    expect(model?.kind).toBe("managed_clone_relocation_required");
    expect(model?.stamp).toBe("managed-clone-stamp");
    expect(selectActiveSessionRecovery({ ...session, state: "CANCELLED" }, [])).toBeNull();
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
        metadata: { ...session.metadata, ...resolution() },
      },
      [message("current")],
    ),
  ).toBeNull();
  expect(
    selectActiveSessionRecovery(
      { ...waiting, metadata: { ...session.metadata, ...resolution() } },
      [
        message("current"),
        {
          id: "boot",
          session_id: "session",
          type: "script_execution",
          created_at: RESOLVED_AT,
          metadata: { script_type: "agent_boot", status: "exited", exit_code: 0 },
        },
      ],
    ),
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
        metadata: { ...session.metadata, ...resolution() },
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
        metadata: { ...starting.metadata, ...resolution() },
      },
      [],
    ),
  ).toBeNull();
});
