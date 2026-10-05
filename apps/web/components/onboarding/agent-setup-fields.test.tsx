import { useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { AgentSetupFields, type AgentSetupFieldsProps } from "./agent-setup-fields";
import { StepAgents } from "./step-agents";
import type { AgentSetting } from "./agent-settings";
import type { AvailableAgent, DynamicModelsResponse } from "@/lib/types/http";

const TEST_AGENT_NAME = "test-agent";
const MODEL_SELECTOR_LABEL = "Profile start model settings";

function renderFields(props: AgentSetupFieldsProps) {
  return render(
    <TooltipProvider>
      <AgentSetupFields {...props} />
    </TooltipProvider>,
  );
}

const probeAgentProfileMock = vi.fn();
const resolveAgentModelConfigMock = vi.fn();

vi.mock("@/lib/api/domains/profile-capability-api", () => ({
  probeAgentProfile: (...args: unknown[]) => probeAgentProfileMock(...args),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  resolveAgentModelConfig: (...args: unknown[]) => resolveAgentModelConfigMock(...args),
}));

const mockAgent: AvailableAgent = {
  name: TEST_AGENT_NAME,
  display_name: "Test Agent",
  available: true,
  supports_mcp: false,
  installation_paths: [],
  capabilities: {
    supports_session_resume: false,
    supports_shell: false,
    supports_workspace_only: false,
  },
  updated_at: "2026-10-05T00:00:00Z",
  model_config: {
    default_model: "gpt-4",
    current_mode_id: "default",
    status: "ok",
    supports_dynamic_models: true,
    available_models: [
      { id: "gpt-4", name: "GPT-4" },
      { id: "gpt-3.5", name: "GPT-3.5" },
    ],
  },
  passthrough_config: {
    supported: true,
    label: "CLI Passthrough",
    description: "Use terminal interface directly",
  },
  permission_settings: {},
};

const mockSetting: AgentSetting = {
  profileId: "profile-1",
  draft: {
    model: "gpt-4",
    cli_passthrough: false,
  },
  baseline: {
    model: "gpt-4",
    cli_passthrough: false,
  },
  savedLaunchSettings: {
    env_vars: [],
    cli_flags: [],
    command_prefix: "",
  },
  dirty: false,
};

function capabilityResponse(models: { id: string; name: string }[]): DynamicModelsResponse {
  return {
    agent_name: TEST_AGENT_NAME,
    status: "ok",
    models,
    modes: [{ id: "default", name: "Default" }],
    commands: [],
    current_model_id: models[0]?.id,
    current_mode_id: "default",
    context_revision: "rev-1",
    error: null,
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  probeAgentProfileMock.mockResolvedValue(capabilityResponse([]));
  resolveAgentModelConfigMock.mockResolvedValue({
    agent_name: TEST_AGENT_NAME,
    model: "gpt-4",
    status: "ok",
    config_options: [],
    error: null,
    context_revision: "rev-1",
  });
});

afterEach(cleanup);

describe("AgentSetupFields", () => {
  it("keeps an empty profile catalog empty instead of offering global models", async () => {
    renderFields({ agent: mockAgent, setting: mockSetting, onChange: vi.fn() });
    await screen.findByText("No models found.");
    const selector = screen.getByRole("button", { name: MODEL_SELECTOR_LABEL });
    expect(selector.textContent).toContain("gpt-4");
    fireEvent.click(selector);
    expect(screen.queryByRole("option", { name: "GPT-3.5" })).toBeNull();
  });

  it("uses the advertised static catalog without a profile probe", () => {
    const agent = {
      ...mockAgent,
      model_config: { ...mockAgent.model_config, supports_dynamic_models: false },
    };
    renderFields({ agent, setting: mockSetting, onChange: vi.fn() });
    expect(screen.queryByRole("button", { name: "Refresh models" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: MODEL_SELECTOR_LABEL }));
    expect(screen.getByRole("option", { name: "GPT-3.5" })).toBeTruthy();
    expect(probeAgentProfileMock).not.toHaveBeenCalled();
  });

  it("renders only required controls (model selector, refresh icon, passthrough) and omits advanced/permission controls", async () => {
    probeAgentProfileMock.mockResolvedValueOnce(
      capabilityResponse([
        { id: "gpt-4", name: "GPT-4" },
        { id: "gpt-3.5", name: "GPT-3.5" },
      ]),
    );

    renderFields({
      agent: mockAgent,
      setting: mockSetting,
      onChange: vi.fn(),
    });

    expect(screen.getByTestId("onboarding-agent-model-field")).toBeTruthy();
    expect(screen.getByTestId("onboarding-agent-refresh-models")).toBeTruthy();
    expect(screen.getByTestId("onboarding-agent-passthrough-field")).toBeTruthy();

    expect(screen.queryByText(/Auto-approve/i)).toBeNull();
    expect(screen.queryByText(/Advanced settings/i)).toBeNull();
    expect(screen.queryByText(/Fallback settings/i)).toBeNull();
    expect(screen.queryByText(/CLI Flags/i)).toBeNull();
    expect(screen.queryByText(/Command prefix/i)).toBeNull();
  });

  it("omits passthrough toggle when not supported", async () => {
    const unsupportedAgent: AvailableAgent = {
      ...mockAgent,
      passthrough_config: { supported: false, label: "", description: "" },
    };

    renderFields({
      agent: unsupportedAgent,
      setting: mockSetting,
      onChange: vi.fn(),
    });

    expect(screen.queryByTestId("onboarding-agent-passthrough-field")).toBeNull();
  });

  it("calls onChange when passthrough toggle is clicked", async () => {
    const onChange = vi.fn();
    renderFields({
      agent: mockAgent,
      setting: mockSetting,
      onChange,
    });

    const switchEl = screen.getByRole("switch", { name: "CLI Passthrough" });
    fireEvent.click(switchEl);

    expect(onChange).toHaveBeenCalledWith({ cli_passthrough: true });
  });
  it("handles loading, error, empty, and gone model states properly", async () => {
    let resolveProbe!: (value: DynamicModelsResponse) => void;
    probeAgentProfileMock.mockImplementationOnce(
      () => new Promise((resolve) => (resolveProbe = resolve)),
    );

    const onStatusChange = vi.fn();
    renderFields({
      agent: mockAgent,
      setting: mockSetting,
      onChange: vi.fn(),
      onStatusChange,
    });

    // Loading state
    expect(screen.getByRole("status")).toBeTruthy();
    expect(onStatusChange).toHaveBeenCalledWith("probing", null);

    // Resolve with models
    resolveProbe(capabilityResponse([{ id: "gpt-4", name: "GPT-4" }]));
    await waitFor(() => {
      expect(screen.queryByRole("status")).toBeNull();
    });
    expect(onStatusChange).toHaveBeenCalledWith("ok", null);
  });

  it("calls refresh when refresh button is clicked", async () => {
    probeAgentProfileMock.mockResolvedValue(capabilityResponse([{ id: "gpt-4", name: "GPT-4" }]));

    renderFields({
      agent: mockAgent,
      setting: mockSetting,
      onChange: vi.fn(),
    });

    await waitFor(() => expect(probeAgentProfileMock).toHaveBeenCalledTimes(1));

    const refreshButton = screen.getByTestId("onboarding-agent-refresh-models");
    fireEvent.click(refreshButton);

    await waitFor(() => expect(probeAgentProfileMock).toHaveBeenCalledTimes(2));
  });
});

it("announces authentication failure and links to the saved profile settings", async () => {
  probeAgentProfileMock.mockResolvedValue({
    ...capabilityResponse([]),
    status: "auth_required",
  });
  renderFields({ agent: mockAgent, setting: mockSetting, onChange: vi.fn() });
  expect((await screen.findByRole("alert")).textContent).toContain("Authentication required");
  expect(screen.getByRole("link", { name: "Settings" }).getAttribute("href")).toBe(
    "/settings/agents/test-agent/profiles/profile-1",
  );
});

it("forwards the profile error with its unsupported status and keeps the collapsed row unhealthy", async () => {
  const error = "Saved profile discovery is unsupported";
  probeAgentProfileMock.mockResolvedValue({
    ...capabilityResponse([]),
    status: "unsupported",
    error,
  });
  const onStatusChange = vi.fn();
  const fields = renderFields({
    agent: mockAgent,
    setting: mockSetting,
    onChange: vi.fn(),
    onStatusChange,
  });
  await waitFor(() => expect(onStatusChange).toHaveBeenCalledWith("unsupported", error));
  expect(screen.getByRole("alert").textContent).toBe(error);
  fields.unmount();
  render(
    <TooltipProvider>
      <StepAgents
        availableAgents={[mockAgent]}
        tools={[]}
        agentSettings={{ [TEST_AGENT_NAME]: mockSetting }}
        loading={false}
        onUpdateSetting={vi.fn()}
      />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: /Test Agent/ });
  fireEvent.click(trigger);
  await screen.findByText("Error");
  fireEvent.click(trigger);
  expect(trigger.textContent).toContain("Error");
  expect(trigger.textContent).not.toContain("Installed");
});

