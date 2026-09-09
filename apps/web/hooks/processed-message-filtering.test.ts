import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import {
  hasFailedAgentBootAfter,
  hasSessionRecoveryResolutionAfter,
  hasSuccessfulAgentBootAfter,
  isSuccessfulScriptExecutionMetadata,
} from "./processed-message-filtering";

const ERROR_AT = "2026-05-30T00:00:00Z";
const AFTER = "2026-05-30T00:01:00Z";
const BEFORE = "2026-05-29T23:59:00Z";

function bootMessage(createdAt: string, metadata: Record<string, unknown> = {}): Message {
  return {
    id: `boot-${createdAt}-${JSON.stringify(metadata)}`,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "",
    type: "script_execution",
    created_at: createdAt,
    metadata: {
      script_type: "agent_boot",
      agent_name: "Mock",
      is_resuming: true,
      status: "exited",
      ...metadata,
    },
  } as Message;
}

describe("hasSuccessfulAgentBootAfter", () => {
  it("returns true when the agent was resumed after the failure", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER)], ERROR_AT)).toBe(true);
  });

  it("returns true when a fresh start re-established the agent after the failure", () => {
    expect(
      hasSuccessfulAgentBootAfter([bootMessage(AFTER, { is_resuming: false })], ERROR_AT),
    ).toBe(true);
  });

  it("returns true when an explicit zero exit code reports success", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER, { exit_code: 0 })], ERROR_AT)).toBe(
      true,
    );
  });

  it("returns false when the only successful boot predates the failure", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(BEFORE)], ERROR_AT)).toBe(false);
  });

  it("returns false when the resume attempt failed", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER, { status: "failed" })], ERROR_AT)).toBe(
      false,
    );
  });

  it("returns false when the boot exited non-zero", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER, { exit_code: 1 })], ERROR_AT)).toBe(
      false,
    );
  });

  it("returns false while the resume is still running", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER, { status: "running" })], ERROR_AT)).toBe(
      false,
    );
  });

  it("ignores non-boot scripts that finished after the failure", () => {
    const setup = bootMessage(AFTER, { script_type: "setup" });
    expect(hasSuccessfulAgentBootAfter([setup], ERROR_AT)).toBe(false);
  });

  it("ignores messages that are not script executions", () => {
    const chat = { ...bootMessage(AFTER), type: "message" } as Message;
    expect(hasSuccessfulAgentBootAfter([chat], ERROR_AT)).toBe(false);
  });

  it("returns false for unparseable or missing timestamps", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage("")], ERROR_AT)).toBe(false);
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER)], "")).toBe(false);
    expect(hasSuccessfulAgentBootAfter(undefined, ERROR_AT)).toBe(false);
    expect(hasSuccessfulAgentBootAfter([], ERROR_AT)).toBe(false);
  });

  it("finds the boot even when it is not the newest message", () => {
    const later = { ...bootMessage("2026-05-30T00:02:00Z"), type: "message" } as Message;
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER), later], ERROR_AT)).toBe(true);
  });
});

describe("recovery resolution metadata", () => {
  it("resolves a card only when the session timestamp is newer", () => {
    expect(hasSessionRecoveryResolutionAfter({ recovery_resolved_at: AFTER }, ERROR_AT)).toBe(true);
    expect(hasSessionRecoveryResolutionAfter({ recovery_resolved_at: BEFORE }, ERROR_AT)).toBe(
      false,
    );
  });

  it("rejects missing or invalid recovery timestamps", () => {
    expect(hasSessionRecoveryResolutionAfter(undefined, ERROR_AT)).toBe(false);
    expect(hasSessionRecoveryResolutionAfter({ recovery_resolved_at: "invalid" }, ERROR_AT)).toBe(
      false,
    );
    expect(hasSessionRecoveryResolutionAfter({ recovery_resolved_at: AFTER }, "invalid")).toBe(
      false,
    );
  });
});

describe("isSuccessfulScriptExecutionMetadata", () => {
  it("uses the shared exited-zero success rule", () => {
    expect(isSuccessfulScriptExecutionMetadata({ status: "exited" })).toBe(true);
    expect(isSuccessfulScriptExecutionMetadata({ status: "exited", exit_code: 0 })).toBe(true);
    expect(isSuccessfulScriptExecutionMetadata({ status: "exited", exit_code: 1 })).toBe(false);
    expect(isSuccessfulScriptExecutionMetadata({ status: "running" })).toBe(false);
  });
});

describe("hasFailedAgentBootAfter", () => {
  it("returns true when a boot after the failure reports status failed", () => {
    expect(hasFailedAgentBootAfter([bootMessage(AFTER, { status: "failed" })], ERROR_AT)).toBe(
      true,
    );
  });

  it("returns true when a boot after the failure exits non-zero", () => {
    expect(hasFailedAgentBootAfter([bootMessage(AFTER, { exit_code: 1 })], ERROR_AT)).toBe(true);
  });

  it("returns false for a successful boot", () => {
    expect(hasFailedAgentBootAfter([bootMessage(AFTER)], ERROR_AT)).toBe(false);
  });

  it("returns false while the boot is still running", () => {
    expect(hasFailedAgentBootAfter([bootMessage(AFTER, { status: "running" })], ERROR_AT)).toBe(
      false,
    );
  });

  it("returns false when the failed boot predates the failure", () => {
    expect(hasFailedAgentBootAfter([bootMessage(BEFORE, { status: "failed" })], ERROR_AT)).toBe(
      false,
    );
  });

  it("ignores non-boot scripts that failed after the failure", () => {
    const setup = bootMessage(AFTER, { script_type: "setup", status: "failed" });
    expect(hasFailedAgentBootAfter([setup], ERROR_AT)).toBe(false);
  });
});
