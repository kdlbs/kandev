import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { SettingsSaveProvider, useSettingsSaveContributor } from "./settings-save-provider";
import { fetchDynamicModels, resolveAgentModelConfig } from "@/lib/api/domains/settings-api";
import { __resetModelConfigResolutionCache } from "@/hooks/domains/settings/use-dynamic-models";
import { ProfileFormFields, type ProfileFormData } from "./profile-form-fields";
import type { ModelConfig } from "@/lib/types/http";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchDynamicModels: vi.fn(async () => ({
    agent_name: "opencode",
    status: "ok",
    models: [
      { id: "model-a", name: "Model A" },
      { id: "model-b", name: "Model B" },
    ],
    modes: [],
    commands: [],
    error: null,
  })),
  resolveAgentModelConfig: vi.fn(async (_agentName: string, request: { model: string }) => ({
    agent_name: "opencode",
    model: request.model,
    status: "ok",
    config_options: [
      {
        type: "select",
        id: reasoningEffortOptionId,
        name: reasoningEffortName,
        current_value: "max",
        options: [{ value: "max", name: "Max" }],
      },
    ],
    error: null,
  })),
}));

afterEach(cleanup);

const modelConfig: ModelConfig = {
  default_model: "mock-fast",
  available_models: [{ id: "mock-fast", name: "Mock Fast" }],
  supports_dynamic_models: false,
};

const reasoningEffortOptionId = "reasoning_effort";
const reasoningEffortName = "Reasoning effort";
const profileStartModelSettingsLabel = "Profile start model settings";

function formData(overrides: Partial<ProfileFormData> = {}): ProfileFormData {
  return {
    name: "Profile",
    model: "mock-fast",
    mode: "",
    cli_passthrough: false,
    cli_flags: [],
    command_prefix: "",
    ...overrides,
  } as ProfileFormData;
}

function renderForm(
  profile: ProfileFormData,
  config: ModelConfig = modelConfig,
  onChange: (patch: Partial<ProfileFormData>) => void = vi.fn(),
  cursorMcpAuthSupported = false,
) {
  return render(
    <TooltipProvider>
      <ProfileFormFields
        profile={profile}
        onChange={onChange}
        modelConfig={config}
        permissionSettings={{}}
        passthroughConfig={null}
        agentName="mock-agent"
        cursorMcpAuthSupported={cursorMcpAuthSupported}
      />
    </TooltipProvider>,
  );
}

function renderStatefulForm(
  profile: ProfileFormData,
  config: ModelConfig,
  onChange: (patch: Partial<ProfileFormData>) => void,
  cursorMcpAuthSupported = false,
) {
  function StatefulForm() {
    const [currentProfile, setCurrentProfile] = useState(profile);
    return (
      <ProfileFormFields
        profile={currentProfile}
        onChange={(patch) => {
          onChange(patch);
          setCurrentProfile((current) => ({ ...current, ...patch }));
        }}
        modelConfig={config}
        permissionSettings={{}}
        passthroughConfig={null}
        agentName="mock-agent"
        cursorMcpAuthSupported={cursorMcpAuthSupported}
      />
    );
  }

  return render(
    <TooltipProvider>
      <StatefulForm />
    </TooltipProvider>,
  );
}

function expandFallbackSettings() {
  fireEvent.click(screen.getByTestId("profile-fallback-settings-trigger"));
}