it("forwards a localized fallback when a failed profile probe has no error text", async () => {
  probeAgentProfileMock.mockResolvedValue({ ...capabilityResponse([]), status: "failed" });
  const onStatusChange = vi.fn();
  renderFields({ agent: mockAgent, setting: mockSetting, onChange: vi.fn(), onStatusChange });
  await screen.findByRole("alert");
  await waitFor(() => expect(onStatusChange).toHaveBeenCalledWith("failed", expect.any(String)));
  expect(onStatusChange.mock.calls.at(-1)?.[1]).toBe(screen.getByRole("alert").textContent);
});

// @covers AC-AGENTS-FIRST-RUN-SETUP-002.4
it("uses the shared selector for profile-context reasoning options without editing them on open", async () => {
  probeAgentProfileMock.mockResolvedValue(capabilityResponse([{ id: "gpt-4", name: "GPT-4" }]));
  resolveAgentModelConfigMock.mockResolvedValue({
    agent_name: TEST_AGENT_NAME,
    model: "gpt-4",
    status: "ok",
    error: null,
    context_revision: "rev-1",
    config_options: [
      {
        id: "reasoning",
        name: "Reasoning",
        type: "select",
        category: "thought_level",
        current_value: "low",
        options: [
          { value: "low", name: "Low" },
          { value: "high", name: "High" },
        ],
      },
    ],
  });
  const onChange = vi.fn();
  function Form() {
    const [setting, setSetting] = useState(mockSetting);
    return (
      <AgentSetupFields
        agent={mockAgent}
        setting={setting}
        onChange={(patch) => {
          onChange(patch);
          setSetting((current) => ({ ...current, draft: { ...current.draft, ...patch } }));
        }}
      />
    );
  }
  render(
    <TooltipProvider>
      <Form />
    </TooltipProvider>,
  );
  await waitFor(() =>
    expect(resolveAgentModelConfigMock).toHaveBeenCalledWith(
      TEST_AGENT_NAME,
      expect.objectContaining({ profile_id: "profile-1", model: "gpt-4" }),
    ),
  );
  expect(onChange).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: MODEL_SELECTOR_LABEL }));
  fireEvent.click(await screen.findByRole("button", { name: /Reasoning/ }));
  fireEvent.click(await screen.findByRole("button", { name: /^High$/ }));
  expect(onChange).toHaveBeenCalledWith({ config_options: { reasoning: "high" } });
});

it("keeps discovery and options when a provider is collapsed and reopened", async () => {
  probeAgentProfileMock.mockResolvedValue(capabilityResponse([{ id: "gpt-4", name: "GPT-4" }]));
  render(
    <TooltipProvider>
      <StepAgents
        availableAgents={[mockAgent]}
        tools={[]}
        agentSettings={{ [TEST_AGENT_NAME]: mockSetting }}
        loading={false}
        onUpdateSetting={vi.fn()}
      />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: /Test Agent/ });
  fireEvent.click(trigger);
  await waitFor(() => expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1));
  fireEvent.click(trigger);
  fireEvent.click(trigger);
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: MODEL_SELECTOR_LABEL }) as HTMLButtonElement).disabled,
    ).toBe(false),
  );
  expect(probeAgentProfileMock).toHaveBeenCalledTimes(1);
  expect(resolveAgentModelConfigMock).toHaveBeenCalledTimes(1);
});
