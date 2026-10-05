import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
  AgentModelConfigResponse,
  DynamicModelsResponse,
  ModelConfig,
  ProfileCapabilityRequest,
  ResolveAgentModelConfigRequest,
} from "@/lib/types/http";
import type { AgentUpdateJob } from "@/lib/api";

const MOCK_AGENT_NAME = "mock-agent";
const PROFILE_ID_A = "profile-a";
const REVISION_A = "revision-a";
const REVISION_B = "revision-b";
const DRAFT_A = "draft-a";
const DRAFT_B = "draft-b";
const SAVED_MODEL_ID = "saved-model";
const SAVED_MODEL_NAME = "Saved model";
const CATALOG_SELECTOR_DESCRIPTION = "catalog selector";
const DRAFT_B_MODEL_ID = "draft-b-model";
const DRAFT_B_MODEL_NAME = "Draft B model";
const probeAgentProfileMock = vi.fn();
const resolveAgentModelConfigMock = vi.fn();

vi.mock("@/lib/api/domains/settings-api", () => ({
  resolveAgentModelConfig: (...args: unknown[]) => resolveAgentModelConfigMock(...args),
}));

vi.mock("@/lib/api/domains/profile-capability-api", () => ({
  probeAgentProfile: (...args: unknown[]) => probeAgentProfileMock(...args),
}));

import { useProfileModelCapabilities } from "./use-profile-model-capabilities";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";

let profileStore: ReturnType<typeof useAppStoreApi> | null = null;

function StoreCapture() {
  profileStore = useAppStoreApi();
  return null;
}

function profileStoreWrapper({ children }: { children: ReactNode }) {
  return (
    <StateProvider>
      <StoreCapture />
      {children}
    </StateProvider>
  );
}

function updateJob(overrides: Partial<AgentUpdateJob> = {}): AgentUpdateJob {
  return {
    job_id: "activation-1",
    agent_name: MOCK_AGENT_NAME,
    status: "succeeded",
    started_at: "2026-01-01T00:00:00.000Z",
    finished_at: "2026-01-01T00:01:00.000Z",
    ...overrides,
  };
}

const modelConfig: ModelConfig = {
  default_model: SAVED_MODEL_ID,
  available_models: [],
  supports_dynamic_models: true,
  status: "ok",
};

function capabilityResponse(
  models: { id: string; name: string }[],
  contextRevision: string,
  observedVersion?: string,
): DynamicModelsResponse {
  return {
    agent_name: MOCK_AGENT_NAME,
    status: "ok",
    models,
    modes: [{ id: "default", name: "Default" }],
    commands: [],
    current_model_id: models[0]?.id,
    current_mode_id: "default",
    context_revision: contextRevision,
    ...(observedVersion
      ? {
          runtime_info: {
            scope: "host",
            observed_at: "2026-01-01T00:00:00.000Z",
            components: [
              {
                role: "bridge",
                name: "ACP bridge",
                source: "managed",
                owner: "kandev",
                effective_version: "1.1.0",
                observed_version: observedVersion,
              },
            ],
          },
        }
      : {}),
    error: null,
  };
}

function optionResponse(model: string, contextRevision: string): AgentModelConfigResponse {
  return {
    agent_name: MOCK_AGENT_NAME,
    model,
    status: "ok",
    config_options: [],
    context_revision: contextRevision,
    error: null,
  };
}

const savedLaunchSettings = {
  env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "saved" }],
  cli_flags: [],
  command_prefix: "",
};

const initialProfile = {
  model: SAVED_MODEL_ID,
  mode: "",
  env_vars: savedLaunchSettings.env_vars,
  cli_flags: savedLaunchSettings.cli_flags,
  command_prefix: savedLaunchSettings.command_prefix,
};

afterEach(() => {
  cleanup();
  profileStore = null;
  probeAgentProfileMock.mockReset();
  resolveAgentModelConfigMock.mockReset();
});