describe("ProfileFormFields command prefix visibility", () => {
  it("keeps the command prefix behind collapsed advanced options", () => {
    renderForm(formData({ cli_passthrough: false }));

    expect(screen.queryByTestId("command-prefix-input")).toBeNull();

    fireEvent.click(screen.getByTestId("profile-advanced-options-trigger"));
    expect(screen.getByTestId("command-prefix-input")).not.toBeNull();
  });

  it("places fallback settings before advanced options at the bottom of the form", () => {
    renderForm(formData({ cli_passthrough: false }));

    const fallback = screen.getByTestId("profile-fallback-settings");
    const advanced = screen.getByTestId("profile-advanced-options");
    expect(
      fallback.compareDocumentPosition(advanced) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("hides the command prefix field for a TUI-passthrough profile", () => {
    renderForm(formData({ cli_passthrough: true, command_prefix: "greywall --" }));

    expect(screen.queryByTestId("command-prefix-input")).toBeNull();
  });
});

describe("ProfileFormFields Cursor MCP auth preference", () => {
  it("does not show the preference for an unsupported profile", () => {
    renderForm(formData(), modelConfig, vi.fn(), false);
    expect(
      screen.queryByRole("checkbox", { name: /Share local Cursor MCP credentials/ }),
    ).toBeNull();
  });

  it("shows a default-enabled checkbox and emits an explicit false value", () => {
    const onChange = vi.fn();
    renderStatefulForm(formData(), modelConfig, onChange, true);

    const checkbox = screen.getByRole("checkbox", { name: /Share local Cursor MCP credentials/ });
    expect(checkbox.getAttribute("data-state")).toBe("checked");
    fireEvent.click(checkbox);
    expect(onChange).toHaveBeenCalledWith({ cursor_mcp_auth_enabled: false });
    expect(screen.getByText(/Running sessions keep credentials already loaded/)).toBeTruthy();
  });
});

describe("ProfileFormFields no-silent-model-fallback rows", () => {
  it("renders the start model trigger red when the configured model is gone", () => {
    renderForm(formData({ model: "claude-gone" }));

    const trigger = screen.getByRole("button", { name: profileStartModelSettingsLabel });
    expect(trigger.className).toContain("text-destructive");
    expect(trigger.textContent).toContain("claude-gone");
  });

  it("does not infer an advertised variation for a gone saved model", () => {
    renderForm(formData({ model: "opus" }), {
      ...modelConfig,
      available_models: [{ id: "opus[1m]", name: "Opus (1m)" }],
    });

    expect(screen.queryByTestId("profile-model-variation-advisory")).toBeNull();
    expect(
      screen.getByRole("button", { name: profileStartModelSettingsLabel }).textContent,
    ).toContain("opus");
  });

  it("shows the agent fallback row when auto-fallback is off", () => {
    renderForm(formData({ auto_fallback: false }));
    expandFallbackSettings();
    expect(screen.queryByTestId("profile-fallback-model-field")).not.toBeNull();
  });

  it("keeps the agent fallback row visible but disabled when auto-fallback is on", () => {
    renderForm(formData({ auto_fallback: true }));
    expandFallbackSettings();
    expect(screen.queryByTestId("profile-fallback-model-field")).not.toBeNull();
    expect(screen.getByRole("switch", { name: "Agent fallback" })).toHaveProperty("disabled", true);
    expect(screen.queryByTestId("profile-auto-fallback-field")).not.toBeNull();
  });

  it("disables fallback controls while exact model is required", () => {
    renderForm(formData({ require_exact_model: true, fallback_model: "mock-fast" }));
    expandFallbackSettings();
    expect(
      screen.getByRole("switch", { name: "Require exact model" }).getAttribute("data-state"),
    ).toBe("checked");
    expect(
      screen.getByRole("switch", { name: "Fallback automatically to next model" }),
    ).toHaveProperty("disabled", true);
    expect(screen.getByRole("switch", { name: "Agent fallback" })).toHaveProperty("disabled", true);
    expect(screen.getByTestId("profile-fallback-settings-summary").textContent).toContain(
      "Exact model required",
    );
  });

  it("marks a gone fallback model red in its picker", () => {
    renderForm(formData({ fallback_model: "gpt-gone" }));
    expandFallbackSettings();
    const trigger = screen.getByRole("button", { name: "Agent fallback model settings" });
    expect(trigger.className).toContain("text-destructive");
    expect(trigger.textContent).toContain("gpt-gone");
  });

  it("hides the fallback selector until its attached switch is on", () => {
    renderForm(formData({ auto_fallback: false }));
    expandFallbackSettings();
    // The optional fallback row renders its label + switch, but the model
    // selector only appears once the user opts in via the attached switch.
    expect(screen.queryByTestId("profile-fallback-model-field")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Agent fallback model settings" })).toBeNull();
  });

  it("shows the fallback selector when a fallback model is configured", () => {
    renderForm(formData({ fallback_model: "mock-fast" }));
    expandFallbackSettings();
    expect(screen.getByRole("button", { name: "Agent fallback model settings" })).not.toBeNull();
  });

  it("clears the fallback model when its attached switch is turned off", () => {
    const onChange = vi.fn();
    renderForm(formData({ fallback_model: "mock-fast" }), modelConfig, onChange);
    expandFallbackSettings();
    const fallbackToggle = screen.getByRole("switch", { name: "Agent fallback" });
    fallbackToggle.click();
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ fallback_model: "" }));
  });
});

describe("ProfileFormFields Copilot model options", () => {
  const opusId = "claude-opus-5";
  const opusName = "Claude Opus 5";
  const haikuId = "claude-haiku-4.5";
  const haikuName = "Claude Haiku 4.5";
  const copilotModelConfig: ModelConfig = {
    default_model: opusId,
    available_models: [
      { id: opusId, name: opusName, meta: { copilotUsage: "15x" } },
      { id: haikuId, name: haikuName, meta: { copilotUsage: "0.33x" } },
    ],
    supports_dynamic_models: false,
    config_options: [
      {
        type: "select",
        id: "model",
        name: "Model",
        category: "model",
        current_value: opusId,
        options: [
          { value: opusId, name: opusName, description: opusName },
          { value: haikuId, name: haikuName, description: haikuName },
        ],
      },
    ],
  };

  it("shows the usage multiplier and drops the duplicated name from the config-option list", () => {
    renderForm(formData({ model: opusId }), copilotModelConfig);

    fireEvent.click(screen.getByRole("button", { name: profileStartModelSettingsLabel }));

    const opusOption = screen.getByRole("option", { name: /Claude Opus 5/ });
    // The multiplier survives the config-option path (it lives on available_models meta).
    expect(within(opusOption).getByText("15x")).not.toBeNull();
    // The duplicated name is replaced by the model id, not shown twice.
    expect(within(opusOption).getByText(opusId)).not.toBeNull();
    expect(within(opusOption).queryAllByText(opusName)).toHaveLength(1);

    const haikuOption = screen.getByRole("option", { name: /Claude Haiku 4\.5/ });
    expect(within(haikuOption).getByText("0.33x")).not.toBeNull();
  });
});

describe("ProfileFormFields model options", () => {
  it("uses a free-text model input for an OpenAI-compatible provider", async () => {
    __resetModelConfigResolutionCache();
    vi.mocked(fetchDynamicModels).mockClear();
    vi.mocked(resolveAgentModelConfig).mockClear();
    const onChange = vi.fn();

    renderForm(
      formData({ provider_kind: "openai_compatible", model: "gateway-model" }),
      { ...modelConfig, supports_dynamic_models: true },
      onChange,
    );

    const input = screen.getByTestId("profile-model-input");
    expect((input as HTMLInputElement).value).toBe("gateway-model");
    fireEvent.change(input, { target: { value: "gateway-only" } });
    expect(onChange).toHaveBeenCalledWith({ model: "gateway-only" });
    await waitFor(() => {
      expect(fetchDynamicModels).not.toHaveBeenCalled();
      expect(resolveAgentModelConfig).not.toHaveBeenCalled();
    });
  });

  it("constrains a single start model field on desktop", () => {
    renderForm(formData());

    const row = screen.getByTestId("profile-capabilities-model-row");
    expect(row.firstElementChild?.className).toContain("md:max-w-xl");
  });

  it("keeps the model and mode fields balanced when modes are available", () => {
    renderForm(formData({ mode: "default" }), {
      ...modelConfig,
      available_modes: [{ id: "default", name: "Default" }],
      current_mode_id: "default",
    });

    const row = screen.getByTestId("profile-capabilities-model-row");
    expect(row.firstElementChild?.className).toContain("flex-1");
    expect(screen.getByTestId("profile-mode-field")).not.toBeNull();
  });

  it("loads model-specific options in the profile model selector", async () => {
    const dynamicModelConfig: ModelConfig = {
      default_model: "model-a",
      current_model_id: "model-b",
      available_models: [
        { id: "model-a", name: "Model A" },
        { id: "model-b", name: "Model B" },
      ],
      config_options: [],
      supports_dynamic_models: true,
    };

    renderForm(formData({ model: "model-a" }), dynamicModelConfig);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: profileStartModelSettingsLabel })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: profileStartModelSettingsLabel }));

    expect(
      await screen.findByTestId(`config-option-trigger-${reasoningEffortOptionId}`),
    ).toBeTruthy();
  });

  it("preserves a saved option value during the initial resolution", async () => {
    const onChange = vi.fn();
    const dynamicModelConfig: ModelConfig = {
      default_model: "model-a",
      current_model_id: "model-a",
      available_models: [{ id: "model-a", name: "Model A" }],
      config_options: [
        {
          type: "select",
          id: reasoningEffortOptionId,
          name: reasoningEffortName,
          current_value: "medium",
          options: [{ value: "medium", name: "Medium" }],
        },
      ],
      supports_dynamic_models: true,
    };

    renderForm(
      formData({ model: "model-a", config_options: { [reasoningEffortOptionId]: "medium" } }),
      dynamicModelConfig,
      onChange,
    );

    await waitFor(() =>
      expect(screen.getByRole("button", { name: profileStartModelSettingsLabel })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: profileStartModelSettingsLabel }));
    expect(
      await screen.findByTestId(`config-option-trigger-${reasoningEffortOptionId}`),
    ).toBeTruthy();
    expect(onChange).not.toHaveBeenCalled();
  });
});

