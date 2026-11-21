import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
import { ProfileFormFields, type ProfileFormData } from "./profile-form-fields";
import { resolveAgentModelConfig } from "@/lib/api/domains/settings-api";
import type { ModelConfig } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({ onChange: vi.fn() }));

const AGENT = "devin-acp";
const STATUS_OK = "ok";
const PAIR_A = "pair-a";
const PAIR_B = "pair-b";
const BALANCED_A = "Balanced A";
const BALANCED_B = "Balanced B";

vi.mock("@/lib/api/domains/settings-api", () => ({
  probeAgentProfile: vi.fn(async () => ({
    agent_name: AGENT,
    status: STATUS_OK,
    models: [
      { id: PAIR_A, name: BALANCED_A },
      { id: PAIR_B, name: BALANCED_B },
    ],
    modes: [
      { id: "smart", name: "Smart" },
      { id: "autonomous", name: "Autonomous" },
    ],
    commands: [],
    context_revision: "profile-composed",
    error: null,
  })),
  fetchDynamicModels: vi.fn(async () => ({
    agent_name: AGENT,
    status: STATUS_OK,
    models: [],
    modes: [],
    commands: [],
    error: null,
  })),
  resolveAgentModelConfig: vi.fn(async (_agentName: string, request: { model: string }) => ({
    agent_name: AGENT,
    model: request.model,
    status: STATUS_OK,
    config_options: [modelConfigOption(request.model), thinkingOption()],
    context_revision: "profile-composed",
    error: null,
  })),
}));

vi.mock("@/lib/api/domains/profile-capability-api", () => ({
  probeAgentProfile: vi.fn(async () => ({
    agent_name: AGENT,
    status: STATUS_OK,
    models: [
      { id: PAIR_A, name: BALANCED_A },
      { id: PAIR_B, name: BALANCED_B },
    ],
    modes: [
      { id: "smart", name: "Smart" },
      { id: "autonomous", name: "Autonomous" },
    ],
    commands: [],
    context_revision: "profile-composed",
    error: null,
  })),
}));

function modelConfigOption(currentValue = PAIR_A) {
  return {
    type: "select",
    id: "model",
    name: "Model",
    current_value: currentValue,
    category: "model",
    options: [
      { value: PAIR_A, name: BALANCED_A },
      { value: PAIR_B, name: BALANCED_B },
    ],
    catalog: {
      version: 1,
      nodes: [
        {
          kind: "group",
          id: "composed",
          label: "Composed",
          children: [
            { kind: "choice", value: PAIR_A, label: BALANCED_A },
            { kind: "choice", value: PAIR_B, label: BALANCED_B },
          ],
        },
      ],
      compositions: [
        {
          id: "composed",
          group_id: "composed",
          label: "Composed",
          dimensions: [
            {
              id: "pairing",
              label: "Pairing",
              choices: [
                { value: "a", label: "Pairing A" },
                { value: "b", label: "Pairing B" },
              ],
            },
          ],
          bindings: [
            { values: { pairing: "a" }, model: PAIR_A },
            { values: { pairing: "b" }, model: PAIR_B },
          ],
        },
      ],
    },
  };
}

function thinkingOption() {
  return {
    type: "select",
    id: "thinking",
    name: "Thinking",
    current_value: "low",
    options: [
      { value: "low", name: "Low" },
      { value: "high", name: "High" },
    ],
  };
}

const modelConfig: ModelConfig = {
  default_model: PAIR_A,
  available_models: [
    { id: PAIR_A, name: BALANCED_A },
    { id: PAIR_B, name: BALANCED_B },
  ],
  supports_dynamic_models: true,
  config_options: [modelConfigOption(), thinkingOption()],
};

function profile(): ProfileFormData {
  return {
    name: "Devin profile",
    model: PAIR_A,
    mode: "smart",
    config_options: { thinking: "low" },
    cli_passthrough: false,
    cli_flags: [],
    command_prefix: "",
    env_vars: [{ key: "DEVIN_SANDBOX", value: "false" }],
    auto_approve: false,
    allow_indexing: true,
  } as ProfileFormData;
}

afterEach(() => {
  cleanup();
  mocks.onChange.mockClear();
});

describe("profile composed configuration", () => {
  it("applies the exact model and provider options only after the explicit action", async () => {
    const current = profile();
    render(
      <StateProvider>
        <TooltipProvider>
          <ProfileFormFields
            profile={current}
            baselineProfile={current}
            onChange={mocks.onChange}
            modelConfig={modelConfig}
            permissionSettings={{}}
            passthroughConfig={null}
            agentName="devin-acp"
            capabilityProfileId="profile-1"
          />
        </TooltipProvider>
      </StateProvider>,
    );

    await waitFor(() =>
      expect(screen.getByTestId("profile-capability-status").getAttribute("data-status")).toBe(
        "ready",
      ),
    );
    const trigger = screen.getByRole("button", { name: "Profile start model settings" });
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByTestId("model-catalog-composition-row-composed"));
    expect(mocks.onChange).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("model-composition-dimension-pairing"));
    fireEvent.click(screen.getByTestId("model-composition-choice-pairing-b"));
    await waitFor(() => expect(screen.getByTestId("model-provider-option-thinking")).toBeTruthy());
    expect(mocks.onChange).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("model-provider-option-select-thinking"));
    fireEvent.click(await screen.findByRole("option", { name: "High" }));
    fireEvent.click(screen.getByTestId("model-composition-apply"));

    expect(mocks.onChange).toHaveBeenCalledWith({
      model: PAIR_B,
      config_options: { thinking: "high" },
    });
    expect(vi.mocked(resolveAgentModelConfig)).toHaveBeenCalledWith(
      AGENT,
      expect.objectContaining({ model: PAIR_B }),
    );
  });
});
