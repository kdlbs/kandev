import { describe, expect, it } from "vitest";
import {
  buildRunErrorsFromSessions,
  liveSessionMetadataFromStore,
  mergeLiveSessionMetadata,
} from "./chat-entries";
import type { TaskSession } from "@/app/office/tasks/[id]/types";

const URL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go";

function session(overrides: Partial<TaskSession>): TaskSession {
  return {
    id: "s1",
    agentName: "CEO",
    agentRole: "agent",
    state: "FAILED",
    isPrimary: false,
    startedAt: "2026-08-02T15:00:00Z",
    errorMessage: "usage limit reached",
    ...overrides,
  };
}

describe("buildRunErrorsFromSessions", () => {
  it("preserves the remediation URL from last_agent_error metadata", () => {
    const errors = buildRunErrorsFromSessions([
      session({
        metadata: {
          last_agent_error: {
            message: "usage limit reached",
            remediation_url: URL,
          },
        },
      }),
    ]);
    expect(errors).toHaveLength(1);
    expect(errors[0].remediationUrl).toBe(URL);
    expect(errors[0].rawPayload).toBe("usage limit reached");
  });

  it("reads the camelCase form after a store round trip", () => {
    const errors = buildRunErrorsFromSessions([
      session({ metadata: { last_agent_error: { message: "boom", remediationUrl: URL } } }),
    ]);
    expect(errors[0].remediationUrl).toBe(URL);
  });

  it("omits the URL when metadata or the field is absent", () => {
    for (const s of [
      session({ errorMessage: "boom" }),
      session({ metadata: { last_agent_error: { message: "boom" } } }),
      session({ metadata: { last_agent_error: { message: "boom", remediation_url: "" } } }),
    ]) {
      const errors = buildRunErrorsFromSessions([s]);
      expect(errors[0].remediationUrl).toBeUndefined();
    }
  });

  it("drops an unvalidated URL before it reaches RunError", () => {
    const errors = buildRunErrorsFromSessions([
      session({
        metadata: {
          last_agent_error: {
            message: "boom",
            remediation_url: "https://evil.example.com/workspace/wrk_123/go",
          },
        },
      }),
    ]);
    expect(errors[0].remediationUrl).toBeUndefined();
  });

  it("skips non-FAILED sessions", () => {
    const errors = buildRunErrorsFromSessions([
      session({ state: "RUNNING", metadata: { last_agent_error: { remediation_url: URL } } }),
    ]);
    expect(errors).toHaveLength(0);
  });
});

describe("mergeLiveSessionMetadata", () => {
  const initial = { last_agent_error: { message: "boom", remediation_url: URL } };

  it("keeps the initial fetch when the store has no metadata update", () => {
    expect(mergeLiveSessionMetadata(initial, undefined)).toBe(initial);
    expect(mergeLiveSessionMetadata(null, undefined)).toBeNull();
  });

  it("preserves an explicit server-side null so cleared metadata is not resurrected", () => {
    expect(mergeLiveSessionMetadata(initial, null)).toBeNull();
  });

  it("prefers live metadata over the initial fetch", () => {
    const live = { last_agent_error: { message: "newer" } };
    expect(mergeLiveSessionMetadata(initial, live)).toBe(live);
  });
});

describe("liveSessionMetadataFromStore", () => {
  const metadata = { last_agent_error: { message: "boom", remediation_url: URL } };

  it("returns undefined when no store row exists", () => {
    expect(liveSessionMetadataFromStore({}, "s1")).toBeUndefined();
  });

  it("treats a partial row without a metadata field as metadata-absent", () => {
    // Regression: a store row without `metadata` (e.g. a summary projection)
    // must NOT become explicit null, which would erase the initial fetch's
    // metadata and hide the remediation link.
    expect(liveSessionMetadataFromStore({ s1: {} }, "s1")).toBeUndefined();
    expect(mergeLiveSessionMetadata(metadata, liveSessionMetadataFromStore({ s1: {} }, "s1"))).toBe(
      metadata,
    );
  });

  it("preserves an explicit null metadata field", () => {
    expect(liveSessionMetadataFromStore({ s1: { metadata: null } }, "s1")).toBeNull();
  });

  it("returns the live metadata object when present", () => {
    expect(liveSessionMetadataFromStore({ s1: { metadata } }, "s1")).toBe(metadata);
  });
});