describe("useProfileModelCapabilities", () => {
  it("discovers a saved profile and resolves options in the same profile context", async () => {
    probeAgentProfileMock.mockResolvedValueOnce(
      capabilityResponse([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }], REVISION_A),
    );
    resolveAgentModelConfigMock.mockImplementation(
      async (_agent: string, request: ResolveAgentModelConfigRequest) =>
        optionResponse(request.model, REVISION_A),
    );

    const { result } = renderHook(() =>
      useProfileModelCapabilities(MOCK_AGENT_NAME, initialProfile, modelConfig, vi.fn(), {
        profileId: PROFILE_ID_A,
        savedLaunchSettings,
      }),
    );

    await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
    await waitFor(() => expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1));
    expect(probeAgentProfileMock).toHaveBeenCalledWith(MOCK_AGENT_NAME, {
      profile_id: PROFILE_ID_A,
    });
    expect(resolveAgentModelConfigMock).toHaveBeenCalledWith(MOCK_AGENT_NAME, {
      model: SAVED_MODEL_ID,
      profile_id: PROFILE_ID_A,
    });
    expect(result.current.capabilities.models).toEqual([
      { id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME },
    ]);
    expect(result.current.configStatus).toBe("ok");
  });

  it("does not repeat option discovery when equivalent config values are recreated", async () => {
    probeAgentProfileMock.mockResolvedValueOnce(
      capabilityResponse([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }], REVISION_A),
    );
    resolveAgentModelConfigMock.mockImplementation(
      async (_agent: string, request: ResolveAgentModelConfigRequest) =>
        optionResponse(request.model, REVISION_A),
    );

    const profile = { ...initialProfile, config_options: { effort: "medium" } };
    const { result, rerender } = renderHook(
      ({ currentProfile }) =>
        useProfileModelCapabilities(MOCK_AGENT_NAME, currentProfile, modelConfig, vi.fn(), {
          profileId: PROFILE_ID_A,
          savedLaunchSettings,
        }),
      { initialProps: { currentProfile: profile } },
    );

    await waitFor(() => expect(result.current.configStatus).toBe("ok"));
    expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1);

    rerender({ currentProfile: { ...profile, config_options: { effort: "medium" } } });

    expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1);
  });

  it("keeps failed model-option discovery retryable in the same profile context", async () => {
    probeAgentProfileMock.mockResolvedValueOnce(
      capabilityResponse([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }], REVISION_A),
    );
    resolveAgentModelConfigMock
      .mockRejectedValueOnce(new Error("temporary probe failure"))
      .mockResolvedValueOnce(optionResponse(SAVED_MODEL_ID, REVISION_A));

    const { result } = renderHook(() =>
      useProfileModelCapabilities(MOCK_AGENT_NAME, initialProfile, modelConfig, vi.fn(), {
        profileId: PROFILE_ID_A,
        savedLaunchSettings,
      }),
    );

    await waitFor(() => expect(result.current.configStatus).toBe("failed"));
    expect(result.current.configIsLoading).toBe(false);
    expect(result.current.configError).toBe("temporary probe failure");

    await act(async () => {
      await result.current.refreshModelConfig();
    });

    expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(2);
    expect(result.current.configStatus).toBe("ok");
    expect(result.current.configIsLoading).toBe(false);
  });
});

describe("profile discovery after runtime activation", () => {
  it("refreshes the current draft once after a matching successful update", async () => {
    probeAgentProfileMock
      .mockResolvedValueOnce(
        capabilityResponse([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }], REVISION_A, "1.0.0"),
      )
      .mockResolvedValueOnce(
        capabilityResponse([{ id: "new-model", name: "New model" }], REVISION_B, "2.0.0"),
      );

    const onChange = vi.fn();
    const { result, rerender } = renderHook(
      ({ profile }) =>
        useProfileModelCapabilities(MOCK_AGENT_NAME, profile, modelConfig, onChange, {
          profileId: PROFILE_ID_A,
          savedLaunchSettings,
        }),
      { initialProps: { profile: initialProfile }, wrapper: profileStoreWrapper },
    );

    await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
    expect(probeAgentProfileMock).toHaveBeenCalledTimes(1);

    const draft = {
      ...initialProfile,
      env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "activated-draft" }],
      command_prefix: "mock-agent --activated-profile",
    };
    rerender({ profile: draft });
    expect(result.current.discoveryState).toBe("stale");

    await act(async () => {
      profileStore!.getState().upsertAgentUpdateJob(updateJob());
    });

    await waitFor(() => expect(probeAgentProfileMock).toHaveBeenCalledTimes(2));
    expect(probeAgentProfileMock).toHaveBeenLastCalledWith(MOCK_AGENT_NAME, {
      profile_id: PROFILE_ID_A,
      launch_settings: {
        env_vars: [
          {
            key: "MOCK_AGENT_PROFILE_CATALOG",
            value: "activated-draft",
            secret_id: undefined,
          },
        ],
        cli_flags: [],
        command_prefix: "mock-agent --activated-profile",
      },
      refresh: true,
    });
    expect(result.current.capabilities.models).toEqual([{ id: "new-model", name: "New model" }]);
    expect(result.current.discoveryState).toBe("ready");
    expect(result.current.runtimeInfo?.components[0]?.observed_version).toBe("2.0.0");
    expect(draft.model).toBe(SAVED_MODEL_ID);
    expect(onChange).not.toHaveBeenCalled();

    await act(async () => {
      profileStore!.getState().upsertAgentUpdateJob(updateJob());
    });
    expect(probeAgentProfileMock).toHaveBeenCalledTimes(2);
  });
});

