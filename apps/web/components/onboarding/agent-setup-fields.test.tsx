import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { AgentSetupFields, type AgentSetupFieldsProps } from "./agent-setup-fields";
import type { AgentSetting } from "./agent-settings";
import type { AvailableAgent, DynamicModelsResponse } from "@/lib/types/http";

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
  name: "test-agent",
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
    agent_name: "test-agent",
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
});

afterEach(cleanup);

describe("AgentSetupFields", () => {
  it("keeps an empty profile catalog empty instead of offering global models", async () => {
    renderFields({ agent: mockAgent, setting: mockSetting, onChange: vi.fn() });
    await screen.findByText("No models found.");
    const selector = screen.getByRole("button", { name: "Profile start model settings" });
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
    fireEvent.click(screen.getByRole("button", { name: "Profile start model settings" }));
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

    const switchEl = screen.getByTestId("onboarding-agent-passthrough-switch");
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
    expect(onStatusChange).toHaveBeenCalledWith("probing");

    // Resolve with models
    resolveProbe(capabilityResponse([{ id: "gpt-4", name: "GPT-4" }]));
    await waitFor(() => {
      expect(screen.queryByRole("status")).toBeNull();
    });
    expect(onStatusChange).toHaveBeenCalledWith("ok");
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
