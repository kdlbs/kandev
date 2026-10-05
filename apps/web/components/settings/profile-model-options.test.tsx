import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ProfileFormFields, type ProfileFormData } from "./profile-form-fields";
import type {
  AgentModelConfigResponse,
  DynamicModelsResponse,
  ModelConfig,
  ResolveAgentModelConfigRequest,
} from "@/lib/types/http";

const MOCK_AGENT_NAME = "mock-agent";
const MODEL_A = "model-a";
const MODEL_B = "model-b";
const EFFORT_OPTION_ID = "reasoning_effort";
const EFFORT_OPTION_NAME = "Reasoning effort";
const PROFILE_START_MODEL_LABEL = "Profile start model settings";
const probeAgentProfileMock = vi.fn();
const resolveAgentModelConfigMock = vi.fn();

vi.mock("@/lib/api/domains/settings-api", () => ({
  resolveAgentModelConfig: (...args: unknown[]) => resolveAgentModelConfigMock(...args),
}));

vi.mock("@/lib/api/domains/profile-capability-api", () => ({
  probeAgentProfile: (...args: unknown[]) => probeAgentProfileMock(...args),
}));

afterEach(() => {
  cleanup();
  probeAgentProfileMock.mockReset();
  resolveAgentModelConfigMock.mockReset();
});

const dynamicModelConfig: ModelConfig = {
  default_model: MODEL_A,
  current_model_id: MODEL_A,
  available_models: [
    { id: MODEL_A, name: "Model A" },
    { id: MODEL_B, name: "Model B" },
  ],
  config_options: [
    {
      type: "select",
      id: EFFORT_OPTION_ID,
      name: EFFORT_OPTION_NAME,
      current_value: "medium",
      options: [{ value: "medium", name: "Medium" }],
    },
  ],
  supports_dynamic_models: true,
};

const baselineProfile: ProfileFormData = {
  name: "Profile",
  model: MODEL_A,
  mode: "",
  auto_approve: false,
  allow_indexing: false,
  cli_passthrough: false,
  cli_flags: [],
  env_vars: [],
  command_prefix: "",
  config_options: { [EFFORT_OPTION_ID]: "medium" },
};

function capabilityResponse(): DynamicModelsResponse {
  return {
    agent_name: MOCK_AGENT_NAME,
    status: "ok",
    models: [
      { id: MODEL_A, name: "Model A" },
      { id: MODEL_B, name: "Model B" },
    ],
    modes: [],
    commands: [],
    context_revision: "profile-model-options-test",
    error: null,
  };
}

function modelOptionsResponse(
  model: string,
  value: string,
  name: string,
): AgentModelConfigResponse {
  return {
    agent_name: MOCK_AGENT_NAME,
    model,
    status: "ok",
    config_options: [
      {
        type: "select",
        id: EFFORT_OPTION_ID,
        name: EFFORT_OPTION_NAME,
        current_value: value,
        options: [{ value, name }],
      },
    ],
    context_revision: "profile-model-options-test",
    error: null,
  };
}

function renderStatefulProfile() {
  function StatefulProfile() {
    const [profile, setProfile] = useState(baselineProfile);
    return (
      <ProfileFormFields
        profile={profile}
        baselineProfile={baselineProfile}
        onChange={(patch) => setProfile((current) => ({ ...current, ...patch }))}
        modelConfig={dynamicModelConfig}
        permissionSettings={{}}
        passthroughConfig={null}
        agentName={MOCK_AGENT_NAME}
        capabilityProfileId="profile-a"
      />
    );
  }

  render(
    <StateProvider>
      <TooltipProvider>
        <StatefulProfile />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("ProfileFormFields model options after selection", () => {
  it("keeps the model selector open while resolving selected model options", async () => {
    probeAgentProfileMock.mockResolvedValueOnce(capabilityResponse());
    let resolveResponse: ((value: AgentModelConfigResponse) => void) | undefined;
    const pendingResponse = new Promise<AgentModelConfigResponse>((resolve) => {
      resolveResponse = resolve;
    });
    resolveAgentModelConfigMock.mockImplementation(
      async (_agentName: string, request: ResolveAgentModelConfigRequest) =>
        request.model === MODEL_B
          ? pendingResponse
          : modelOptionsResponse(MODEL_A, "medium", "Medium"),
    );

    renderStatefulProfile();

    const selector = await screen.findByRole("button", { name: PROFILE_START_MODEL_LABEL });
    fireEvent.click(selector);
    fireEvent.click(screen.getByRole("option", { name: /Model B/ }));

    await waitFor(() => expect(screen.getByTestId("model-config-options-loading")).toBeTruthy());
    expect(screen.queryByTestId(`config-option-trigger-${EFFORT_OPTION_ID}`)).toBeNull();

    await act(async () => {
      resolveResponse?.(modelOptionsResponse(MODEL_B, "max", "Max"));
    });

    await waitFor(() =>
      expect(screen.getByTestId(`config-option-trigger-${EFFORT_OPTION_ID}`)).toBeTruthy(),
    );
    expect(screen.queryByTestId("model-config-options-loading")).toBeNull();
  });
});
