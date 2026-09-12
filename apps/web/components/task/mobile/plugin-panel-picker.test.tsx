import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      tasks: { activeTaskId: "task-1", activeSessionId: "session-1" },
      taskSessions: {
        items: {
          "session-1": { id: "session-1", is_passthrough: false },
        },
      },
    }),
}));
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

    render(<PluginPanelPicker taskId="task-1" open onOpenChange={vi.fn()} onSelect={vi.fn()} />);

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

    render(
      <PluginPanelPicker taskId="task-1" open onOpenChange={onOpenChange} onSelect={onSelect} />,
    );

    fireEvent.click(screen.getByTestId("mobile-plugin-panel-option-picker-plugin-b-notes"));

    expect(onSelect).toHaveBeenCalledWith("plugin:picker-plugin-b:notes");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("omits registrations whose visibility predicate rejects the active mobile context", () => {
    function Notes() {
      return null;
    }
    pluginRegistry.forPlugin(PLUGIN_A).registerTaskPanel({
      id: "notes",
      title: "Notes",
      Component: Notes,
      mobileEnabled: true,
      visible: (context) => context.presentation === "desktop",
    });

    render(<PluginPanelPicker taskId="task-1" open onOpenChange={vi.fn()} onSelect={vi.fn()} />);

    expect(screen.queryByTestId(`mobile-plugin-panel-option-${PLUGIN_A}-notes`)).toBeNull();
  });

  it("does not invoke plugin visibility without a task context", () => {
    function Notes() {
      return null;
    }
    const visible = vi.fn(() => true);
    pluginRegistry.forPlugin(PLUGIN_A).registerTaskPanel({
      id: "notes",
      title: "Notes",
      Component: Notes,
      mobileEnabled: true,
      visible,
    });

    render(<PluginPanelPicker open onOpenChange={vi.fn()} onSelect={vi.fn()} />);

    expect(visible).not.toHaveBeenCalled();
    expect(screen.queryByTestId(`mobile-plugin-panel-option-${PLUGIN_A}-notes`)).toBeNull();
  });
});
