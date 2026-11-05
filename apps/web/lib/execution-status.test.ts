import { sessionId as toSessionId, taskId as toTaskId } from "./types/ids";
import { describe, expect, it } from "vitest";
import type { ExecutionObservation, ExecutionSnapshot } from "./types/execution";
import { getExecutionFacts, executionObservationDeadline } from "./execution-facts";
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

const facts = {
  schema_version: 1 as const,
  process: {
    state: "running" as const,
    checked_at: active.checked_at,
    valid_until: "2026-10-24T00:00:31Z",
  },
  launch_checks: {
    state: "passed" as const,
    receipt: {
      checked_at: active.checked_at,
      policy_digest: snapshot.sandbox!.policy_digest,
      package_digest: snapshot.sandbox!.package_digest,
      runtime_digest: snapshot.sandbox!.runtime_digest,
      checks: ["policy_ceiling", "runtime_checksum", "private_artifacts", "process_containment"],
    },
  },
  runtime: {
    state: "reported" as const,
    observations: [
      {
        source_id: "runtime.v2",
        evidence_digest: "a".repeat(64),
        observed_at: "2026-10-23T00:00:00Z",
        platform: "linux" as const,
        landlock_abi: 5,
        execute_enforced: true,
        command_mediation_active: true,
      },
    ],
  },
  audit: {
    integrity: "unchecked" as const,
    finalization: "unknown" as const,
    signer: "unchecked" as const,
  },
};
const running = {
  ...session,
  execution_observation: {
    ...active,
    status: "unknown" as const,
    provider_health: "unavailable" as const,
    facts,
  },
};

describe("independent execution facts", () => {
  it("invalidates current signer trust on disconnect while retaining original verification", () => {
    const verification = {
      id: "verification",
      completed_at: "2026-10-23T00:00:00Z",
      artifact_set_digest: "a".repeat(64),
      verifier_version: "fixture/1",
      signature_state: "verified" as const,
      ledger_state: "valid" as const,
      trust_revision: "operator-revision",
    };
    const audited = {
      ...running,
      state: "RUNNING" as const,
      started_at: snapshot.captured_at,
      updated_at: snapshot.captured_at,
      execution_observation: {
        ...running.execution_observation,
        status: "stopped" as const,
        facts: {
          ...facts,
          process: { state: "stopped" as const },
          audit: {
            integrity: "valid" as const,
            finalization: "final" as const,
            signer: "trusted" as const,
            verification,
          },
        },
      },
    };
    expect(getExecutionFacts(audited, "connected", now)?.audit.signer).toBe("trusted");
    for (const value of [getExecutionFacts(audited, "disconnected", now)]) {
      expect(value?.audit.signer).toBe("untrusted");
      expect(value?.audit.verification).toEqual(verification);
    }
    expect(getExecutionFacts(audited, "connected", now + 30_000)?.audit.signer).toBe("trusted");
    expect(executionObservationDeadline(audited, now)).toBeNull();
    expect(getExecutionStatus(audited, "connected", now)).toBe("stopped");
  });
  it("shows owned liveness independently and never upgrades confinement", () => {
    expect(getExecutionFacts(running, "connected", now)?.process.state).toBe("running");
    expect(getExecutionStatus(running, "connected", now)).toBe("unknown");
    expect(
      getExecutionFacts(running, "connected", now)?.runtime.observations?.[0].observed_at,
    ).toBe("2026-10-23T00:00:00Z");
  });
  it("expires liveness on disconnect or deadline while retaining history", () => {
    for (const value of [
      getExecutionFacts(running, "disconnected", now),
      getExecutionFacts(running, "connected", now + 30_000),
    ]) {
      expect(value?.process.state).toBe("unknown");
      expect(value?.launch_checks.receipt).toEqual(facts.launch_checks.receipt);
    }
  });
  it.each(["0", "2026-02-30T00:00:00Z", "2026-10-24T00:00:02Z"])(
    "rejects malformed or future process time %s",
    (checked_at) => {
      const value = {
        ...running,
        execution_observation: {
          ...running.execution_observation,
          facts: { ...facts, process: { ...facts.process, checked_at } },
        },
      };
      expect(getExecutionFacts(value, "connected", now)?.process.state).toBe("unknown");
    },
  );
  it("does not reuse sibling facts or invent launch receipts for old rows", () => {
    expect(
      getExecutionFacts(
        {
          ...running,
          execution_observation: { ...running.execution_observation, attempt_id: "sibling" },
        },
        "connected",
        now,
      ),
    ).toBeNull();
    expect(getExecutionFacts(session, "connected", now)?.launch_checks.state).toBe("unavailable");
    const value = {
      ...running,
      execution_observation: {
        ...running.execution_observation,
        facts: {
          ...facts,
          launch_checks: {
            ...facts.launch_checks,
            receipt: { ...facts.launch_checks.receipt, policy_digest: "f".repeat(64) },
          },
        },
      },
    };
    expect(getExecutionFacts(value, "connected", now)?.launch_checks.state).toBe("unavailable");
  });
});

it("schedules process expiry when confinement is unknown and after an earlier evidence deadline", () => {
  const value = {
    ...session,
    execution_observation: {
      ...active,
      status: "unknown" as const,
      facts: {
        schema_version: 1 as const,
        process: {
          state: "running" as const,
          checked_at: active.checked_at,
          valid_until: "2026-10-24T00:00:31Z",
        },
        launch_checks: { state: "unavailable" as const },
        runtime: { state: "missing" as const },
        audit: {
          integrity: "unchecked" as const,
          finalization: "unknown" as const,
          signer: "unchecked" as const,
        },
      },
    },
  };
  expect(
    executionObservationDeadline(value as Parameters<typeof executionObservationDeadline>[0], now),
  ).toBe(now + 30_000);
  const withActive = {
    ...value,
    execution_observation: { ...value.execution_observation, status: "active" as const },
  };
  expect(
    executionObservationDeadline(
      withActive as Parameters<typeof executionObservationDeadline>[0],
      now + 29_000,
    ),
  ).toBe(now + 30_000);
});
