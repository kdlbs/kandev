import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { DynamicModelsResponse, ProfileCapabilityRequest } from "@/lib/types/http";

const MOCK_AGENT_NAME = "mock-agent";
const PROFILE_ID_A = "profile-a";
const PROFILE_ID_B = "profile-b";
const REVISION_A = "revision-a";
const SAVED_MODEL_ID = "saved-model";
const SAVED_MODEL_NAME = "Saved model";

const probeAgentProfileMock = vi.fn();
const resolveAgentModelConfigMock = vi.fn();

vi.mock("@/lib/api/domains/settings-api", () => ({
  resolveAgentModelConfig: (...args: unknown[]) => resolveAgentModelConfigMock(...args),
}));

vi.mock("@/lib/api/domains/profile-capability-api", () => ({
  probeAgentProfile: (...args: unknown[]) => probeAgentProfileMock(...args),
}));

import { useProfileCapabilityDiscovery } from "./use-profile-capability-discovery";

function capabilityResponse(
  models: { id: string; name: string }[],
  contextRevision: string,
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
    error: null,
  };
}

const savedLaunchSettings = {
  env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "saved" }],
  cli_flags: [{ flag: "--catalog", enabled: true, description: "catalog" }],
  command_prefix: "npx --",
};

const savedProfile = {
  model: SAVED_MODEL_ID,
  mode: "",
  env_vars: savedLaunchSettings.env_vars,
  cli_flags: savedLaunchSettings.cli_flags,
  command_prefix: savedLaunchSettings.command_prefix,
};

afterEach(() => {
  cleanup();
  probeAgentProfileMock.mockReset();
  resolveAgentModelConfigMock.mockReset();
});

it("automatically discovers matching baseline models for a saved concrete profile without manual refresh", async () => {
  probeAgentProfileMock.mockResolvedValueOnce(
    capabilityResponse([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }], REVISION_A),
  );

  const { result } = renderHook(() =>
    useProfileCapabilityDiscovery(MOCK_AGENT_NAME, savedProfile, {
      profileId: PROFILE_ID_A,
      savedLaunchSettings,
      supportsDynamicModels: true,
    }),
  );

  expect(result.current.discoveryState).toBe("loading");
  await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
  expect(probeAgentProfileMock).toHaveBeenCalledTimes(1);
  expect(probeAgentProfileMock).toHaveBeenCalledWith(MOCK_AGENT_NAME, {
    profile_id: PROFILE_ID_A,
  });
  expect(result.current.models).toEqual([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }]);
  expect(resolveAgentModelConfigMock).not.toHaveBeenCalled();
});

it("does not call resolveAgentModelConfig under any circumstances (baseline-only)", async () => {
  probeAgentProfileMock.mockResolvedValueOnce(
    capabilityResponse([{ id: SAVED_MODEL_ID, name: SAVED_MODEL_NAME }], REVISION_A),
  );

  const { result } = renderHook(() =>
    useProfileCapabilityDiscovery(MOCK_AGENT_NAME, savedProfile, {
      profileId: PROFILE_ID_A,
      savedLaunchSettings,
      supportsDynamicModels: true,
    }),
  );

  await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
  expect(resolveAgentModelConfigMock).not.toHaveBeenCalled();
});

it("does not probe a draft profile until refresh is called", async () => {
  const draftProfile = {
    ...savedProfile,
    env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "draft-val" }],
  };

  const { result } = renderHook(() =>
    useProfileCapabilityDiscovery(MOCK_AGENT_NAME, draftProfile, {
      profileId: PROFILE_ID_A,
      savedLaunchSettings,
      supportsDynamicModels: true,
    }),
  );

  expect(probeAgentProfileMock).not.toHaveBeenCalled();
  expect(result.current.discoveryState).toBe("stale");

  probeAgentProfileMock.mockResolvedValueOnce(
    capabilityResponse([{ id: "draft-model", name: "Draft Model" }], "rev-draft"),
  );

  await act(async () => {
    await result.current.refresh();
  });

  expect(probeAgentProfileMock).toHaveBeenCalledTimes(1);
  expect(probeAgentProfileMock).toHaveBeenCalledWith(MOCK_AGENT_NAME, {
    profile_id: PROFILE_ID_A,
    launch_settings: {
      env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "draft-val", secret_id: undefined }],
      cli_flags: [{ flag: "--catalog", enabled: true, description: "catalog" }],
      command_prefix: "npx --",
    },
    refresh: true,
  } satisfies ProfileCapabilityRequest);
  expect(result.current.discoveryState).toBe("ready");
  expect(result.current.models).toEqual([{ id: "draft-model", name: "Draft Model" }]);
});
it("returns ready state immediately for static providers without probing", async () => {
  const { result } = renderHook(() =>
    useProfileCapabilityDiscovery(MOCK_AGENT_NAME, savedProfile, {
      profileId: PROFILE_ID_A,
      savedLaunchSettings,
      supportsDynamicModels: false,
    }),
  );

  expect(result.current.discoveryState).toBe("ready");
  expect(probeAgentProfileMock).not.toHaveBeenCalled();
});

