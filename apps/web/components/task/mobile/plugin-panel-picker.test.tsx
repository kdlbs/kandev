import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { PluginPanelPicker } from "./plugin-panel-picker";

const PLUGIN_A = "picker-plugin-a";
const PLUGIN_B = "picker-plugin-b";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_A);
  pluginRegistry.unregisterPlugin(PLUGIN_B);
});

describe("PluginPanelPicker", () => {
  it("renders every mobile registration as a touch-sized option, including duplicate titles", () => {
    function Notes() {
      return null;
    }
    pluginRegistry
      .forPlugin(PLUGIN_A)
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes, mobileEnabled: true });
    pluginRegistry
      .forPlugin(PLUGIN_B)
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes, mobileEnabled: true });

    render(<PluginPanelPicker open onOpenChange={vi.fn()} onSelect={vi.fn()} />);

    const options = screen.getAllByTestId(/^mobile-plugin-panel-option-/);
    expect(options).toHaveLength(2);
    expect(options.map((option) => option.textContent)).toEqual(["Notes", "Notes"]);
    expect(options.every((option) => option.className.includes("min-h-11"))).toBe(true);
    expect(screen.getByText("Panels")).toBeTruthy();
  });

  it("selects by stable panel id and dismisses the sheet", () => {
    function Notes() {
      return null;
    }
    pluginRegistry
      .forPlugin(PLUGIN_B)
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes, mobileEnabled: true });
    const onOpenChange = vi.fn();
    const onSelect = vi.fn();

    render(<PluginPanelPicker open onOpenChange={onOpenChange} onSelect={onSelect} />);

    fireEvent.click(screen.getByTestId("mobile-plugin-panel-option-picker-plugin-b-notes"));

    expect(onSelect).toHaveBeenCalledWith("plugin:picker-plugin-b:notes");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
