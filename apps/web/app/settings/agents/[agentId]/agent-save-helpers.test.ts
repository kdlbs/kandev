import { beforeEach, describe, it, expect, vi } from "vitest";
import {
  createAgentAction,
  createAgentProfileAction,
  updateAgentProfileAction,
  updateAgentProfileMcpConfigAction,
} from "@/app/actions/agents";
import { agentProfileId as toAgentProfileId, type AgentProfile } from "@/lib/types/http";
import {
  isProfileDirty,
  mergeSavedAgentDraft,
  saveExistingAgent,
  saveNewAgent,
  toAgentProfilePatch,
  type SaveAgentCallbacks,
  type DraftAgent,
  type DraftProfile,
} from "./agent-save-helpers";
import type { ProfileFormData } from "@/components/settings/profile-form-fields";

vi.mock("@/app/actions/agents", () => ({
  createAgentAction: vi.fn(),
  createAgentProfileAction: vi.fn(),
  deleteAgentProfileAction: vi.fn(),
  updateAgentAction: vi.fn(),
  updateAgentProfileAction: vi.fn(),
  updateAgentProfileMcpConfigAction: vi.fn(),
}));

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
const PERSISTED_PROFILE_ID = toAgentProfileId("persisted-profile");

beforeEach(() => {
  vi.clearAllMocks();
});

function createTestCallbacks(initialDraft: DraftAgent) {
  let currentDraft = initialDraft;
  const upsertAgent = vi.fn();
  const replaceRoute = vi.fn();
  const callbacks: SaveAgentCallbacks = {
    onToastError: vi.fn(),
    currentAgentModelConfig: {
      default_model: "mock-fast",
      available_models: [],
      supports_dynamic_models: false,
    },
    permissionSettings: {},
    resolveDisplayName: () => "Mock",
    upsertAgent,
    setDraftAgent: (value) => {
      currentDraft = typeof value === "function" ? value(currentDraft) : value;
    },
    ensureProfiles: (agent) => agent,
    cloneAgent: (agent) => ({
      ...agent,
      profiles: agent.profiles.map((profile) => ({ ...profile })),
    }),
    replaceRoute,
  };
  return { callbacks, upsertAgent, replaceRoute, getDraft: () => currentDraft };
}

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
    const saved = agentWithProfiles([{ ...submittedProfile, id: PERSISTED_PROFILE_ID }]);

    const merged = mergeSavedAgentDraft(
      current,
      submitted,
      saved,
      new Map([[submittedProfile.id, PERSISTED_PROFILE_ID]]),
    );

    expect(merged.workspace_id).toBe("newer-workspace");
    expect(merged.profiles[0].id).toBe(PERSISTED_PROFILE_ID);
    expect(merged.profiles[0].name).toBe("Newer name");
  });

  it("keeps existing profiles while remapping a newly created profile", () => {
    const existing = draftFrom(baseProfile, { id: toAgentProfileId("existing") });
    const submittedProfile = draftFrom(baseProfile, {
      id: toAgentProfileId("draft-profile"),
      name: "Submitted",
    });
    const persisted = { ...submittedProfile, id: PERSISTED_PROFILE_ID };
    const submitted = agentWithProfiles([submittedProfile]);
    const current = agentWithProfiles([{ ...submittedProfile, name: "Newer" }]);
    const saved = agentWithProfiles([existing, persisted]);

    const merged = mergeSavedAgentDraft(
      current,
      submitted,
      saved,
      new Map([[submittedProfile.id, persisted.id]]),
    );

    expect(merged.profiles.map((profile) => profile.id)).toEqual([existing.id, persisted.id]);
    expect(merged.profiles[1].name).toBe("Newer");
  });
});

