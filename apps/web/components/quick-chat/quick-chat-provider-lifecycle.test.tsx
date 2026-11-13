import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";
import type { QuickTerminalTab } from "@/lib/state/slices/ui/types";

const lifecycleProbe = vi.hoisted(() => ({
  current: null as unknown,
  mounts: 0,
  unmounts: 0,
}));
const updateTerminalRequest = vi.hoisted(() => vi.fn());
const listTerminalRequests = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));
vi.mock("@/hooks/use-quick-chat-resync", () => ({ useQuickChatResync: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api/domains/quick-terminal-api", () => ({
  listQuickTerminalTabs: (...args: unknown[]) => listTerminalRequests(...args),
  updateQuickTerminalTab: (...args: unknown[]) => updateTerminalRequest(...args),
  toQuickTerminalTab: (tab: unknown) => tab,
}));
vi.mock("./quick-chat-modal", async () => {
  const React = await import("react");
  const { useAppStore } = await import("@/components/state-provider");
  const { useQuickTerminalLifecycle } =
    await import("@/hooks/domains/session/use-quick-terminal-lifecycle");

  return {
    QuickChatModal: ({ workspaceId }: { workspaceId: string }) => {
      const terminalTabs = useAppStore((state) => state.quickChat.terminalTabs);
      const updateQuickTerminal = useAppStore((state) => state.updateQuickTerminal);
      const lifecycle = useQuickTerminalLifecycle(workspaceId, {
        terminalTabs,
        updateQuickTerminal,
      });
      lifecycleProbe.current = lifecycle;
      React.useEffect(() => {
        lifecycleProbe.mounts += 1;
        return () => {
          lifecycleProbe.unmounts += 1;
        };
      }, []);
      return React.createElement("div", { "data-testid": "quick-chat-modal-probe" });
    },
  };
});

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { QuickChatProvider } from "./quick-chat-provider";

const WORKSPACE_ID = "workspace-1";
const TAB_ID = "terminal-1";
const CHAT_ID = "chat-1";
const OLD_SESSION_ID = "old-session";
const NEW_SESSION_ID = "new-session";

type LifecycleProbe = {
  handleTerminalStateChange: (
    tabId: string,
    state: {
      status: "exited" | "running";
      sessionId: string | null;
      exitCode: number | null;
      error: null;
    },
    generation: number,
  ) => void;
  handleRecoverTerminal: (tabId: string) => Promise<void>;
  terminalRecoveryByTab: Record<string, { generation: number; phase?: string }>;
};

function currentLifecycle() {
  return lifecycleProbe.current as LifecycleProbe;
}

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((onResolve) => {
    resolve = onResolve;
  });
  return { promise, resolve };
}

function StoreCapture({
  capture,
}: {
  capture: (store: ReturnType<typeof useAppStoreApi>) => void;
}) {
  capture(useAppStoreApi());
  return null;
}

let store: ReturnType<typeof useAppStoreApi> | null = null;
let descriptor: QuickTerminalTab;

function setQuickChat(patch: Partial<AppState["quickChat"]>) {
  if (!store) throw new Error("store is not mounted");
  store.setState((state) => ({ quickChat: { ...state.quickChat, ...patch } }));
}

beforeEach(() => {
  lifecycleProbe.current = null;
  lifecycleProbe.mounts = 0;
  lifecycleProbe.unmounts = 0;
  store = null;
  descriptor = {
    tabId: TAB_ID,
    workspaceId: WORKSPACE_ID,
    sessionId: OLD_SESSION_ID,
    sequence: 1,
    status: "running",
  };
  updateTerminalRequest.mockReset();
  updateTerminalRequest.mockImplementation(
    async (_tabId: string, update: Partial<QuickTerminalTab>) => {
      descriptor = { ...descriptor, ...update };
      return descriptor;
    },
  );
  listTerminalRequests.mockReset();
  listTerminalRequests.mockImplementation(async () => ({ tabs: [descriptor] }));
});

afterEach(() => {
  cleanup();
});

describe("QuickChatProvider terminal lifecycle ownership", () => {
  it("drains an exited PATCH across conversation dismissal before recovering the same tab", async () => {
    const pendingPatch = deferred();
    updateTerminalRequest.mockImplementationOnce(
      async (_tabId: string, update: Partial<QuickTerminalTab>) => {
        await pendingPatch.promise;
        descriptor = { ...descriptor, ...update };
        return descriptor;
      },
    );

    render(
      <StateProvider>
        <QuickChatProvider>
          <StoreCapture capture={(nextStore) => (store = nextStore)} />
        </QuickChatProvider>
      </StateProvider>,
    );
    if (!store) throw new Error("store is not mounted");

    act(() => {
      store!.setState((state) => ({
        workspaces: { ...state.workspaces, activeId: WORKSPACE_ID },
        quickChat: {
          ...state.quickChat,
          isOpen: true,
          sessions: [{ sessionId: CHAT_ID, workspaceId: WORKSPACE_ID, kind: "chat" }],
          terminalTabs: [descriptor],
          activeKind: "terminal",
          activeSessionId: null,
          activeTerminalTabId: TAB_ID,
        },
      }));
    });
    expect(await screen.findByTestId("quick-chat-modal-probe")).toBeTruthy();

    act(() => {
      currentLifecycle().handleTerminalStateChange(
        TAB_ID,
        { status: "exited", sessionId: OLD_SESSION_ID, exitCode: 0, error: null },
        0,
      );
    });
    await waitFor(() => expect(updateTerminalRequest).toHaveBeenCalledTimes(1));

    act(() => {
      setQuickChat({
        activeKind: "conversation",
        activeSessionId: CHAT_ID,
        activeTerminalTabId: null,
      });
    });
    act(() => setQuickChat({ isOpen: false }));
    const unmountsAfterDismissal = lifecycleProbe.unmounts;
    expect(unmountsAfterDismissal).toBe(0);

    act(() => {
      setQuickChat({
        isOpen: true,
        activeKind: "terminal",
        activeSessionId: null,
        activeTerminalTabId: TAB_ID,
      });
    });
    expect(await screen.findByTestId("quick-chat-modal-probe")).toBeTruthy();
    expect(lifecycleProbe.mounts).toBe(1);

    let recovery!: Promise<void>;
    act(() => {
      recovery = currentLifecycle().handleRecoverTerminal(TAB_ID);
    });
    await act(async () => {
      await Promise.resolve();
    });
    const descriptorReadsBeforePatchSettled = listTerminalRequests.mock.calls.length;

    await act(async () => {
      pendingPatch.resolve();
      await recovery;
    });
    expect(descriptorReadsBeforePatchSettled).toBe(0);
    expect(lifecycleProbe.unmounts).toBe(unmountsAfterDismissal);
    expect(currentLifecycle().terminalRecoveryByTab[TAB_ID]).toEqual({
      generation: 1,
      phase: "starting",
    });

    act(() => {
      currentLifecycle().handleTerminalStateChange(
        TAB_ID,
        { status: "running", sessionId: NEW_SESSION_ID, exitCode: null, error: null },
        1,
      );
    });
    await waitFor(() => expect(descriptor.sessionId).toBe(NEW_SESSION_ID));
    expect(descriptor.status).toBe("running");
    expect(
      store!.getState().quickChat.terminalTabs.find((tab) => tab.tabId === TAB_ID),
    ).toMatchObject({ sessionId: NEW_SESSION_ID, status: "running" });
  });
});