describe("ProfileFormFields initial model options", () => {
  it("uses matching initial model options without probing again", async () => {
    __resetModelConfigResolutionCache();
    vi.mocked(resolveAgentModelConfig).mockClear();
    const dynamicModelConfig: ModelConfig = {
      default_model: "model-a",
      current_model_id: "model-a",
      available_models: [{ id: "model-a", name: "Model A" }],
      config_options: [
        {
          type: "select",
          id: reasoningEffortOptionId,
          name: reasoningEffortName,
          current_value: "medium",
          options: [{ value: "medium", name: "Medium" }],
        },
      ],
      supports_dynamic_models: true,
    };

    renderForm(formData({ model: "model-a" }), dynamicModelConfig);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: profileStartModelSettingsLabel })).toBeTruthy(),
    );

    expect(resolveAgentModelConfig).not.toHaveBeenCalled();
  });

  it("uses a matching initial model snapshot without probing again", async () => {
    __resetModelConfigResolutionCache();
    vi.mocked(resolveAgentModelConfig).mockClear();
    const dynamicModelConfig: ModelConfig = {
      default_model: "model-a",
      current_model_id: "model-a",
      available_models: [{ id: "model-a", name: "Model A" }],
      config_options: [],
      supports_dynamic_models: true,
      status: "ok",
    };

    renderForm(formData({ model: "model-a" }), dynamicModelConfig);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: profileStartModelSettingsLabel })).toBeTruthy(),
    );

    expect(resolveAgentModelConfig).not.toHaveBeenCalled();
  });

  it("does not probe again when the initial capability snapshot failed", async () => {
    __resetModelConfigResolutionCache();
    vi.mocked(resolveAgentModelConfig).mockClear();
    const dynamicModelConfig: ModelConfig = {
      default_model: "mock-fast",
      current_model_id: "mock-fast",
      available_models: [],
      config_options: [],
      supports_dynamic_models: true,
      status: "failed",
      error: "mock-agent is not available",
    };

    renderForm(formData({ model: "mock-fast" }), dynamicModelConfig);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: profileStartModelSettingsLabel })).toBeTruthy(),
    );

    expect(resolveAgentModelConfig).not.toHaveBeenCalled();
  });
});

