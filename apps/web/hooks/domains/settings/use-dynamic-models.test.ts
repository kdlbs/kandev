import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DynamicModelsResponse, ModelConfig } from "@/lib/types/http";

const fetchDynamicModelsMock = vi.fn();

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchDynamicModels: (...args: unknown[]) => fetchDynamicModelsMock(...args),
}));

import { useAgentCapabilities } from "./use-dynamic-models";

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

function queryWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
}

afterEach(() => {
  cleanup();
  fetchDynamicModelsMock.mockReset();
});

describe("useAgentCapabilities", () => {
  it("exposes the status returned by a forced capability refresh", async () => {
    fetchDynamicModelsMock
      .mockResolvedValueOnce(response("not_installed"))
      .mockResolvedValueOnce(response("ok"));

    const { result } = renderHook(() => useAgentCapabilities("grok-acp", initialConfig), {
      wrapper: queryWrapper(),
    });
    await waitFor(() => expect(fetchDynamicModelsMock).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.status).toBe("ok");
    expect(fetchDynamicModelsMock).toHaveBeenLastCalledWith(
      "grok-acp",
      expect.objectContaining({ refresh: true }),
    );
  });

  it("updates status when the probe returns status detail text", async () => {
    fetchDynamicModelsMock.mockResolvedValueOnce({
      ...response("auth_required"),
      error: "login required",
    });

    const { result } = renderHook(() => useAgentCapabilities("grok-acp", initialConfig), {
      wrapper: queryWrapper(),
    });

    await waitFor(() => expect(result.current.error).toBe("login required"));
    expect(result.current.status).toBe("auth_required");
  });

  it("keeps status in sync with a newer available-agent snapshot", async () => {
    fetchDynamicModelsMock.mockResolvedValue(response("not_installed"));

    const { result, rerender } = renderHook(
      ({ initial }) => useAgentCapabilities("grok-acp", initial),
      { initialProps: { initial: initialConfig }, wrapper: queryWrapper() },
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

    const { result } = renderHook(() => useAgentCapabilities("grok-acp", initialConfig), {
      wrapper: queryWrapper(),
    });
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
      { initialProps: { initial: initialConfig }, wrapper: queryWrapper() },
    );
    await waitFor(() => expect(fetchDynamicModelsMock).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.refresh();
    });
    rerender({ initial: { ...initialConfig, status: "probing" } });

    expect(result.current.status).toBe("ok");
  });
});
