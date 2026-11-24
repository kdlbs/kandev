import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { useProfileModelCapabilities } from "./use-profile-model-capabilities";

vi.mock("@/lib/api/domains/profile-capability-api", () => ({
  probeAgentProfile: vi.fn(async () => ({
    agent_name: "pi-acp",
    status: "ok",
    models: [
      { id: "reasoning", name: "Reasoning" },
      { id: "plain", name: "Plain" },
    ],
    modes: [],
    commands: [],
    context_revision: "pi-test",
    error: null,
  })),
}));
vi.mock("@/lib/api/domains/settings-api", () => ({
  resolveAgentModelConfig: vi.fn(async (_agent: string, request: { model: string }) => ({
    agent_name: "pi-acp",
    model: request.model,
    status: "ok",
    context_revision: "pi-test",
    error: null,
    config_options: [
      {
        type: "select",
        id: "thought_level",
        category: "thought_level",
        name: "Thinking level",
        current_value: "off",
        options: (request.model === "plain" ? ["off"] : ["off", "medium"]).map((value) => ({
          value,
          name: value,
        })),
      },
    ],
  })),
}));
afterEach(cleanup);

it("preserves saved Pi thinking intent when a new model cannot use it", async () => {
  const { result } = renderHook(() => {
    const [profile, setProfile] = useState({
      model: "reasoning",
      mode: "",
      config_options: { thought_level: "medium" } as Record<string, string>,
    });
    const capability = useProfileModelCapabilities(
      "pi-acp",
      profile,
      { default_model: "reasoning", available_models: [], supports_dynamic_models: true },
      (patch) => setProfile((current) => ({ ...current, ...patch })),
      {
        profileId: "pi-profile",
        savedLaunchSettings: { mode: "", env_vars: [], cli_flags: [], command_prefix: "" },
      },
    );
    return { profile, setProfile, capability };
  });
  await waitFor(() => expect(result.current.capability.configStatus).toBe("ok"));
  act(() => result.current.setProfile((profile) => ({ ...profile, model: "plain" })));
  await waitFor(() => expect(result.current.capability.configOptions[0]?.options).toHaveLength(1));
  expect(result.current.profile.config_options).toEqual({ thought_level: "medium" });
  expect(result.current.capability.isConfigResolutionPending).toBe(false);
});
