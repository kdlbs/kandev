import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const providerState = vi.hoisted(() => ({
  quickChat: {
    isOpen: true,
    sessions: [{ sessionId: "chat-1", workspaceId: "ws-1" }],
    terminalTabs: [],
    activeKind: "conversation" as const,
    activeSessionId: "chat-1",
    activeTerminalTabId: null,
  },
  workspaces: { activeId: "ws-1" },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof providerState) => unknown) => selector(providerState),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));
vi.mock("@/hooks/use-quick-chat-resync", () => ({ useQuickChatResync: vi.fn() }));
vi.mock("./quick-chat-modal", () => ({ QuickChatModal: () => null }));

import { captureQuickChatLauncherFocus } from "./quick-chat-focus";
import { QuickChatProvider } from "./quick-chat-provider";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("QuickChatProvider focus contract", () => {
  it("restores focus to the launcher after the shared surface closes", () => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 0;
    });
    const { rerender } = render(
      <QuickChatProvider>
        <button type="button">launch</button>
      </QuickChatProvider>,
    );
    const launcher = document.querySelector("button") as HTMLButtonElement;
    launcher.focus();
    captureQuickChatLauncherFocus();

    providerState.quickChat.isOpen = false;
    rerender(
      <QuickChatProvider>
        <button type="button">launch</button>
      </QuickChatProvider>,
    );

    expect(document.activeElement).toBe(launcher);
  });
});