describe("saveNewAgent", () => {
  it("reconciles a created agent so a failed MCP write retries without another create", async () => {
    const draftProfile = draftFrom(baseProfile, {
      id: toAgentProfileId("draft-profile"),
      mcp_config: {
        enabled: true,
        servers: '{"mcpServers":{"playwright":{"command":"npx"}}}',
        dirty: true,
        error: null,
      },
    });
    const draftAgent = agentWithProfiles([draftProfile]);
    const created = agentWithProfiles([
      { ...draftProfile, id: PERSISTED_PROFILE_ID, mcp_config: undefined },
    ]);
    const { callbacks, upsertAgent, replaceRoute, getDraft } = createTestCallbacks(draftAgent);
    const failure = new Error("MCP unavailable");
    vi.mocked(createAgentAction).mockResolvedValue(created);
    vi.mocked(updateAgentProfileMcpConfigAction)
      .mockRejectedValueOnce(failure)
      .mockResolvedValueOnce({
        profile_id: PERSISTED_PROFILE_ID,
        enabled: true,
        servers: {},
        meta: {},
      });

    await expect(saveNewAgent(draftAgent, callbacks)).rejects.toBe(failure);

    const reconciled = upsertAgent.mock.calls[0][0];
    expect(reconciled.profiles[0]).toMatchObject({
      id: PERSISTED_PROFILE_ID,
      mcp_config: { dirty: true },
    });
    expect(getDraft().profiles[0]).toMatchObject({
      id: PERSISTED_PROFILE_ID,
      mcp_config: { dirty: true },
    });
    expect(replaceRoute).toHaveBeenCalledWith("/settings/agents/mock-agent");

    const savedDraft = await saveExistingAgent(getDraft(), reconciled, false, callbacks);

    expect(createAgentAction).toHaveBeenCalledOnce();
    expect(createAgentProfileAction).not.toHaveBeenCalled();
    expect(updateAgentProfileMcpConfigAction).toHaveBeenCalledTimes(2);
    expect(savedDraft.profiles[0].mcp_config).toBeUndefined();
  });
});

describe("saveExistingAgent", () => {
  it("reconciles a created profile so a failed MCP write retries without duplication", async () => {
    const newProfile = draftFrom(baseProfile, {
      id: toAgentProfileId("draft-new-profile"),
      name: "New profile",
      mcp_config: {
        enabled: true,
        servers: '{"mcpServers":{"playwright":{"command":"npx"}}}',
        dirty: true,
        error: null,
      },
    });
    const updatedExisting = { ...baseProfile, name: "Updated existing profile" };
    const savedAgent = agentWithProfiles([baseProfile]);
    const draftAgent = agentWithProfiles([updatedExisting, newProfile]);
    const createdProfile = { ...newProfile, id: PERSISTED_PROFILE_ID, mcp_config: undefined };
    const { callbacks, upsertAgent, getDraft } = createTestCallbacks(draftAgent);
    const failure = new Error("MCP unavailable");
    vi.mocked(updateAgentProfileAction).mockResolvedValue(updatedExisting);
    vi.mocked(createAgentProfileAction).mockResolvedValue(createdProfile);
    vi.mocked(updateAgentProfileMcpConfigAction)
      .mockRejectedValueOnce(failure)
      .mockResolvedValueOnce({
        profile_id: PERSISTED_PROFILE_ID,
        enabled: true,
        servers: {},
      });

    await expect(saveExistingAgent(draftAgent, savedAgent, false, callbacks)).rejects.toBe(failure);

    const reconciled = upsertAgent.mock.calls[0][0];
    expect(reconciled.profiles[0].name).toBe("Updated existing profile");
    expect(reconciled.profiles[1]).toMatchObject({
      id: PERSISTED_PROFILE_ID,
      mcp_config: { dirty: true },
    });
    expect(getDraft().profiles[0].name).toBe("Updated existing profile");
    expect(getDraft().profiles[1]).toMatchObject({
      id: PERSISTED_PROFILE_ID,
      mcp_config: { dirty: true },
    });

    const savedDraft = await saveExistingAgent(getDraft(), reconciled, false, callbacks);

    expect(createAgentProfileAction).toHaveBeenCalledOnce();
    expect(updateAgentProfileAction).toHaveBeenCalledOnce();
    expect(updateAgentProfileMcpConfigAction).toHaveBeenCalledTimes(2);
    expect(savedDraft.profiles[1].mcp_config).toBeUndefined();
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
