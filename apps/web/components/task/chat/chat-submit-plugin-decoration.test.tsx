import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  ChatSubmitPluginDecoration,
  type ChatSubmitDecorationSlotProps,
} from "./chat-submit-plugin-decoration";

const SLOT = "chat-submit-decoration";

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

function renderDecoration(
  overrides: Partial<React.ComponentProps<typeof ChatSubmitPluginDecoration>> = {},
) {
  return render(
    <ChatSubmitPluginDecoration
      sessionId="s2"
      taskId="t1"
      taskTitle="Demo"
      presentation="desktop"
      isSending={false}
      isAgentBusy={false}
      isDisabled={false}
      planModeEnabled={false}
      {...overrides}
    />,
  );
}

describe("ChatSubmitPluginDecoration", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin("plugin-a");
  });

  it("renders nothing when no plugin registered a chat-submit-decoration component", () => {
    const { container } = renderDecoration();
    expect(container.innerHTML).toBe("");
  });

  it("forwards task context, the active session, and all session ids", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatSubmitDecorationSlotProps;
      return (
        <div data-testid="ring">
          {`${ctx.taskId}|${ctx.taskTitle}|${ctx.activeSessionId}|${ctx.sessionIds.join(",")}`}
        </div>
      );
    });

    renderDecoration();

    expect(screen.getByTestId("ring").textContent).toBe("t1|Demo|s2|s1,s2");
  });

  it("includes the active session id even when the store list omits it", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatSubmitDecorationSlotProps;
      return <div data-testid="ring">{ctx.sessionIds.join(",")}</div>;
    });

    renderDecoration({ sessionId: "s9" });

    expect(screen.getByTestId("ring").textContent).toBe("s9,s1,s2");
  });

  it("forwards the send button's live state so a decoration can react to it", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as ChatSubmitDecorationSlotProps;
      return (
        <div data-testid="ring">
          {`${ctx.presentation}|${ctx.isSending}|${ctx.isAgentBusy}|${ctx.disabled}|${ctx.planModeEnabled}`}
        </div>
      );
    });

    renderDecoration({
      presentation: "mobile",
      isSending: true,
      isAgentBusy: true,
      isDisabled: true,
      planModeEnabled: true,
    });

    expect(screen.getByTestId("ring").textContent).toBe("mobile|true|true|true|true");
  });

  it("layers the decoration over the button box without swallowing its clicks", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, () => <div data-testid="ring" />);

    renderDecoration();

    const layer = screen.getByTestId("chat-submit-decoration-layer");
    expect(layer.className).toContain("absolute");
    expect(layer.className).toContain("inset-0");
    // A decoration must never block the send button underneath it. Interactive
    // behavior should observe the host button or use a separate toolbar action.
    expect(layer.className).toContain("pointer-events-none");
  });
});
