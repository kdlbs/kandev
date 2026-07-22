import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PluginConfigField } from "@/lib/plugins/config-schema";
import { PluginConfigForm } from "./plugin-config-form";

const { listUtilityAgents } = vi.hoisted(() => ({ listUtilityAgents: vi.fn() }));

vi.mock("@/lib/api/domains/utility-api", () => ({ listUtilityAgents }));

const fields: PluginConfigField[] = [
  {
    name: "greeting",
    label: "Greeting",
    type: "string",
    required: false,
    secret: false,
  },
  {
    name: "enabled",
    label: "Enabled",
    type: "boolean",
    required: false,
    secret: false,
  },
];

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function utilityAgentField(required: boolean): PluginConfigField {
  return {
    name: "utility_agent",
    label: "Utility agent",
    type: "utility_agent",
    required,
    secret: false,
  };
}

type UtilityAgentRenderOptions = {
  required?: boolean;
  value?: string;
  initialValue?: string;
  onChange?: ReturnType<typeof vi.fn<(name: string, value: string | boolean) => void>>;
};

function renderUtilityAgentField(options: UtilityAgentRenderOptions = {}) {
  const value = options.value ?? "";
  const onChange = options.onChange ?? vi.fn<(name: string, value: string | boolean) => void>();
  render(
    <PluginConfigForm
      fields={[utilityAgentField(options.required ?? true)]}
      values={{ utility_agent: value }}
      initialValues={{ utility_agent: options.initialValue ?? value }}
      disabled={false}
      onChange={onChange}
    />,
  );
  return onChange;
}

describe("PluginConfigForm", () => {
  it("marks only changed controls as dirty", () => {
    render(
      <PluginConfigForm
        fields={fields}
        values={{ greeting: "changed", enabled: false }}
        initialValues={{ greeting: "saved", enabled: false }}
        disabled={false}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("Greeting").getAttribute("data-settings-dirty")).toBe("true");
    expect(screen.getByLabelText("Enabled").getAttribute("data-settings-dirty")).toBe("false");
  });

  it("distinguishes duplicate names by submitting the selected stable ID", async () => {
    listUtilityAgents.mockResolvedValue({
      agents: [
        { id: "builtin", name: "Summarize", enabled: true },
        { id: "custom", name: "Summarize", enabled: true },
        { id: "disabled", name: "Disabled custom", enabled: false },
      ],
    });
    const onChange = renderUtilityAgentField();

    const trigger = screen.getByRole("combobox");
    await waitFor(() => expect(trigger.hasAttribute("disabled")).toBe(false));
    fireEvent.click(trigger);

    const enabled = await screen.findAllByRole("option", { name: "Summarize" });
    const disabled = screen.getByText("Disabled custom");
    expect(disabled.closest('[role="option"]')?.getAttribute("data-disabled")).toBe("");
    expect(screen.queryByText("Not set")).toBeNull();
    fireEvent.click(enabled[1]);
    expect(onChange).toHaveBeenCalledWith("utility_agent", "custom");
  });

  it("allows an optional utility agent selection to be cleared", async () => {
    listUtilityAgents.mockResolvedValue({
      agents: [{ id: "builtin", name: "Summarize", enabled: true }],
    });
    const onChange = renderUtilityAgentField({ required: false, value: "builtin" });

    const trigger = screen.getByRole("combobox");
    await waitFor(() => expect(trigger.hasAttribute("disabled")).toBe(false));
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByText("Not set"));
    expect(onChange).toHaveBeenCalledWith("utility_agent", "");
  });

  it("disables a saved selection while agents load without showing an empty placeholder", () => {
    listUtilityAgents.mockReturnValue(new Promise(() => undefined));
    renderUtilityAgentField({ required: false, value: "builtin" });

    expect(screen.getByRole("combobox").hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Loading selected utility agent...")).not.toBeNull();
    expect(screen.queryByText("Select a utility agent...")).toBeNull();
  });

  it("fails safely when utility agents cannot be loaded", async () => {
    listUtilityAgents.mockRejectedValue(new Error("offline"));
    renderUtilityAgentField({ required: false });

    expect(await screen.findByText("Utility agents unavailable")).not.toBeNull();
    expect(screen.getByRole("combobox").hasAttribute("disabled")).toBe(true);
  });

  it("marks a changed utility agent selection as dirty", async () => {
    listUtilityAgents.mockResolvedValue({
      agents: [{ id: "custom", name: "Summarize", enabled: true }],
    });
    renderUtilityAgentField({ value: "custom", initialValue: "builtin" });

    const trigger = screen.getByRole("combobox");
    await waitFor(() => expect(trigger.hasAttribute("disabled")).toBe(false));
    expect(trigger.getAttribute("data-settings-dirty")).toBe("true");
  });
});