describe("ProfileFormFields model options after selection", () => {
  it("keeps the model selector open while resolving the selected model options", async () => {
    __resetModelConfigResolutionCache();
    let resolveResponse: ((value: unknown) => void) | undefined;
    const response = new Promise((resolve) => {
      resolveResponse = resolve;
    });
    vi.mocked(resolveAgentModelConfig).mockReturnValueOnce(response as never);

    const dynamicModelConfig: ModelConfig = {
      default_model: "model-a",
      current_model_id: "model-a",
      available_models: [
        { id: "model-a", name: "Model A" },
        { id: "model-b", name: "Model B" },
      ],
      config_options: [
        {
          type: "select",
          id: reasoningEffortOptionId,
          name: reasoningEffortName,
          current_value: "medium",
          options: [{ value: "medium", name: "Medium" }],
        },
      ],
      supports_dynamic_models: true,
    };

    renderStatefulForm(
      formData({ model: "model-a", config_options: { [reasoningEffortOptionId]: "medium" } }),
      dynamicModelConfig,
      vi.fn(),
    );

    const selector = await screen.findByRole("button", { name: profileStartModelSettingsLabel });
    fireEvent.click(selector);
    fireEvent.click(screen.getByRole("option", { name: /Model B/ }));

    await waitFor(() => expect(screen.getByTestId("model-config-options-loading")).toBeTruthy());
    expect(screen.queryByTestId(`config-option-trigger-${reasoningEffortOptionId}`)).toBeNull();

    await act(async () => {
      resolveResponse?.({
        agent_name: "mock-agent",
        model: "model-b",
        status: "ok",
        config_options: [
          {
            type: "select",
            id: reasoningEffortOptionId,
            name: reasoningEffortName,
            current_value: "max",
            options: [{ value: "max", name: "Max" }],
          },
        ],
        error: null,
      });
    });

    await waitFor(() =>
      expect(screen.getByTestId("config-option-trigger-reasoning_effort")).toBeTruthy(),
    );
    expect(screen.queryByTestId("model-config-options-loading")).toBeNull();
  });

  it("removes a saved option value after the user changes the model", async () => {
    const onChange = vi.fn();
    const dynamicModelConfig: ModelConfig = {
      default_model: "model-a",
      current_model_id: "model-a",
      available_models: [
        { id: "model-a", name: "Model A" },
        { id: "model-b", name: "Model B" },
      ],
      config_options: [
        {
          type: "select",
          id: reasoningEffortOptionId,
          name: reasoningEffortName,
          current_value: "medium",
          options: [{ value: "medium", name: "Medium" }],
        },
      ],
      supports_dynamic_models: true,
    };

    renderStatefulForm(
      formData({ model: "model-a", config_options: { [reasoningEffortOptionId]: "medium" } }),
      dynamicModelConfig,
      onChange,
    );

    await waitFor(() =>
      expect(screen.getByRole("button", { name: profileStartModelSettingsLabel })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: profileStartModelSettingsLabel }));
    expect(
      await screen.findByTestId(`config-option-trigger-${reasoningEffortOptionId}`),
    ).toBeTruthy();
    fireEvent.click(await screen.findByText("Model B"));

    await waitFor(() => expect(onChange).toHaveBeenCalledWith({ config_options: {} }));
  });
});

