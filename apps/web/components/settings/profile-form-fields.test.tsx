import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { SettingsSaveProvider, useSettingsSaveContributor } from "./settings-save-provider";
import { resolveAgentModelConfig } from "@/lib/api/domains/settings-api";
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
        name: "Reasoning effort",
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
      />
    </TooltipProvider>,
  );
}

function renderStatefulForm(
  profile: ProfileFormData,
  config: ModelConfig,
  onChange: (patch: Partial<ProfileFormData>) => void,
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
      />
    );
  }

  return render(
    <TooltipProvider>
      <StatefulForm />
    </TooltipProvider>,
  );
}

describe("ProfileFormFields command prefix visibility", () => {
  it("shows the command prefix field for an ACP (non-passthrough) profile", () => {
    renderForm(formData({ cli_passthrough: false }));

    expect(screen.queryByTestId("command-prefix-input")).not.toBeNull();
  });

  it("hides the command prefix field for a TUI-passthrough profile", () => {
    renderForm(formData({ cli_passthrough: true, command_prefix: "greywall --" }));

    expect(screen.queryByTestId("command-prefix-input")).toBeNull();
  });
});

describe("ProfileFormFields model options", () => {
  it("loads model-specific options in the profile model selector", async () => {
    const dynamicModelConfig: ModelConfig = {
      default_model: "model-a",
      current_model_id: "model-a",
      available_models: [{ id: "model-a", name: "Model A" }],
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
          name: "Reasoning effort",
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
          name: "Reasoning effort",
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
      <SettingsSaveProvider>
        <TooltipProvider>
          <SaveHarness />
        </TooltipProvider>
      </SettingsSaveProvider>,
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
