import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  ChatInputPluginActions,
  type ChatInputActionsSlotProps,
} from "./chat-input-plugin-actions";

const SLOT = "chat-input-actions";

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

describe("ChatInputPluginActions", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin("plugin-a");
  });

  it("renders nothing when no plugin registered a chat-input-actions component", () => {
    const { container } = render(
      <ChatInputPluginActions sessionId="s1" taskId="t1" taskTitle="Demo" />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("forwards task context, the active session, and all session ids", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatInputActionsSlotProps;
      return (
        <div data-testid="plugin-icon">
          {`${ctx.taskId}|${ctx.activeSessionId}|${ctx.sessionIds.join(",")}`}
        </div>
      );
    });

    render(<ChatInputPluginActions sessionId="s2" taskId="t1" taskTitle="Demo" />);

    expect(screen.getByTestId("plugin-icon").textContent).toBe("t1|s2|s1,s2");
  });

  it("includes the active session id even when the store list omits it", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatInputActionsSlotProps;
      return <div data-testid="plugin-icon">{ctx.sessionIds.join(",")}</div>;
    });

    // taskId with no store entry -> only the active session propagates.
    render(<ChatInputPluginActions sessionId="s9" taskId="t-unknown" taskTitle="Demo" />);

    expect(screen.getByTestId("plugin-icon").textContent).toBe("s9");
  });

  it("propagates only the active session when taskId is null", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatInputActionsSlotProps;
      return (
        <div data-testid="plugin-icon">{`${ctx.taskId}|${ctx.activeSessionId}|${ctx.sessionIds.join(",")}`}</div>
      );
    });

    render(<ChatInputPluginActions sessionId="s9" taskId={null} />);

    expect(screen.getByTestId("plugin-icon").textContent).toBe("null|s9|s9");
  });
});
