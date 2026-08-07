import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ProfileFormFields, type ProfileFormData } from "./profile-form-fields";
import type { ModelConfig } from "@/lib/types/http";

afterEach(cleanup);

const modelConfig: ModelConfig = {
  default_model: "mock-fast",
  available_models: [{ id: "mock-fast", name: "Mock Fast" }],
  supports_dynamic_models: false,
};

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

function renderForm(profile: ProfileFormData) {
  return render(
    <TooltipProvider>
      <ProfileFormFields
        profile={profile}
        onChange={vi.fn()}
        modelConfig={modelConfig}
        permissionSettings={{}}
        passthroughConfig={null}
        agentName="mock-agent"
      />
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

describe("ProfileFormFields no-silent-model-fallback rows", () => {
  it("renders the start model trigger red when the configured model is gone", () => {
    renderForm(formData({ model: "claude-gone" }));

    const trigger = screen.getByRole("button", { name: "Profile start model settings" });
    expect(trigger.className).toContain("text-destructive");
    expect(trigger.textContent).toContain("claude-gone");
  });

  it("shows the agent fallback row when auto-fallback is off", () => {
    renderForm(formData({ auto_fallback: false }));
    expect(screen.queryByTestId("profile-fallback-model-field")).not.toBeNull();
  });

  it("hides the agent fallback row when auto-fallback is on", () => {
    renderForm(formData({ auto_fallback: true }));
    expect(screen.queryByTestId("profile-fallback-model-field")).toBeNull();
    expect(screen.queryByTestId("profile-auto-fallback-field")).not.toBeNull();
  });

  it("marks a gone fallback model red in its picker", () => {
    renderForm(formData({ fallback_model: "gpt-gone" }));
    const trigger = screen.getByRole("button", { name: "Agent fallback model settings" });
    expect(trigger.className).toContain("text-destructive");
    expect(trigger.textContent).toContain("gpt-gone");
  });

  it("hides the fallback selector until its attached switch is on", () => {
    renderForm(formData({ auto_fallback: false }));
    // The optional fallback row renders its label + switch, but the model
    // selector only appears once the user opts in via the attached switch.
    expect(screen.queryByTestId("profile-fallback-model-field")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Agent fallback model settings" })).toBeNull();
  });

  it("shows the fallback selector when a fallback model is configured", () => {
    renderForm(formData({ fallback_model: "mock-fast" }));
    expect(screen.getByRole("button", { name: "Agent fallback model settings" })).not.toBeNull();
  });

  it("clears the fallback model when its attached switch is turned off", () => {
    const onChange = vi.fn();
    render(
      <TooltipProvider>
        <ProfileFormFields
          profile={formData({ fallback_model: "mock-fast" })}
          onChange={onChange}
          modelConfig={modelConfig}
          permissionSettings={{}}
          passthroughConfig={null}
          agentName="mock-agent"
        />
      </TooltipProvider>,
    );
    const fallbackToggle = screen.getByRole("switch", { name: "Agent fallback" });
    fallbackToggle.click();
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ fallback_model: "" }));
  });
});
