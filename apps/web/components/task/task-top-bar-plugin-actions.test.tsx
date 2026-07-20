import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { TaskTopBarPluginActions, type ChatTopBarSlotProps } from "./task-top-bar-plugin-actions";

const SLOT = "chat-top-bar";

// Minimal store: the wrapper only reads taskSessionsByTask.itemsByTaskId.
const mockState = {
  taskSessionsByTask: {
    itemsByTaskId: {
      t1: [
        { id: "s1", task_id: "t1" },
        { id: "s2", task_id: "t1" },
      ],
    },
  },
};

vi.mock("@/components/state-provider", () => ({
  useOptionalAppStore: (selector: (s: typeof mockState) => unknown) => selector(mockState),
}));

describe("TaskTopBarPluginActions", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin("plugin-a");
  });

  it("renders nothing when no plugin registered a chat-top-bar component", () => {
    const { container } = render(
      <TaskTopBarPluginActions sessionId="s1" taskId="t1" taskTitle="Demo" workspaceId="w1" />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("forwards task/workspace context, the active session, and all session ids", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatTopBarSlotProps;
      return (
        <div data-testid="plugin-topbar">
          {`${ctx.taskId}|${ctx.workspaceId}|${ctx.activeSessionId}|${ctx.sessionIds.join(",")}`}
        </div>
      );
    });

    render(
      <TaskTopBarPluginActions sessionId="s2" taskId="t1" taskTitle="Demo" workspaceId="w1" />,
    );

    expect(screen.getByTestId("plugin-topbar").textContent).toBe("t1|w1|s2|s1,s2");
  });

  it("includes the active session id even when the store list omits it", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatTopBarSlotProps;
      return <div data-testid="plugin-topbar">{ctx.sessionIds.join(",")}</div>;
    });

    // taskId with no store entry -> only the active session propagates.
    render(
      <TaskTopBarPluginActions
        sessionId="s9"
        taskId="t-unknown"
        taskTitle="Demo"
        workspaceId="w1"
      />,
    );

    expect(screen.getByTestId("plugin-topbar").textContent).toBe("s9");
  });

  it("propagates only the active session when taskId is null", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatTopBarSlotProps;
      return (
        <div data-testid="plugin-topbar">{`${ctx.taskId}|${ctx.activeSessionId}|${ctx.sessionIds.join(",")}`}</div>
      );
    });

    render(<TaskTopBarPluginActions sessionId="s9" taskId={null} workspaceId={null} />);

    expect(screen.getByTestId("plugin-topbar").textContent).toBe("null|s9|s9");
  });
});
