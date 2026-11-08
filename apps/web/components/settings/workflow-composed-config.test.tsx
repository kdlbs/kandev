import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SessionConfigRuleCard } from "./workflow-session-config-rule-card";
import type { AvailableAgent, ConfigOptionEntry } from "@/lib/types/http";

const AGENT = "devin-acp";
const PAIR_A = "pair-a";
const PAIR_B = "pair-b";
import type { ConfigureSessionRule } from "@/lib/types/workflow-actions";

vi.mock("@/lib/api/domains/settings-api", () => ({
  resolveAgentModelConfig: vi.fn(async (_agentName: string, request: { model: string }) => ({
    agent_name: AGENT,
    model: request.model,
    status: "ok",
    config_options: [
      {
        type: "select",
        id: "thinking",
        name: "Thinking",
        current_value: "low",
        options: [
          { value: "low", name: "Low" },
          { value: "high", name: "High" },
        ],
      },
    ],
    error: null,
  })),
}));

function modelConfigOption(): ConfigOptionEntry {
  return {
    type: "select",
    id: "model",
    name: "Model",
    current_value: PAIR_A,
    category: "model",
    options: [
      { value: PAIR_A, name: "Balanced A" },
      { value: PAIR_B, name: "Balanced B" },
    ],
    catalog: {
      version: 1,
      nodes: [
        {
          kind: "group",
          id: "composed",
          label: "Composed",
          children: [
            { kind: "choice", value: PAIR_A, label: "Balanced A" },
            { kind: "choice", value: PAIR_B, label: "Balanced B" },
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

const availableAgents: AvailableAgent[] = [
  {
    name: AGENT,
    display_name: "Devin",
    supports_mcp: true,
    installation_paths: [],
    available: true,
    capabilities: {
      supports_session_resume: true,
      supports_shell: true,
      supports_workspace_only: false,
    },
    model_config: {
      default_model: PAIR_A,
      available_models: [
        { id: PAIR_A, name: "Balanced A" },
        { id: PAIR_B, name: "Balanced B" },
      ],
      supports_dynamic_models: false,
      config_options: [modelConfigOption()],
    },
    updated_at: "2026-11-03T00:00:00Z",
  },
];

const rule: ConfigureSessionRule = {
  agent_name: AGENT,
  operation: "set",
  model: PAIR_A,
};

afterEach(() => {
  cleanup();
});

describe("workflow composed configuration", () => {
  it("applies the exact model and provider options to the rule draft", async () => {
    const onChange = vi.fn();
    render(
      <TooltipProvider>
        <SessionConfigRuleCard
          rule={rule}
          index={0}
          choices={[{ name: AGENT, label: "Devin" }]}
          availableAgents={availableAgents}
          readOnly={false}
          onChange={onChange}
          onRemove={() => {}}
          onResolutionPendingChange={() => {}}
        />
      </TooltipProvider>,
    );

    const trigger = screen.getByRole("button", { name: "Settings for devin-acp" });
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByTestId("model-catalog-composition-row-composed"));
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("model-composition-dimension-pairing"));
    fireEvent.click(screen.getByTestId("model-composition-choice-pairing-b"));
    await waitFor(() => expect(screen.getByTestId("model-provider-option-thinking")).toBeTruthy());
    fireEvent.click(screen.getByTestId("model-provider-option-select-thinking"));
    fireEvent.click(await screen.findByRole("option", { name: "High" }));
    fireEvent.click(screen.getByTestId("model-composition-apply"));

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        operation: "set",
        agent_name: AGENT,
        model: PAIR_B,
        config_options: { thinking: "high" },
      }),
    );
  });
});
