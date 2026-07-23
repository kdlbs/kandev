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
