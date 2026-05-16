import { describe, it, expect } from "vitest";
import { normalizeAgentProfile, toAgentProfilePayload } from "./agent-profile-normalize";

describe("normalizeAgentProfile", () => {
  it("converts snake_case wire payload to canonical camelCase", () => {
    const wire = {
      id: "p1",
      agent_id: "claude",
      name: "default",
      agent_display_name: "Claude Code",
      model: "claude-sonnet-4-5",
      mode: "acp",
      allow_indexing: true,
      cli_flags: [{ flag: "--verbose", description: "v", enabled: true }],
      cli_passthrough: false,
      user_modified: true,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-02T00:00:00Z",
    };
    const result = normalizeAgentProfile(wire);
    expect(result).toEqual({
      id: "p1",
      name: "default",
      agentId: "claude",
      agentDisplayName: "Claude Code",
      model: "claude-sonnet-4-5",
      mode: "acp",
      allowIndexing: true,
      cliFlags: [{ flag: "--verbose", description: "v", enabled: true }],
      cliPassthrough: false,
      userModified: true,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-02T00:00:00Z",
    });
  });

  it("falls back to safe defaults for missing fields", () => {
    const result = normalizeAgentProfile({ id: "x", name: "y" });
    expect(result.cliFlags).toEqual([]);
    expect(result.cliPassthrough).toBe(false);
    expect(result.allowIndexing).toBe(false);
    expect(result.agentDisplayName).toBe("");
  });

  it("accepts already-camelCase input", () => {
    const result = normalizeAgentProfile({
      id: "p1",
      name: "default",
      agentId: "codex",
      cliPassthrough: true,
    });
    expect(result.agentId).toBe("codex");
    expect(result.cliPassthrough).toBe(true);
  });
});

describe("toAgentProfilePayload", () => {
  it("converts canonical camelCase back to snake_case wire shape", () => {
    const payload = toAgentProfilePayload({
      id: "p1",
      agentId: "claude",
      name: "default",
      cliPassthrough: false,
      cliFlags: [],
    });
    expect(payload).toEqual({
      id: "p1",
      agent_id: "claude",
      name: "default",
      cli_passthrough: false,
      cli_flags: [],
    });
  });

  it("omits undefined fields rather than emitting nullish keys", () => {
    const payload = toAgentProfilePayload({ id: "p1", name: "x" });
    expect(payload).toEqual({ id: "p1", name: "x" });
    expect("agent_id" in payload).toBe(false);
  });
});
