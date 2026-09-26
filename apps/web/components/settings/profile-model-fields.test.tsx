import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ModelPicker, type ProfileFormData } from "./profile-model-fields";
import type { ModelDiscovery, ModelEntry } from "@/lib/types/http";

afterEach(cleanup);

const startModelAria = "Start model";
const goneLabel = "No longer available; select a different model";
const discoveryNoteTestId = "model-discovery-note";
const customModelId = "claude-opus-5-5";
const filterPlaceholder = "Filter models...";

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

const models: ModelEntry[] = [
  { id: "mock-fast", name: "Mock Fast" },
  { id: "mock-smart", name: "Mock Smart" },
];

function renderPicker(
  profile: ProfileFormData,
  discovery?: ModelDiscovery,
  onChange: (patch: Partial<ProfileFormData>) => void = vi.fn(),
) {
  return render(
    <TooltipProvider>
      <ModelPicker
        profile={profile}
        models={models}
        currentModelId={undefined}
        configOptions={[]}
        onChange={onChange}
        ariaLabel={startModelAria}
        goneModelLabel={goneLabel}
        discovery={discovery}
      />
    </TooltipProvider>,
  );
}

const cliDiscovery: ModelDiscovery = {
  source: "cli_command",
  executable: "codex",
  cli_version: "0.155.1",
  status: "ok",
  allows_custom_model: true,
};

describe("ModelPicker without discovery (non-host-CLI agent)", () => {
  it("keeps a gone saved model disabled with its reason, as before", () => {
    renderPicker(formData({ model: "claude-gone" }));

    const trigger = screen.getByRole("button", { name: startModelAria });
    expect(trigger.className).toContain("text-destructive");

    fireEvent.click(trigger);
    const option = screen.getByRole("option", { name: /claude-gone/i });
    expect(option.getAttribute("aria-disabled")).toBe("true");
  });

  it("renders no discovery note", () => {
    renderPicker(formData());
    expect(screen.queryByTestId(discoveryNoteTestId)).toBeNull();
  });
});

describe("ModelPicker with host CLI discovery", () => {
  it("shows a saved model absent from the list as a selectable custom entry", () => {
    const onChange = vi.fn();
    renderPicker(formData({ model: customModelId }), cliDiscovery, onChange);

    const trigger = screen.getByRole("button", { name: startModelAria });
    expect(trigger.className).not.toContain("text-destructive");

    fireEvent.click(trigger);
    const option = screen.getByRole("option", { name: /claude-opus-5-5/i });
    expect(option.getAttribute("aria-disabled")).not.toBe("true");

    fireEvent.click(option);
    expect(onChange).toHaveBeenCalledWith({ model: customModelId });
  });

  it("renders the discovery note with the CLI source and version", () => {
    renderPicker(formData(), cliDiscovery);
    expect(screen.getByTestId(discoveryNoteTestId).textContent).toBe("Models from codex 0.155.1");
  });

  it("renders the no-listing note when the CLI publishes no model list", () => {
    renderPicker(formData(), {
      source: "acp_probe",
      executable: "claude",
      status: "skipped",
      allows_custom_model: true,
    });
    expect(screen.getByTestId(discoveryNoteTestId).textContent).toBe(
      "claude does not publish a model list. Type a model ID to use another model.",
    );
  });

  it("renders the failure note with the reported reason", () => {
    renderPicker(formData(), {
      source: "acp_probe",
      executable: "codex",
      status: "not_logged_in",
      error: "the command-line tool is not signed in",
      allows_custom_model: true,
    });
    expect(screen.getByTestId(discoveryNoteTestId).textContent).toBe(
      "Model discovery failed: the command-line tool is not signed in",
    );
  });

  it("offers a custom row for typed text with no exact match and selects it verbatim", () => {
    const onChange = vi.fn();
    renderPicker(formData(), cliDiscovery, onChange);

    fireEvent.click(screen.getByRole("button", { name: startModelAria }));
    fireEvent.change(screen.getByPlaceholderText(filterPlaceholder), {
      target: { value: customModelId },
    });

    const customRow = screen.getByTestId("model-config-custom-row");
    expect(customRow.textContent).toContain(`Use "${customModelId}" as model ID`);

    fireEvent.click(customRow);
    expect(onChange).toHaveBeenCalledWith({ model: customModelId });
  });

  it("does not offer the custom row once the typed text exactly matches a listed model", () => {
    renderPicker(formData(), cliDiscovery);

    fireEvent.click(screen.getByRole("button", { name: startModelAria }));
    fireEvent.change(screen.getByPlaceholderText(filterPlaceholder), {
      target: { value: "mock-fast" },
    });

    expect(screen.queryByTestId("model-config-custom-row")).toBeNull();
  });
});
