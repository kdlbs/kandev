import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  TaskTopBarPluginActions,
  type ChatTopBarSlotProps,
  useHasTaskTopBarPluginActions,
} from "./task-top-bar-plugin-actions";

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
const originalSessions = mockState.taskSessionsByTask.itemsByTaskId.t1;

vi.mock("@/components/state-provider", () => ({
  useOptionalAppStore: (selector: (s: typeof mockState) => unknown) => selector(mockState),
}));

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("plugin-a");
  mockState.taskSessionsByTask.itemsByTaskId.t1 = originalSessions;
});

describe("TaskTopBarPluginActions", () => {
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
          {`${ctx.taskId}|${ctx.workspaceId}|${ctx.activeSessionId}|${ctx.sessionIds.join(",")}|${ctx.presentation}`}
        </div>
      );
    });

    render(
      <TaskTopBarPluginActions sessionId="s2" taskId="t1" taskTitle="Demo" workspaceId="w1" />,
    );

    expect(screen.getByTestId("plugin-topbar").textContent).toBe("t1|w1|s2|s1,s2|desktop");
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

  it("does not rerender the contribution for equal session ids", () => {
    const pluginRender = vi.fn();
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, () => {
      pluginRender();
      return null;
    });

    const { rerender } = render(
      <TaskTopBarPluginActions sessionId="s2" taskId="t1" taskTitle="Demo" workspaceId="w1" />,
    );
    mockState.taskSessionsByTask.itemsByTaskId.t1 = originalSessions.map((session) => ({
      ...session,
      state: "finished",
    }));
    rerender(
      <TaskTopBarPluginActions sessionId="s2" taskId="t1" taskTitle="Demo" workspaceId="w1" />,
    );

    expect(pluginRender).toHaveBeenCalledTimes(1);
  });
});

describe("responsive task top-bar plugin actions", () => {
  it("identifies and contains the mobile menu presentation", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatTopBarSlotProps;
      return <button data-testid="mobile-plugin-action">{ctx.presentation}</button>;
    });

    render(
      <TaskTopBarPluginActions
        sessionId="s2"
        taskId="t1"
        taskTitle="Demo"
        workspaceId="w1"
        presentation="mobile"
      />,
    );

    const wrapper = screen.getByTestId("mobile-chat-top-bar-plugin-actions");
    expect(wrapper.contains(screen.getByTestId("mobile-plugin-action"))).toBe(true);
    expect(screen.getByTestId("mobile-plugin-action").textContent).toBe("mobile");
  });

  it("reacts when the first or last chat-top-bar registration changes", () => {
    function Presence() {
      return <span data-testid="slot-presence">{String(useHasTaskTopBarPluginActions())}</span>;
    }

    render(<Presence />);
    expect(screen.getByTestId("slot-presence").textContent).toBe("false");

    act(() => {
      pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, () => null);
    });
    expect(screen.getByTestId("slot-presence").textContent).toBe("true");

    act(() => pluginRegistry.unregisterPlugin("plugin-a"));
    expect(screen.getByTestId("slot-presence").textContent).toBe("false");
  });
});