describe("stale profile runtime observations", () => {
  it("keeps the last runtime observation visible when the launch draft changes", async () => {
    probeAgentProfileMock.mockResolvedValueOnce(
      capabilityResponse([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }], REVISION_A, "1.0.0"),
    );

    const { result, rerender } = renderHook(
      ({ profile }) =>
        useProfileModelCapabilities(MOCK_AGENT_NAME, profile, modelConfig, vi.fn(), {
          profileId: PROFILE_ID_A,
          savedLaunchSettings,
        }),
      { initialProps: { profile: initialProfile } },
    );

    await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
    rerender({
      profile: {
        ...initialProfile,
        env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "edited-draft" }],
      },
    });

    expect(result.current.discoveryState).toBe("stale");
    expect(result.current.capabilities.models).toEqual([]);
    expect(result.current.runtimeInfo?.components[0]?.observed_version).toBe("1.0.0");
  });
});

describe("runtime update job baselines", () => {
  it("ignores historical, failed, and unrelated runtime update jobs", async () => {
    const historical = updateJob({ job_id: "historical" });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <StateProvider initialState={{ updateJobs: { byAgent: { [MOCK_AGENT_NAME]: historical } } }}>
        <StoreCapture />
        {children}
      </StateProvider>
    );
    probeAgentProfileMock.mockResolvedValueOnce(
      capabilityResponse([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }], REVISION_A, "1.0.0"),
    );

    const { result } = renderHook(
      () =>
        useProfileModelCapabilities(MOCK_AGENT_NAME, initialProfile, modelConfig, vi.fn(), {
          profileId: PROFILE_ID_A,
          savedLaunchSettings,
        }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
    expect(probeAgentProfileMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      profileStore!
        .getState()
        .upsertAgentUpdateJob(updateJob({ job_id: "failed", status: "failed" }));
      profileStore!
        .getState()
        .upsertAgentUpdateJob(updateJob({ job_id: "unrelated", agent_name: "other-agent" }));
    });
    expect(probeAgentProfileMock).toHaveBeenCalledTimes(1);
    expect(result.current.capabilities.models).toEqual([
      { id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME },
    ]);
    expect(result.current.runtimeInfo?.components[0]?.observed_version).toBe("1.0.0");
  });
});

describe("activation refresh failure", () => {
  it("keeps the last successful snapshot when activation refresh fails", async () => {
    probeAgentProfileMock
      .mockResolvedValueOnce(
        capabilityResponse([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }], REVISION_A, "1.0.0"),
      )
      .mockRejectedValueOnce(new Error("temporary profile refresh failure"));

    const { result } = renderHook(
      () =>
        useProfileModelCapabilities(MOCK_AGENT_NAME, initialProfile, modelConfig, vi.fn(), {
          profileId: PROFILE_ID_A,
          savedLaunchSettings,
        }),
      { wrapper: profileStoreWrapper },
    );

    await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
    await act(async () => {
      profileStore!
        .getState()
        .upsertAgentUpdateJob(updateJob({ job_id: "activation-after-ready" }));
    });

    await waitFor(() => expect(result.current.discoveryState).toBe("failed"));
    expect(result.current.capabilities.models).toEqual([
      { id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME },
    ]);
    expect(result.current.runtimeInfo?.components[0]?.observed_version).toBe("1.0.0");
  });
});