it("handles probe failure and exposes error message", async () => {
  probeAgentProfileMock.mockRejectedValueOnce(new Error("Connection failed"));

  const { result } = renderHook(() =>
    useProfileCapabilityDiscovery(MOCK_AGENT_NAME, savedProfile, {
      profileId: PROFILE_ID_A,
      savedLaunchSettings,
      supportsDynamicModels: true,
    }),
  );

  await waitFor(() => expect(result.current.discoveryState).toBe("failed"));
  expect(result.current.error).toBe("Connection failed");
});

it("handles empty models response while maintaining ready state", async () => {
  probeAgentProfileMock.mockResolvedValueOnce(capabilityResponse([], REVISION_A));

  const { result } = renderHook(() =>
    useProfileCapabilityDiscovery(MOCK_AGENT_NAME, savedProfile, {
      profileId: PROFILE_ID_A,
      savedLaunchSettings,
      supportsDynamicModels: true,
    }),
  );

  await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
  expect(result.current.models).toEqual([]);
  expect(result.current.error).toBeNull();
});

it("waits for advertised dynamic support before automatically probing", async () => {
  probeAgentProfileMock.mockResolvedValue(
    capabilityResponse([{ id: "model-a", name: "Model A" }], "rev-a"),
  );
  const { result, rerender } = renderHook(
    ({ supportsDynamicModels }: { supportsDynamicModels: boolean | undefined }) =>
      useProfileCapabilityDiscovery(MOCK_AGENT_NAME, savedProfile, {
        profileId: PROFILE_ID_A,
        savedLaunchSettings,
        supportsDynamicModels,
      }),
    { initialProps: { supportsDynamicModels: undefined as boolean | undefined } },
  );
  expect(probeAgentProfileMock).not.toHaveBeenCalled();
  expect(result.current.discoveryState).toBe("ready");
  rerender({ supportsDynamicModels: true });
  await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
  expect(probeAgentProfileMock).toHaveBeenCalledTimes(1);
  expect(result.current.models).toEqual([{ id: "model-a", name: "Model A" }]);
});

it("clears prior state and re-probes on profileId change", async () => {
  probeAgentProfileMock.mockResolvedValueOnce(
    capabilityResponse([{ id: "model-a", name: "Model A" }], "rev-a"),
  );

  const { result, rerender } = renderHook(
    ({ profileId }) =>
      useProfileCapabilityDiscovery(MOCK_AGENT_NAME, savedProfile, {
        profileId,
        savedLaunchSettings,
        supportsDynamicModels: true,
      }),
    { initialProps: { profileId: PROFILE_ID_A } },
  );

  await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
  expect(result.current.models).toEqual([{ id: "model-a", name: "Model A" }]);

  probeAgentProfileMock.mockResolvedValueOnce(
    capabilityResponse([{ id: "model-b", name: "Model B" }], "rev-b"),
  );

  rerender({ profileId: PROFILE_ID_B });
  await waitFor(() => expect(result.current.discoveryState).toBe("ready"));
  expect(probeAgentProfileMock).toHaveBeenCalledTimes(2);
  expect(probeAgentProfileMock).toHaveBeenLastCalledWith(MOCK_AGENT_NAME, {
    profile_id: PROFILE_ID_B,
  });
  expect(result.current.models).toEqual([{ id: "model-b", name: "Model B" }]);
});

it("rejects late probe responses if launch settings changed in flight", async () => {
  let resolveOldProbe: ((response: DynamicModelsResponse) => void) | undefined;
  probeAgentProfileMock.mockImplementationOnce(
    () => new Promise<DynamicModelsResponse>((resolve) => (resolveOldProbe = resolve)),
  );

  const draft = {
    ...savedProfile,
    env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "val-1" }],
  };

  const { result, rerender } = renderHook(
    ({ profile }) =>
      useProfileCapabilityDiscovery(MOCK_AGENT_NAME, profile, {
        profileId: PROFILE_ID_A,
        savedLaunchSettings,
        supportsDynamicModels: true,
      }),
    { initialProps: { profile: draft } },
  );

  expect(result.current.discoveryState).toBe("stale");

  let refreshPromise: Promise<void> | undefined;
  act(() => {
    refreshPromise = result.current.refresh();
  });

  await waitFor(() => expect(probeAgentProfileMock).toHaveBeenCalledTimes(1));

  // Change launch settings while probe is in flight
  rerender({
    profile: {
      ...draft,
      env_vars: [{ key: "MOCK_AGENT_PROFILE_CATALOG", value: "val-2" }],
    },
  });

  expect(result.current.discoveryState).toBe("stale");

  await act(async () => {
    resolveOldProbe?.(capabilityResponse([{ id: "old-model", name: "Old model" }], "old"));
    await refreshPromise;
  });

  // Old response was rejected because launch settings changed
  expect(result.current.models).toEqual([]);
});