describe("ProfileFormFields save coordination", () => {
  it("blocks the coordinated profile save while model options are resolving", async () => {
    __resetModelConfigResolutionCache();
    let resolveResponse: ((value: unknown) => void) | undefined;
    const response = new Promise((resolve) => {
      resolveResponse = resolve;
    });
    vi.mocked(resolveAgentModelConfig).mockReturnValueOnce(response as never);
    const save = vi.fn();
    const dynamicModelConfig: ModelConfig = {
      default_model: "model-a",
      current_model_id: "model-a",
      available_models: [{ id: "model-a", name: "Model A" }],
      config_options: [],
      supports_dynamic_models: true,
    };

    function SaveHarness() {
      const [pending, setPending] = useState(false);
      useSettingsSaveContributor({
        id: "profile:model-config",
        revision: 1,
        isDirty: true,
        canSave: !pending,
        invalidReason: pending ? "Loading model options" : undefined,
        save,
        discard: vi.fn(),
      });
      return (
        <ProfileFormFields
          profile={formData({ model: "model-a" })}
          onChange={vi.fn()}
          modelConfig={dynamicModelConfig}
          permissionSettings={{}}
          passthroughConfig={null}
          agentName="mock-agent"
          onModelConfigResolutionPendingChange={setPending}
        />
      );
    }

    render(
      <StateProvider>
        <SettingsSaveProvider>
          <TooltipProvider>
            <SaveHarness />
          </TooltipProvider>
        </SettingsSaveProvider>
      </StateProvider>,
    );

    const saveButton = await screen.findByRole("button", { name: "Save changes" });
    await waitFor(() => expect(saveButton.hasAttribute("disabled")).toBe(true));
    fireEvent.click(saveButton);
    expect(save).not.toHaveBeenCalled();

    await act(async () => {
      resolveResponse?.({
        agent_name: "mock-agent",
        model: "model-a",
        status: "ok",
        config_options: [],
        error: null,
      });
    });
    await waitFor(() => expect(saveButton.hasAttribute("disabled")).toBe(false));
    fireEvent.click(saveButton);
    await waitFor(() => expect(save).toHaveBeenCalledOnce());
  });
});