describe("draft profile capability discovery", () => {
  it("does not probe a draft until refresh and rejects the result after launch settings change", async () => {
    let resolveOldProbe: ((response: DynamicModelsResponse) => void) | undefined;
    probeAgentProfileMock.mockImplementationOnce(
      () => new Promise<DynamicModelsResponse>((resolve) => (resolveOldProbe = resolve)),
    );

    const onChange = vi.fn();
    const draft = {
      ...initialProfile,
      env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: DRAFT_A }],
      cli_flags: [{ flag: "--catalog", enabled: true, description: CATALOG_SELECTOR_DESCRIPTION }],
      command_prefix: "npx --",
    };
    const { result, rerender } = renderHook(
      ({ profile }) => useProfileModelCapabilities(MOCK_AGENT_NAME, profile, modelConfig, onChange),
      { initialProps: { profile: draft } },
    );

    expect(probeAgentProfileMock).not.toHaveBeenCalled();
    expect(result.current.discoveryState).toBe("stale");

    let oldRefresh: Promise<void> | undefined;
    act(() => {
      oldRefresh = result.current.refresh();
    });
    await waitFor(() => expect(probeAgentProfileMock).toHaveBeenCalledTimes(1));
    expect(probeAgentProfileMock).toHaveBeenLastCalledWith(MOCK_AGENT_NAME, {
      launch_settings: {
        env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: DRAFT_A, secret_id: undefined }],
        cli_flags: [
          { flag: "--catalog", enabled: true, description: CATALOG_SELECTOR_DESCRIPTION },
        ],
        command_prefix: "npx --",
      },
      refresh: true,
    } satisfies ProfileCapabilityRequest);

    rerender({
      profile: { ...draft, env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: DRAFT_B }] },
    });
    expect(result.current.discoveryState).toBe("stale");
    await act(async () => {
      resolveOldProbe?.(capabilityResponse([{ id: "old-model", name: "Old model" }], "old"));
      await oldRefresh;
    });
    expect(result.current.capabilities.models).toEqual([]);

    probeAgentProfileMock.mockResolvedValueOnce(
      capabilityResponse([{ id: DRAFT_B_MODEL_ID, name: DRAFT_B_MODEL_NAME }], REVISION_B),
    );
    resolveAgentModelConfigMock.mockImplementation(
      async (_agent: string, request: ResolveAgentModelConfigRequest) =>
        optionResponse(request.model, REVISION_B),
    );
    await act(async () => {
      await result.current.refresh();
    });

    await waitFor(() => expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1));
    expect(probeAgentProfileMock).toHaveBeenLastCalledWith(MOCK_AGENT_NAME, {
      launch_settings: {
        env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: DRAFT_B, secret_id: undefined }],
        cli_flags: [
          { flag: "--catalog", enabled: true, description: CATALOG_SELECTOR_DESCRIPTION },
        ],
        command_prefix: "npx --",
      },
      refresh: true,
    });
    expect(resolveAgentModelConfigMock).toHaveBeenCalledWith(MOCK_AGENT_NAME, {
      model: SAVED_MODEL_ID,
      launch_settings: {
        env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: DRAFT_B, secret_id: undefined }],
        cli_flags: [
          { flag: "--catalog", enabled: true, description: CATALOG_SELECTOR_DESCRIPTION },
        ],
        command_prefix: "npx --",
      },
    });
    expect(result.current.capabilities.models).toEqual([
      { id: DRAFT_B_MODEL_ID, name: DRAFT_B_MODEL_NAME },
    ]);

    rerender({
      profile: {
        ...draft,
        model: DRAFT_B_MODEL_ID,
        env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: DRAFT_B }],
      },
    });
    await waitFor(() => expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(2));
    expect(resolveAgentModelConfigMock).toHaveBeenLastCalledWith(MOCK_AGENT_NAME, {
      model: DRAFT_B_MODEL_ID,
      launch_settings: {
        env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: DRAFT_B, secret_id: undefined }],
        cli_flags: [
          { flag: "--catalog", enabled: true, description: CATALOG_SELECTOR_DESCRIPTION },
        ],
        command_prefix: "npx --",
      },
    });
    expect(onChange).not.toHaveBeenCalled();
  });
});
