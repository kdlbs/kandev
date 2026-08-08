import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type {
  AgentModelConfigResponse,
  ConfigOptionEntry,
  DynamicModelsResponse,
  ModelConfig,
} from "@/lib/types/http";

const fetchDynamicModelsMock = vi.fn();
const resolveAgentModelConfigMock = vi.fn();

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchDynamicModels: (...args: unknown[]) => fetchDynamicModelsMock(...args),
  resolveAgentModelConfig: (...args: unknown[]) => resolveAgentModelConfigMock(...args),
}));

import {
  invalidateModelConfigResolutionCache,
  useAgentCapabilities,
  useResolvedModelConfig,
} from "./use-dynamic-models";

const initialConfig: ModelConfig = {
  default_model: "",
  available_models: [],
  available_modes: [],
  available_commands: [],
  supports_dynamic_models: true,
  status: "not_installed",
  error: "agent not installed",
};

function response(status: DynamicModelsResponse["status"]): DynamicModelsResponse {
  return {
    agent_name: "grok-acp",
    status,
    models: [],
    modes: [],
    commands: [],
    error: null,
  };
}

afterEach(() => {
  cleanup();
  invalidateModelConfigResolutionCache();
  fetchDynamicModelsMock.mockReset();
  resolveAgentModelConfigMock.mockReset();
});

function resolvedResponse(
  model: string,
  configOptions: ConfigOptionEntry[],
): AgentModelConfigResponse {
  return {
    agent_name: "opencode",
    model,
    status: "ok",
    config_options: configOptions,
    error: null,
  };
}

describe("useAgentCapabilities", () => {
  it("exposes the status returned by a forced capability refresh", async () => {
    fetchDynamicModelsMock
      .mockResolvedValueOnce(response("not_installed"))
      .mockResolvedValueOnce(response("ok"));

    const { result } = renderHook(() => useAgentCapabilities("grok-acp", initialConfig));
    await waitFor(() => expect(fetchDynamicModelsMock).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.status).toBe("ok");
    expect(fetchDynamicModelsMock).toHaveBeenLastCalledWith("grok-acp", { refresh: true });
  });

  it("updates status when the probe returns status detail text", async () => {
    fetchDynamicModelsMock.mockResolvedValueOnce({
      ...response("auth_required"),
      error: "login required",
    });

    const { result } = renderHook(() => useAgentCapabilities("grok-acp", initialConfig));

    await waitFor(() => expect(result.current.error).toBe("login required"));
    expect(result.current.status).toBe("auth_required");
  });

  it("keeps status in sync with a newer available-agent snapshot", async () => {
    fetchDynamicModelsMock.mockResolvedValue(response("not_installed"));

    const { result, rerender } = renderHook(
      ({ initial }) => useAgentCapabilities("grok-acp", initial),
      { initialProps: { initial: initialConfig } },
    );
    await waitFor(() => expect(fetchDynamicModelsMock).toHaveBeenCalledTimes(1));

    rerender({ initial: { ...initialConfig, status: "probing" } });

    expect(result.current.status).toBe("probing");
  });

  it("keeps loaded capabilities when a refresh probe fails", async () => {
    fetchDynamicModelsMock
      .mockResolvedValueOnce({
        ...response("ok"),
        models: [{ id: "grok-4", name: "Grok 4" }],
        current_model_id: "grok-4",
      })
      .mockResolvedValueOnce({
        agent_name: "grok-acp",
        status: "failed",
        models: [],
        error: "probe failed",
      });

    const { result } = renderHook(() => useAgentCapabilities("grok-acp", initialConfig));
    await waitFor(() => expect(result.current.models).toHaveLength(1));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.status).toBe("failed");
    expect(result.current.error).toBe("probe failed");
    expect(result.current.models).toEqual([{ id: "grok-4", name: "Grok 4" }]);
    expect(result.current.currentModelId).toBe("grok-4");
  });

  it("does not let a stale snapshot overwrite a completed manual refresh", async () => {
    fetchDynamicModelsMock
      .mockResolvedValueOnce(response("not_installed"))
      .mockResolvedValueOnce(response("ok"));

    const { result, rerender } = renderHook(
      ({ initial }) => useAgentCapabilities("grok-acp", initial),
      { initialProps: { initial: initialConfig } },
    );
    await waitFor(() => expect(fetchDynamicModelsMock).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.refresh();
    });
    rerender({ initial: { ...initialConfig, status: "probing" } });

    expect(result.current.status).toBe("ok");
  });

  // The catch branch falls back to the catalog string. Nothing else in this
  // file makes `fetchDynamicModels` reject, so without this the branch can
  // drift undetected: rename the key and `t()` silently returns the key name.
  // The mount effect issues the fetch, so rejecting once is enough.
  it("falls back to the catalog string when the capability fetch rejects with a non-Error", async () => {
    fetchDynamicModelsMock.mockRejectedValueOnce("raw rejection");

    const { result } = renderHook(() => useAgentCapabilities("grok-acp", initialConfig));

    await waitFor(() => expect(result.current.error).toBe("Failed to fetch capabilities"));
    expect(result.current.isLoading).toBe(false);
  });

  it("surfaces the thrown message when the capability fetch rejects with an Error", async () => {
    fetchDynamicModelsMock.mockRejectedValueOnce(new Error("probe timed out"));

    const { result } = renderHook(() => useAgentCapabilities("grok-acp", initialConfig));

    await waitFor(() => expect(result.current.error).toBe("probe timed out"));
  });

  it("polls a pending bootstrap until the capability snapshot is ready", async () => {
    fetchDynamicModelsMock
      .mockResolvedValueOnce(response("not_configured"))
      .mockResolvedValueOnce(response("probing"))
      .mockResolvedValueOnce({
        ...response("ok"),
        modes: [{ id: "plan", name: "Plan" }],
      });

    const { result } = renderHook(() => useAgentCapabilities("grok-acp", initialConfig));

    await waitFor(
      () => {
        expect(result.current.modes).toEqual([{ id: "plan", name: "Plan" }]);
        expect(result.current.isLoading).toBe(false);
      },
      { timeout: 2_000 },
    );
    expect(fetchDynamicModelsMock).toHaveBeenCalledTimes(3);
  });
});

