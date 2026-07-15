import { describe, it, expect } from "vitest";
import { agentProfileId as toAgentProfileId, type AgentProfile } from "@/lib/types/http";
import {
  isProfileDirty,
  mergeSavedAgentDraft,
  toAgentProfilePatch,
  type DraftAgent,
  type DraftProfile,
} from "./agent-save-helpers";
import type { ProfileFormData } from "@/components/settings/profile-form-fields";

const baseProfile: AgentProfile = {
  id: toAgentProfileId("p1"),
  agentId: "a1",
  name: "Profile",
  agentDisplayName: "Mock",
  model: "mock-fast",
  mode: "default",
  allowIndexing: false,
  autoApprove: false,
  cliFlags: [],
  cliPassthrough: false,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

const draftFrom = (saved: AgentProfile, overrides: Partial<DraftProfile> = {}): DraftProfile => ({
  ...saved,
  ...overrides,
});

const ALLOW_ALL_TOOLS_FLAG = "--allow-all-tools";

describe("toAgentProfilePatch", () => {
  it("maps snake_case form keys to camelCase AgentProfile fields", () => {
    const patch: Partial<ProfileFormData> = {
      name: "CLI",
      model: "claude-sonnet",
      mode: "default",
      allow_indexing: true,
      auto_approve: true,
      cli_passthrough: true,
      cli_flags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "" }],
    };
    expect(toAgentProfilePatch(patch)).toEqual({
      name: "CLI",
      model: "claude-sonnet",
      mode: "default",
      allowIndexing: true,
      autoApprove: true,
      cliPassthrough: true,
      cliFlags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "" }],
    });
  });

  it("omits undefined keys so partial patches do not clobber unrelated fields", () => {
    expect(toAgentProfilePatch({ cli_passthrough: false })).toEqual({ cliPassthrough: false });
    expect(toAgentProfilePatch({})).toEqual({});
  });
});

describe("isProfileDirty", () => {
  it("returns false when draft equals saved", () => {
    expect(isProfileDirty(draftFrom(baseProfile), baseProfile)).toBe(false);
  });

  it("returns true when only mode changes", () => {
    const draft = draftFrom(baseProfile, { mode: "plan-mock" });
    expect(isProfileDirty(draft, baseProfile)).toBe(true);
  });

  it("treats undefined mode as equal to empty string", () => {
    const saved: AgentProfile = { ...baseProfile, mode: undefined };
    const draft = draftFrom(saved, { mode: "" });
    expect(isProfileDirty(draft, saved)).toBe(false);
  });

  it("returns true when mode changes from empty to a value", () => {
    const saved: AgentProfile = { ...baseProfile, mode: "" };
    const draft = draftFrom(saved, { mode: "plan-mock" });
    expect(isProfileDirty(draft, saved)).toBe(true);
  });

  it("returns true when mode changes from a value to empty (cleared)", () => {
    const saved: AgentProfile = { ...baseProfile, mode: "plan-mock" };
    const draft = draftFrom(saved, { mode: "" });
    expect(isProfileDirty(draft, saved)).toBe(true);
  });

  it("returns true when there is no saved profile", () => {
    expect(isProfileDirty(draftFrom(baseProfile))).toBe(true);
  });

  it("returns true when cliFlags list changes", () => {
    const draft = draftFrom(baseProfile, {
      cliFlags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "" }],
    });
    expect(isProfileDirty(draft, baseProfile)).toBe(true);
  });

  it("returns true when a cliFlag enabled state changes", () => {
    const saved: AgentProfile = {
      ...baseProfile,
      cliFlags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: false, description: "" }],
    };
    const draft = draftFrom(saved, {
      cliFlags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "" }],
    });
    expect(isProfileDirty(draft, saved)).toBe(true);
  });

  it("returns false when cliFlags are equal", () => {
    const flags = [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "desc" }];
    const saved: AgentProfile = { ...baseProfile, cliFlags: flags };
    const draft = draftFrom(saved, { cliFlags: [...flags] });
    expect(isProfileDirty(draft, saved)).toBe(false);
  });

  it("returns true when autoApprove changes via camelCase draft field", () => {
    const draft = draftFrom(baseProfile, { autoApprove: true });
    expect(isProfileDirty(draft, baseProfile)).toBe(true);
  });

  it("returns false when stale snake_case auto_approve disagrees with camelCase", () => {
    const draft = draftFrom(baseProfile, { auto_approve: true, autoApprove: false });
    expect(isProfileDirty(draft, baseProfile)).toBe(false);
  });
});

describe("mergeSavedAgentDraft", () => {
  it("remaps created profile IDs while preserving edits made during save", () => {
    const submittedProfile = draftFrom(baseProfile, {
      id: toAgentProfileId("draft-profile"),
      name: "Submitted name",
    });
    const submitted = agentWithProfiles([submittedProfile]);
    const current = {
      ...submitted,
      workspace_id: "newer-workspace",
      profiles: [{ ...submittedProfile, name: "Newer name" }],
    };
    const saved = agentWithProfiles([
      { ...submittedProfile, id: toAgentProfileId("persisted-profile") },
    ]);

    const merged = mergeSavedAgentDraft(current, submitted, saved);

    expect(merged.workspace_id).toBe("newer-workspace");
    expect(merged.profiles[0].id).toBe(toAgentProfileId("persisted-profile"));
    expect(merged.profiles[0].name).toBe("Newer name");
  });
});

function agentWithProfiles(profiles: DraftProfile[]): DraftAgent {
  return {
    id: "agent-1",
    name: "mock-agent",
    supports_mcp: true,
    profiles,
    created_at: "",
    updated_at: "",
  };
}
