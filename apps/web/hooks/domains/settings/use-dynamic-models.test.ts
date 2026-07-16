import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
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

afterEach(() => {
  cleanup();
  fetchDynamicModelsMock.mockReset();
});

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
});