describe("useResolvedModelConfig", () => {
  it("loads options for the selected model and ignores an older response", async () => {
    let resolveFirst: ((response: AgentModelConfigResponse) => void) | undefined;
    let resolveSecond: ((response: AgentModelConfigResponse) => void) | undefined;
    resolveAgentModelConfigMock.mockImplementation((_, request: { model: string }) => {
      if (request.model === "model-a") {
        return new Promise<AgentModelConfigResponse>((resolve) => {
          resolveFirst = resolve;
        });
      }
      return new Promise<AgentModelConfigResponse>((resolve) => {
        resolveSecond = resolve;
      });
    });

    const baseline: ConfigOptionEntry[] = [
      { type: "select", id: "effort", name: "Effort", current_value: "low", options: [] },
    ];
    const { result, rerender } = renderHook(
      ({ model }) =>
        useResolvedModelConfig("opencode", model, {
          mode: "build",
          configOptions: { effort: "low" },
          initialConfigOptions: baseline,
        }),
      { initialProps: { model: "model-a" } },
    );

    await waitFor(() => expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1));
    expect(resolveAgentModelConfigMock).toHaveBeenLastCalledWith("opencode", {
      model: "model-a",
      mode: "build",
      config_options: { effort: "low" },
    });

    rerender({ model: "model-b" });
    await waitFor(() => expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(2));

    const secondOptions: ConfigOptionEntry[] = [
      {
        type: "select",
        id: "reasoning_effort",
        name: "Reasoning effort",
        current_value: "max",
        options: [],
      },
    ];
    await act(async () => {
      resolveSecond?.(resolvedResponse("model-b", secondOptions));
    });
    expect(result.current.configOptions).toEqual(secondOptions);

    await act(async () => {
      resolveFirst?.(resolvedResponse("model-a", baseline));
    });
    expect(result.current.configOptions).toEqual(secondOptions);
  });
});

describe("useResolvedModelConfig draft updates", () => {
  it("does not re-resolve for an equivalent draft option object", async () => {
    const configOptions: ConfigOptionEntry[] = [
      {
        type: "select",
        id: "effort",
        name: "Effort",
        current_value: "low",
        options: [
          { value: "low", name: "Low" },
          { value: "high", name: "High" },
        ],
      },
    ];
    fetchDynamicModelsMock.mockResolvedValue(response("ok"));
    resolveAgentModelConfigMock.mockResolvedValue(
      resolvedResponse("profile-draft-model", configOptions),
    );

    const { rerender } = renderHook(
      ({ draft }) =>
        useResolvedModelConfig("profile-draft-agent", "profile-draft-model", {
          configOptions: draft,
          initialConfigOptions: configOptions,
        }),
      { initialProps: { draft: { effort: "low" } } },
    );

    await waitFor(() => expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1));

    rerender({ draft: { effort: "low" } });
    await act(async () => Promise.resolve());

    expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1);
    expect(resolveAgentModelConfigMock).toHaveBeenCalledWith("profile-draft-agent", {
      model: "profile-draft-model",
      config_options: { effort: "low" },
    });
  });
});

describe("useResolvedModelConfig cache invalidation", () => {
  const cacheRefreshModel = "cache-refresh-model";

  it("invalidates model option resolutions when capabilities refresh", async () => {
    const firstOptions: ConfigOptionEntry[] = [
      { type: "select", id: "effort", name: "Effort", current_value: "low", options: [] },
    ];
    const secondOptions: ConfigOptionEntry[] = [
      { type: "select", id: "effort", name: "Effort", current_value: "high", options: [] },
    ];
    fetchDynamicModelsMock.mockResolvedValue(response("ok"));
    resolveAgentModelConfigMock
      .mockResolvedValueOnce(resolvedResponse(cacheRefreshModel, firstOptions))
      .mockResolvedValueOnce(resolvedResponse(cacheRefreshModel, secondOptions));

    const modelConfig: ModelConfig = {
      ...initialConfig,
      current_model_id: cacheRefreshModel,
      default_model: cacheRefreshModel,
    };
    const cacheRefreshAgent = "cache-refresh-agent";
    const firstResolution = renderHook(() =>
      useResolvedModelConfig(cacheRefreshAgent, cacheRefreshModel, {
        initialConfigOptions: firstOptions,
      }),
    );
    await waitFor(() => expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1));

    const capabilities = renderHook(() => useAgentCapabilities(cacheRefreshAgent, modelConfig));
    await waitFor(() => expect(fetchDynamicModelsMock).toHaveBeenCalledTimes(1));
    await act(async () => {
      await capabilities.result.current.refresh();
    });

    const secondResolution = renderHook(() =>
      useResolvedModelConfig(cacheRefreshAgent, cacheRefreshModel, {
        initialConfigOptions: firstOptions,
      }),
    );
    await waitFor(() =>
      expect(secondResolution.result.current.configOptions).toEqual(secondOptions),
    );
    expect(firstResolution.result.current.configOptions).toEqual(firstOptions);
  });
});
