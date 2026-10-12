import { describe, expect, it } from "vitest";
import type { AgentProfile } from "@/lib/types/agent-profile";
import { reconcileAgentProfileSnapshot, sameEditableProfile } from "./agent-profile-reconciliation";

const BASE_UPDATED_AT = "2026-09-01T00:00:00Z";
const UPDATED_AT = "2026-09-01T01:00:00Z";
const LATEST_UPDATED_AT = "2026-09-01T02:00:00Z";
const TEST_SERVER_ID = "plugin-atlassian-jira";
const LOCAL_DRAFT_NAME = "Local draft";
const SUBMITTED_NAME = "Submitted";
const ASSISTANT_NAME = "Assistant";
const EXTERNAL_CONFLICT = "external-conflict";

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

  it("treats provider-only edits as editable changes", () => {
    expect(
      sameEditableProfile(
        profile({ providerKind: "openai_compatible", providerBaseUrl: "http://router/v1" }),
        profile({ providerKind: "openai_compatible", providerBaseUrl: "http://other/v1" }),
      ),
    ).toBe(false);
  });

  it("treats Cursor MCP auth preference as an editable change", () => {
    expect(
      sameEditableProfile(
        profile({ cursorMcpAuthEnabled: true }),
        profile({ cursorMcpAuthEnabled: false }),
      ),
    ).toBe(false);
  });

  it("treats Cursor plugin MCP import preference as an editable change", () => {
    expect(
      sameEditableProfile(
        profile({ cursorPluginsMcpEnabled: true }),
        profile({ cursorPluginsMcpEnabled: false }),
      ),
    ).toBe(false);
  });

  it("treats profile MCP selection fields as editable changes", () => {
    const inherited = profile({ mcpSelectionMode: "inherit" } as Partial<AgentProfile>);
    const selected = profile({
      mcpSelectionMode: "selected",
      mcpSelectedServers: [TEST_SERVER_ID],
    } as Partial<AgentProfile>);

    expect(sameEditableProfile(inherited, selected)).toBe(false);
  });

  it("treats selected server IDs as an order-independent set", () => {
    const first = profile({
      mcpSelectionMode: "selected",
      mcpSelectedServers: [TEST_SERVER_ID, "filesystem"],
    } as Partial<AgentProfile>);
    const reordered = profile({
      mcpSelectionMode: "selected",
      mcpSelectedServers: ["filesystem", TEST_SERVER_ID],
    } as Partial<AgentProfile>);

    expect(sameEditableProfile(first, reordered)).toBe(true);
  });
});

describe("reconcileAgentProfileSnapshot clean and conflict", () => {
  it("adopts a newer update when the editor is clean", () => {
    const previous = profile();
    const incoming = profile({ name: ASSISTANT_NAME, updatedAt: UPDATED_AT });
    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft: previous,
      saved: previous,
      conflicted: false,
    });

    expect(result.kind).toBe("clean-adopted");
    expect(result.draft.name).toBe(ASSISTANT_NAME);
    expect(result.conflicted).toBe(false);
  });

  it("preserves a dirty draft and records a recoverable conflict", () => {
    const previous = profile();
    const draft = profile({ name: LOCAL_DRAFT_NAME });
    const incoming = profile({ name: ASSISTANT_NAME, updatedAt: UPDATED_AT });
    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft,
      saved: previous,
      conflicted: false,
    });

    expect(result.kind).toBe(EXTERNAL_CONFLICT);
    expect(result.draft.name).toBe(LOCAL_DRAFT_NAME);
    expect(result.saved.name).toBe(ASSISTANT_NAME);
    expect(result.conflicted).toBe(true);
  });
});

describe("reconcileAgentProfileSnapshot submission acknowledgements", () => {
  it("treats the matching submitted snapshot as its own acknowledgement", () => {
    const previous = profile();
    const submitted = profile({ name: SUBMITTED_NAME });
    const incoming = profile({ name: SUBMITTED_NAME, updatedAt: UPDATED_AT });
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
    const submitted = profile({ name: SUBMITTED_NAME });
    const newerDraft = profile({ name: "Edited again" });
    const incoming = profile({ name: SUBMITTED_NAME, updatedAt: UPDATED_AT });
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
    expect(result.saved.name).toBe(SUBMITTED_NAME);
  });
});

describe("reconcileAgentProfileSnapshot MCP preferences", () => {
  it("keeps a local Cursor MCP auth choice when a newer server snapshot conflicts", () => {
    const previous = profile({ cursorMcpAuthEnabled: true });
    const draft = profile({ cursorMcpAuthEnabled: false });
    const incoming = profile({ cursorMcpAuthEnabled: true, updatedAt: UPDATED_AT });
    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft,
      saved: previous,
      conflicted: false,
    });

    expect(result.kind).toBe(EXTERNAL_CONFLICT);
    expect(result.draft.cursorMcpAuthEnabled).toBe(false);
    expect(result.saved.cursorMcpAuthEnabled).toBe(true);
  });

  it("keeps local MCP selection edits when a newer server snapshot conflicts", () => {
    const previous = profile({
      mcpSelectionMode: "inherit",
      mcpSelectedServers: [],
    } as Partial<AgentProfile>);
    const draft = profile({
      mcpSelectionMode: "selected",
      mcpSelectedServers: [TEST_SERVER_ID],
    } as Partial<AgentProfile>);
    const incoming = profile({
      mcpSelectionMode: "inherit",
      mcpSelectedServers: [],
      updatedAt: UPDATED_AT,
    } as Partial<AgentProfile>);
    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft,
      saved: previous,
      conflicted: false,
    });

    expect(result.kind).toBe(EXTERNAL_CONFLICT);
    expect(
      (result.draft as AgentProfile & { mcpSelectedServers?: string[] }).mcpSelectedServers,
    ).toEqual([TEST_SERVER_ID]);
    expect(result.saved.updatedAt).toBe(UPDATED_AT);
  });
});

describe("reconcileAgentProfileSnapshot provider drafts", () => {
  it("preserves a provider edit made after submission", () => {
    const previous = profile({
      providerKind: "openai_compatible",
      providerBaseUrl: "http://old/v1",
    });
    const submitted = profile({
      providerKind: "openai_compatible",
      providerBaseUrl: "http://submitted/v1",
    });
    const newerDraft = profile({
      providerKind: "openai_compatible",
      providerBaseUrl: "http://edited-again/v1",
    });
    const incoming = profile({
      providerKind: "openai_compatible",
      providerBaseUrl: "http://submitted/v1",
      updatedAt: UPDATED_AT,
    });

    const result = reconcileAgentProfileSnapshot({
      previous,
      incoming,
      draft: newerDraft,
      saved: previous,
      submitted,
      conflicted: false,
    });

    expect(result.kind).toBe("own-acknowledgement");
    expect(result.draft.providerBaseUrl).toBe("http://edited-again/v1");
    expect(result.saved.providerBaseUrl).toBe("http://submitted/v1");
  });
});

describe("reconcileAgentProfileSnapshot stale responses", () => {
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
        unclassified: { enabled: false, consecutiveFailureThreshold: 0 },
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

    expect(result.kind).toBe(EXTERNAL_CONFLICT);
    expect(result.draft.dynamic?.candidates).toHaveLength(1);
    expect(result.saved.dynamic?.version).toBe(2);
  });
});
