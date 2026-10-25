import { sessionId as toSessionId, taskId as toTaskId } from "./types/ids";
import { describe, expect, it } from "vitest";
import type { ExecutionObservation, ExecutionSnapshot } from "./types/execution";
import { getExecutionStatus } from "./execution-status";

const now = Date.parse("2026-10-24T00:00:01Z");
const snapshot: ExecutionSnapshot = {
  schema_version: 1,
  attempt_id: "attempt",
  session_id: "session",
  task_id: "task",
  execution_id: "execution",
  generation: 1,
  sequence: 2,
  executor_id: "executor",
  executor_type: "local",
  agent_profile_id: "profile",
  transport: "chat",
  wrapper: {
    source: "profile",
    digest: "a".repeat(64),
    command: { executable: "nono", omitted_arguments: 1, references: [] },
  },
  agent: { executable: "agent", omitted_arguments: 1, references: [] },
  captured_at: "2026-10-24T00:00:00Z",
  sandbox: {
    attempt_id: "attempt",
    generation: 7,
    installation_id: "installation",
    provider_key: "fixture",
    provider_version: "1",
    package_digest: "a".repeat(64),
    runtime_version: "1",
    runtime_digest: "b".repeat(64),
    revision_id: "policy",
    revision_number: 1,
    policy_name: "Policy",
    policy_digest: "c".repeat(64),
    selection_source: "task",
    ceiling_revision_id: "ceiling",
    ceiling_digest: "d".repeat(64),
  },
};
const active: ExecutionObservation = {
  schema_version: 1,
  observer_generation: 2,
  revision: 3,
  session_id: "session",
  task_id: "task",
  snapshot_sequence: 2,
  attempt_id: "attempt",
  generation: 7,
  attempt_version: 4,
  status: "active",
  provider_health: "available",
  reason: "",
  mechanism: "fixture-kernel",
  checked_at: "2026-10-24T00:00:01Z",
  evidence_at: "2026-10-24T00:00:00Z",
  valid_until: "2026-10-24T00:00:30Z",
};
const session = {
  id: toSessionId("session"),
  task_id: toTaskId("task"),
  execution_snapshot: snapshot,
  execution_observation: active,
};

describe("truthful execution status", () => {
  it("uses fresh bound evidence and expires at the producer deadline", () => {
    expect(getExecutionStatus(session, "connected", now)).toBe("active");
    expect(getExecutionStatus(session, "connected", now + 29_000)).toBe("unknown");
  });
  it("never infers confinement from a wrapper name or missing history", () => {
    expect(
      getExecutionStatus(
        { ...session, execution_snapshot: { ...snapshot, sandbox: null } },
        "connected",
        now,
      ),
    ).toBe("not_configured");
    expect(getExecutionStatus({ ...session, execution_snapshot: null }, "connected", now)).toBe(
      "unknown",
    );
    expect(getExecutionStatus({ ...session, execution_observation: null }, "connected", now)).toBe(
      "unknown",
    );
  });
  it.each([
    { attempt_id: "foreign" },
    { session_id: "sibling" },
    { task_id: "foreign" },
    { generation: 8 },
    { snapshot_sequence: 1 },
    { observer_generation: 0 },
    { revision: -1 },
    { attempt_version: 0 },
    { checked_at: "invalid" },
    { checked_at: "2026-10-24T00:00:02Z" },
    { evidence_at: undefined },
    { mechanism: undefined },
    { mechanism: "/private/path" },
    { valid_until: undefined },
    { valid_until: "2026-10-24T00:05:00Z" },
    { evidence_at: "2026-10-24T00:00:02Z" },
    { reason: "evidence_unavailable" },
  ])("rejects unbound or invalid active evidence: %j", (patch) => {
    expect(
      getExecutionStatus(
        { ...session, execution_observation: { ...active, ...patch } },
        "connected",
        now,
      ),
    ).toBe("unknown");
  });
  it.each(["disconnected", "connecting", "reconnecting", "error"] as const)(
    "removes active status while %s",
    (connection) => {
      expect(getExecutionStatus(session, connection, now)).toBe("unknown");
    },
  );
  it.each(["configured", "starting", "unknown", "failed", "stopped"] as const)(
    "preserves the explicit %s outcome without a shield",
    (status) => {
      const value = {
        ...session,
        execution_observation: { ...active, status, valid_until: undefined },
      };
      expect(getExecutionStatus(value, "connected", now)).toBe(status);
      if (status === "stopped" || status === "failed") {
        expect(getExecutionStatus(value, "disconnected", now + 86_400_000)).toBe(status);
      }
    },
  );
  it("keeps provider health separate from confinement evidence", () => {
    expect(
      getExecutionStatus(
        {
          ...session,
          execution_observation: {
            ...active,
            status: "unknown",
            provider_health: "available",
            valid_until: undefined,
          },
        },
        "connected",
        now,
      ),
    ).toBe("unknown");
  });
});
