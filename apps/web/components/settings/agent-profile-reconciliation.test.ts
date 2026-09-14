import { describe, expect, it } from "vitest";
import type { AgentProfile } from "@/lib/types/agent-profile";
import { reconcileAgentProfileSnapshot, sameEditableProfile } from "./agent-profile-reconciliation";

const BASE_UPDATED_AT = "2026-09-01T00:00:00Z";
const UPDATED_AT = "2026-09-01T01:00:00Z";
const LATEST_UPDATED_AT = "2026-09-01T02:00:00Z";

function profile(overrides: Partial<AgentProfile> = {}): AgentProfile {
  return {
    id: "profile-1" as AgentProfile["id"],
    name: "Default",
    agentId: "agent-1",
    agentDisplayName: "Agent",
    model: "model-a",
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: BASE_UPDATED_AT,
    updatedAt: BASE_UPDATED_AT,
    ...overrides,
  };
}

describe("sameEditableProfile", () => {
  it("ignores timestamps and object key order", () => {
    expect(
      sameEditableProfile(
        profile({ configOptions: { z: "2", a: "1" }, updatedAt: UPDATED_AT }),
        profile({ configOptions: { a: "1", z: "2" }, updatedAt: UPDATED_AT }),
      ),
    ).toBe(true);
  });
});

describe("reconcileAgentProfileSnapshot", () => {
  it("adopts a newer update when the editor is clean", () => {
    const previous = profile();
    const incoming = profile({ name: "Assistant", updatedAt: UPDATED_AT });
    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft: previous,
      saved: previous,
      conflicted: false,
    });

    expect(result.kind).toBe("clean-adopted");
    expect(result.draft.name).toBe("Assistant");
    expect(result.conflicted).toBe(false);
  });

  it("preserves a dirty draft and records a recoverable conflict", () => {
    const previous = profile();
    const draft = profile({ name: "Local draft" });
    const incoming = profile({ name: "Assistant", updatedAt: UPDATED_AT });
    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft,
      saved: previous,
      conflicted: false,
    });

    expect(result.kind).toBe("external-conflict");
    expect(result.draft.name).toBe("Local draft");
    expect(result.saved.name).toBe("Assistant");
    expect(result.conflicted).toBe(true);
  });

  it("treats the matching submitted snapshot as its own acknowledgement", () => {
    const previous = profile();
    const submitted = profile({ name: "Submitted" });
    const incoming = profile({ name: "Submitted", updatedAt: UPDATED_AT });
    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft: submitted,
      saved: previous,
      submitted,
      conflicted: false,
    });

    expect(result.kind).toBe("own-acknowledgement");
    expect(result.draft).toBe(incoming);
    expect(result.conflicted).toBe(false);
  });

  it("keeps edits made after submission when the acknowledgement arrives", () => {
    const previous = profile();
    const submitted = profile({ name: "Submitted" });
    const newerDraft = profile({ name: "Edited again" });
    const incoming = profile({ name: "Submitted", updatedAt: UPDATED_AT });
    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft: newerDraft,
      saved: previous,
      submitted,
      conflicted: false,
    });

    expect(result.kind).toBe("own-acknowledgement");
    expect(result.draft.name).toBe("Edited again");
    expect(result.saved.name).toBe("Submitted");
  });

  it("ignores a late response older than the current baseline", () => {
    const previous = profile({ name: "Current", updatedAt: LATEST_UPDATED_AT });
    const incoming = profile({ name: "Late", updatedAt: UPDATED_AT });
    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft: previous,
      saved: previous,
      conflicted: false,
    });

    expect(result.kind).toBe("ignored");
    expect(result.saved.name).toBe("Current");
  });
});

describe("reconcileAgentProfileSnapshot dynamic drafts", () => {
  it("preserves a dynamic candidate draft during an external routing update", () => {
    const candidate = {
      position: 0,
      executionProfileId: "profile-2" as AgentProfile["id"],
      enabled: true,
      policies: {
        version: 1,
        transient: {
          retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
          waitForReset: { enabled: false, maxWaitSeconds: 0 },
          onExhausted: "skip" as const,
        },
        hard: {
          retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
          waitForReset: { enabled: false, maxWaitSeconds: 0 },
          onExhausted: "skip" as const,
        },
      },
    };
    const previous = profile({ kind: "dynamic", dynamic: { version: 1, candidates: [] } });
    const draft = profile({ kind: "dynamic", dynamic: { version: 1, candidates: [candidate] } });
    const incoming = profile({
      kind: "dynamic",
      dynamic: { version: 2, candidates: [] },
      updatedAt: UPDATED_AT,
    });

    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft,
      saved: previous,
      conflicted: false,
    });

    expect(result.kind).toBe("external-conflict");
    expect(result.draft.dynamic?.candidates).toHaveLength(1);
    expect(result.saved.dynamic?.version).toBe(2);
  });
});
