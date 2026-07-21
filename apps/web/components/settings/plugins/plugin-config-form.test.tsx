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

afterEach(cleanup);

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

  it("lists configured utility agents and submits the selected name", async () => {
    listUtilityAgents.mockResolvedValue({
      agents: [
        { id: "builtin", name: "Summarize", enabled: true },
        { id: "custom", name: "Disabled custom", enabled: false },
      ],
    });
    const onChange = vi.fn();
    render(
      <PluginConfigForm
        fields={[
          {
            name: "utility_agent",
            label: "Utility agent",
            type: "utility_agent",
            required: true,
            secret: false,
          },
        ]}
        values={{ utility_agent: "" }}
        initialValues={{ utility_agent: "" }}
        disabled={false}
        onChange={onChange}
      />,
    );

    const trigger = screen.getByRole("combobox");
    await waitFor(() => expect(listUtilityAgents).toHaveBeenCalled());
    fireEvent.click(trigger);

    const enabled = await screen.findByText("Summarize");
    const disabled = screen.getByText("Disabled custom");
    expect(disabled.closest('[role="option"]')?.getAttribute("data-disabled")).toBe("");
    fireEvent.click(enabled);
    expect(onChange).toHaveBeenCalledWith("utility_agent", "Summarize");
  });
});
