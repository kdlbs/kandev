import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

let mockActiveTaskId: string | null = "task_1";
let mockActiveSessionId: string | null = "session_1";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      tasks: { activeTaskId: mockActiveTaskId, activeSessionId: mockActiveSessionId },
      taskSessions: {
        items: {
          session_1: { id: "session_1", is_passthrough: false, name: "Session" },
        },
      },
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

  it("passes session kind and a presentation-scoped navigation facade", () => {
    const onOpenMessage = vi.fn(() => ({ status: "accepted" as const }));
    function Notes(props: {
      sessionKind: string | null;
      conversation: { openMessage: (messageId: string) => { status: string } };
    }) {
      return (
        <button type="button" onClick={() => props.conversation.openMessage("message-1")}>
          {props.sessionKind}
        </button>
      );
    }
    pluginRegistry
      .forPlugin("plugin-a")
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes, mobileEnabled: true });

    render(
      <PluginTaskPanel
        pluginId="plugin-a"
        panelKey="notes"
        panelId="plugin:plugin-a:notes"
        presentation="mobile"
        onOpenMessage={onOpenMessage}
      />,
    );

    screen.getByRole("button", { name: "managed" }).click();
    expect(onOpenMessage).toHaveBeenCalledWith("message-1");
  });

  it("reports unavailable for invalid or rejected mobile navigation", () => {
    let emptyResult = "";
    let rejectedResult = "";
    const onOpenMessage = vi.fn(() => ({ status: "unavailable" as const }));
    function Notes(props: {
      conversation: { openMessage: (messageId: string) => { status: string } };
    }) {
      return (
        <>
          <button
            type="button"
            onClick={() => (emptyResult = props.conversation.openMessage(" ").status)}
          >
            empty
          </button>
          <button
            type="button"
            onClick={() => (rejectedResult = props.conversation.openMessage("message-1").status)}
          >
            rejected
          </button>
        </>
      );
    }
    pluginRegistry
      .forPlugin("plugin-a")
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes, mobileEnabled: true });

    render(
      <PluginTaskPanel
        pluginId="plugin-a"
        panelKey="notes"
        panelId="plugin:plugin-a:notes"
        presentation="mobile"
        onOpenMessage={onOpenMessage}
      />,
    );

    screen.getByRole("button", { name: "empty" }).click();
    expect(emptyResult).toBe("unavailable");
    expect(onOpenMessage).not.toHaveBeenCalled();

    screen.getByRole("button", { name: "rejected" }).click();
    expect(rejectedResult).toBe("unavailable");
    expect(onOpenMessage).toHaveBeenCalledWith("message-1");
  });
});
describe("PluginTaskPanel session lifecycle", () => {
  it("keeps the panel bound to a deleted active session", () => {
    function Notes(props: { sessionId: string | null }) {
      return <div data-testid="notes-session">{props.sessionId}</div>;
    }
    pluginRegistry
      .forPlugin("plugin-a")
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes });

    const view = render(
      <PluginTaskPanel
        pluginId="plugin-a"
        panelKey="notes"
        panelId="plugin:plugin-a:notes"
        presentation="desktop"
      />,
    );
    expect(screen.getByTestId("notes-session").textContent).toBe("session_1");

    mockActiveSessionId = null;
    view.rerender(
      <PluginTaskPanel
        pluginId="plugin-a"
        panelKey="notes"
        panelId="plugin:plugin-a:notes"
        presentation="desktop"
      />,
    );
    expect(screen.getByTestId("notes-session").textContent).toBe("session_1");
  });
});

describe("PluginTaskPanel failure containment", () => {
  it("fails closed when a managed visibility predicate throws", () => {
    function Notes() {
      return <div>secret panel</div>;
    }
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    pluginRegistry.forPlugin("plugin-a").registerTaskPanel({
      id: "notes",
      title: "Notes",
      Component: Notes,
      visible: () => {
        throw new Error("predicate failed");
      },
    });

    render(
      <PluginTaskPanel
        pluginId="plugin-a"
        panelKey="notes"
        panelId="plugin:plugin-a:notes"
        presentation="desktop"
      />,
    );

    expect(screen.queryByText("secret panel")).toBeNull();
    expect(screen.getByText("This panel is no longer available.")).not.toBeNull();
    expect(consoleError).toHaveBeenCalledWith(
      expect.stringContaining("plugin-a:notes"),
      expect.any(Error),
    );
    consoleError.mockRestore();
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
describe("PluginTaskPanel lease", () => {
  it("revokes a panel navigation lease when visibility changes", () => {
    let visible = true;
    let openMessage: ((messageId: string) => { status: string }) | undefined;
    const onOpenMessage = vi.fn(() => ({ status: "accepted" as const }));
    function Notes(props: {
      conversation: { openMessage: (messageId: string) => { status: string } };
    }) {
      openMessage = props.conversation.openMessage;
      return <div>visible panel</div>;
    }
    pluginRegistry.forPlugin("plugin-a").registerTaskPanel({
      id: "notes",
      title: "Notes",
      Component: Notes,
      mobileEnabled: true,
      visible: () => visible,
    });

    const view = render(
      <PluginTaskPanel
        pluginId="plugin-a"
        panelKey="notes"
        panelId="plugin:plugin-a:notes"
        presentation="mobile"
        onOpenMessage={onOpenMessage}
      />,
    );
    const initialOpenMessage = openMessage;
    expect(initialOpenMessage?.("message-1").status).toBe("accepted");
    expect(onOpenMessage).toHaveBeenCalledWith("message-1");

    visible = false;
    view.rerender(
      <PluginTaskPanel
        pluginId="plugin-a"
        panelKey="notes"
        panelId="plugin:plugin-a:notes"
        presentation="mobile"
        onOpenMessage={onOpenMessage}
      />,
    );

    expect(screen.getByText("This panel is no longer available.")).not.toBeNull();
    expect(initialOpenMessage?.("message-2").status).toBe("unavailable");
    expect(onOpenMessage).toHaveBeenCalledTimes(1);
  });
});
