import type { PropsWithChildren } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { VcsDialogsProvider } from "@/components/vcs/vcs-dialogs";
import { ReviewDialog } from "@/components/review/review-dialog";
import { defaultState } from "@/lib/state/default-state";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import { useGlobalViewMode, type ViewMode } from "./use-global-view-mode";

// Only the deferred transport is replaced; UI and preference coordination are real.
class DeferredSocket {
  static readonly OPEN = 1;
  static current: DeferredSocket;
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  requests: { id: string }[] = [];
  constructor() {
    DeferredSocket.current = this;
  }
  send(data: string) {
    this.requests.push(JSON.parse(data) as { id: string });
  }
  close() {
    this.readyState = 3;
  }
  settle() {
    for (const request of this.requests.splice(0)) {
      this.onmessage?.({
        data: JSON.stringify({
          id: request.id,
          type: "response",
          payload: { reviews: [], success: true },
        }),
      });
    }
  }
}

const storageKey = "diff-view-mode";
const sharedModeTestId = "shared-mode";
const opposite = { unified: "split", split: "unified" } as const;
const actionName = (mode: ViewMode) => `Switch to ${opposite[mode]} view`;
const iconFor = (mode: ViewMode) =>
  mode === "split" ? ".tabler-icon-layout-rows" : ".tabler-icon-layout-columns";
let client: WebSocketClient;
let previousClient: WebSocketClient | null;
let sequence = 0;

function Providers({ children }: PropsWithChildren) {
  return (
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, reviewAutoMarkOnScroll: false },
      }}
    >
      <ToastProvider>
        <TooltipProvider>{children}</TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
}

function ModeReader() {
  const [mode] = useGlobalViewMode();
  return <output data-testid={sharedModeTestId}>{mode}</output>;
}

function mountDialog() {
  const id = `diff-mode-${++sequence}`;
  const rendered = render(
    <VcsDialogsProvider sessionId={id}>
      <ModeReader />
      <ReviewDialog
        open
        onOpenChange={vi.fn()}
        sessionId={id}
        onSendComments={vi.fn()}
        cumulativeDiff={null}
        gitStatusFiles={{
          "example.ts": {
            path: "example.ts",
            diff: "@@ -1 +1 @@\n-before\n+after",
            status: "modified",
            additions: 1,
            deletions: 1,
            staged: false,
          },
        }}
        useRepositoryKeys={false}
      />
    </VcsDialogsProvider>,
    { wrapper: Providers },
  );
  const topbar = Array.from(screen.getByRole("dialog").querySelectorAll("button")).find(
    (button) =>
      button.querySelector(`${iconFor("split")}, ${iconFor("unified")}`) &&
      !button.hasAttribute("aria-label"),
  );
  expect(topbar).toBeDefined();
  return { ...rendered, topbar: topbar! };
}

beforeEach(() => {
  localStorage.removeItem(storageKey);
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "queueMicrotask"] });
  previousClient = getWebSocketClient();
  vi.stubGlobal("WebSocket", DeferredSocket);
  client = new WebSocketClient("ws://diff-mode-test", undefined, { enabled: false });
  client.connect();
  DeferredSocket.current.readyState = DeferredSocket.OPEN;
  DeferredSocket.current.onopen?.();
  setWebSocketClient(client);
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    DeferredSocket.current.settle();
    vi.runAllTicks();
  });
  client.disconnect();
  setWebSocketClient(previousClient);
  localStorage.removeItem(storageKey);
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe.each<ViewMode>(["unified", "split"])("Review starting in %s", (initial) => {
  // @covers AC-UI-DIFF-PRESENTATION-001.1
  it("updates the mounted topbar after the real file-header mode action", () => {
    localStorage.setItem(storageKey, initial);
    const { topbar } = mountDialog();
    const wrap = screen
      .getByRole("dialog")
      .querySelector("button:not([aria-label]):has(.tabler-icon-text-wrap)")!;
    expect(wrap.classList.contains("bg-muted")).toBe(true);
    fireEvent.click(wrap);
    expect(wrap.classList.contains("bg-muted")).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: actionName(initial) }));
    expect(wrap.classList.contains("bg-muted")).toBe(false);
    expect(localStorage.getItem(storageKey)).toBe(opposite[initial]);
    expect(screen.getByTestId(sharedModeTestId).textContent).toBe(opposite[initial]);
    expect(screen.getByRole("button", { name: actionName(opposite[initial]) })).toBeTruthy();
    expect(topbar.querySelector(iconFor(opposite[initial]))).not.toBeNull();
  });

  it("reverses the new mode on the first subsequent topbar click", () => {
    localStorage.setItem(storageKey, initial);
    const { topbar } = mountDialog();
    fireEvent.click(screen.getByRole("button", { name: actionName(initial) }));
    fireEvent.click(topbar);
    expect(localStorage.getItem(storageKey)).toBe(initial);
    expect(screen.getByTestId(sharedModeTestId).textContent).toBe(initial);
    expect(screen.getByRole("button", { name: actionName(initial) })).toBeTruthy();
  });

  // @covers AC-UI-DIFF-PRESENTATION-001.2, AC-UI-DIFF-PRESENTATION-001.3
  it("retains raw stored mode and ordinary topbar toggling across remount", () => {
    localStorage.setItem(storageKey, initial);
    const first = mountDialog();
    expect(first.topbar.querySelector(iconFor(initial))).not.toBeNull();
    fireEvent.click(first.topbar);
    expect(localStorage.getItem(storageKey)).toBe(opposite[initial]);
    first.unmount();
    const second = mountDialog();
    expect(second.topbar.querySelector(iconFor(opposite[initial]))).not.toBeNull();
    fireEvent.click(second.topbar);
    expect(localStorage.getItem(storageKey)).toBe(initial);
  });

  it("follows a browser storage event before the next topbar click", () => {
    localStorage.setItem(storageKey, initial);
    const { topbar } = mountDialog();
    act(() => {
      localStorage.setItem(storageKey, opposite[initial]);
      window.dispatchEvent(new StorageEvent("storage", { key: storageKey }));
    });
    expect(screen.getByTestId(sharedModeTestId).textContent).toBe(opposite[initial]);
    expect(topbar.querySelector(iconFor(opposite[initial]))).not.toBeNull();
    fireEvent.click(topbar);
    expect(localStorage.getItem(storageKey)).toBe(initial);
  });
});

// @covers AC-UI-DIFF-PRESENTATION-001.2
it.each([null, "invalid"])("defaults to unified with stored value %s", (stored) => {
  if (stored !== null) localStorage.setItem(storageKey, stored);
  const { topbar } = mountDialog();
  expect(topbar.querySelector(iconFor("unified"))).not.toBeNull();
  expect(screen.getByTestId(sharedModeTestId).textContent).toBe("unified");
  fireEvent.click(topbar);
  expect(localStorage.getItem(storageKey)).toBe("split");
});
