import { beforeEach, describe, it, expect, vi } from "vitest";
import {
  createAgentAction,
  createAgentProfileAction,
  updateAgentProfileAction,
} from "@/app/actions/agents";
import { agentProfileId as toAgentProfileId, type AgentProfile } from "@/lib/types/http";
import {
  isProfileDirty,
  saveExistingAgent,
  saveNewAgent,
  type SaveAgentCallbacks,
  type DraftAgent,
  type DraftProfile,
} from "./agent-save-helpers";

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

const PERSISTED_PROFILE_ID = toAgentProfileId("persisted-profile");
const DRAFT_PROFILE_ID = toAgentProfileId("draft-profile");
const NEW_PROFILE_ID = toAgentProfileId("draft-new-profile");
const NEW_PROFILE_NAME = "New profile";

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

describe("fallback model save payloads", () => {
  it("includes fallback_model and auto_fallback when saving a dirty existing profile", async () => {
    const savedAgent = agentWithProfiles([baseProfile]);
    const draftProfile = draftFrom(baseProfile, {
      fallbackModel: "gpt-5",
      autoFallback: true,
    });
    const draftAgent = agentWithProfiles([draftProfile]);
    const { callbacks } = createTestCallbacks(draftAgent);
    vi.mocked(updateAgentProfileAction).mockResolvedValue({
      ...baseProfile,
      fallbackModel: "gpt-5",
      autoFallback: true,
    });

    await saveExistingAgent(draftAgent, savedAgent, false, callbacks);

    expect(updateAgentProfileAction).toHaveBeenCalledWith(
      baseProfile.id,
      expect.objectContaining({ fallback_model: "gpt-5", auto_fallback: true }),
    );
  });

  it("includes the fallback fields when creating a new agent's profile", async () => {
    const draftProfile = draftFrom(baseProfile, {
      id: DRAFT_PROFILE_ID,
      fallbackModel: "gpt-5",
      autoFallback: true,
    });
    const draftAgent = agentWithProfiles([draftProfile]);
    const { callbacks } = createTestCallbacks(draftAgent);
    vi.mocked(createAgentAction).mockResolvedValue(
      agentWithProfiles([{ ...draftProfile, id: PERSISTED_PROFILE_ID }]),
    );

    await saveNewAgent(draftAgent, callbacks);

    expect(createAgentAction).toHaveBeenCalledWith(
      expect.objectContaining({
        profiles: [expect.objectContaining({ fallback_model: "gpt-5", auto_fallback: true })],
      }),
    );
  });

  it("includes the fallback fields when adding a new profile to an existing agent", async () => {
    const savedAgent = agentWithProfiles([baseProfile]);
    const newProfile = draftFrom(baseProfile, {
      id: NEW_PROFILE_ID,
      name: NEW_PROFILE_NAME,
      fallbackModel: "gpt-5",
      autoFallback: true,
    });
    const draftAgent = agentWithProfiles([baseProfile, newProfile]);
    const { callbacks } = createTestCallbacks(draftAgent);
    vi.mocked(createAgentProfileAction).mockResolvedValue({
      ...newProfile,
      id: PERSISTED_PROFILE_ID,
    });

    await saveExistingAgent(draftAgent, savedAgent, false, callbacks);

    expect(createAgentProfileAction).toHaveBeenCalledWith(
      savedAgent.id,
      expect.objectContaining({ fallback_model: "gpt-5", auto_fallback: true }),
    );
  });
});

describe("provider config save payloads", () => {
  const OPENAI_COMPATIBLE = "openai_compatible";
  const BASE_URL = "http://localhost:20128/v1";

  it("returns true when a provider field changes", () => {
    const draft = draftFrom(baseProfile, {
      providerKind: OPENAI_COMPATIBLE,
      providerBaseUrl: BASE_URL,
    });
    expect(isProfileDirty(draft, baseProfile)).toBe(true);
  });

  it("includes provider fields when saving a dirty existing profile", async () => {
    const savedAgent = agentWithProfiles([baseProfile]);
    const draftProfile = draftFrom(baseProfile, {
      providerKind: OPENAI_COMPATIBLE,
      providerBaseUrl: BASE_URL,
      providerApiKeySecretId: "sec_1",
    });
    const draftAgent = agentWithProfiles([draftProfile]);
    const { callbacks } = createTestCallbacks(draftAgent);
    vi.mocked(updateAgentProfileAction).mockResolvedValue({ ...baseProfile });

    await saveExistingAgent(draftAgent, savedAgent, false, callbacks);

    expect(updateAgentProfileAction).toHaveBeenCalledWith(
      baseProfile.id,
      expect.objectContaining({
        provider_kind: OPENAI_COMPATIBLE,
        provider_base_url: BASE_URL,
        provider_api_key_secret_id: "sec_1",
      }),
    );
  });

  it("includes provider fields when adding a new profile to an existing agent", async () => {
    const savedAgent = agentWithProfiles([baseProfile]);
    const newProfile = draftFrom(baseProfile, {
      id: NEW_PROFILE_ID,
      name: NEW_PROFILE_NAME,
      providerKind: OPENAI_COMPATIBLE,
      providerBaseUrl: BASE_URL,
    });
    const draftAgent = agentWithProfiles([baseProfile, newProfile]);
    const { callbacks } = createTestCallbacks(draftAgent);
    vi.mocked(createAgentProfileAction).mockResolvedValue({
      ...newProfile,
      id: PERSISTED_PROFILE_ID,
    });

    await saveExistingAgent(draftAgent, savedAgent, false, callbacks);

    expect(createAgentProfileAction).toHaveBeenCalledWith(
      savedAgent.id,
      expect.objectContaining({ provider_kind: OPENAI_COMPATIBLE, provider_base_url: BASE_URL }),
    );
  });
});
