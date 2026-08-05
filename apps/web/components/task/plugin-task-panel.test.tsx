import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

let mockActiveTaskId: string | null = "task_1";
let mockActiveSessionId: string | null = "session_1";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      tasks: { activeTaskId: mockActiveTaskId, activeSessionId: mockActiveSessionId },
    }),
}));

import { pluginRegistry } from "@/lib/plugins/registry";
import { PluginTaskPanel } from "./plugin-task-panel";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("plugin-a");
  mockActiveTaskId = "task_1";
  mockActiveSessionId = "session_1";
});

describe("PluginTaskPanel", () => {
  it("renders the registered Component with PluginTaskPanelProps (AC2)", () => {
    function Notes(props: {
      panelId: string;
      taskId: string;
      sessionId: string | null;
      presentation: string;
    }) {
      return (
        <div data-testid="notes-body">
          {props.panelId}|{props.taskId}|{props.sessionId}|{props.presentation}
        </div>
      );
    }
    pluginRegistry
      .forPlugin("plugin-a")
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes });

    render(
      <PluginTaskPanel
        pluginId="plugin-a"
        panelKey="notes"
        panelId="plugin:plugin-a:notes"
        presentation="desktop"
      />,
    );

    expect(screen.getByTestId("notes-body").textContent).toBe(
      "plugin:plugin-a:notes|task_1|session_1|desktop",
    );
  });

  it("renders a not-available fallback when the plugin is no longer registered (AC5)", () => {
    render(
      <PluginTaskPanel
        pluginId="plugin-gone"
        panelKey="notes"
        panelId="plugin:plugin-gone:notes"
        presentation="desktop"
      />,
    );

    expect(screen.getByText("This panel is no longer available.")).not.toBeNull();
  });

  it("renders the error boundary fallback when the plugin Component throws (AC6)", () => {
    function Throws(): never {
      throw new Error("boom");
    }
    pluginRegistry
      .forPlugin("plugin-a")
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Throws });
    const originalConsoleError = console.error;
    console.error = () => {};

    render(
      <PluginTaskPanel
        pluginId="plugin-a"
        panelKey="notes"
        panelId="plugin:plugin-a:notes"
        presentation="desktop"
      />,
    );

    expect(screen.getByText("This plugin panel failed to load.")).not.toBeNull();
    console.error = originalConsoleError;
  });
});
