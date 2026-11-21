import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import type { DockviewApi } from "dockview-react";
import { expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { createAppStore } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  clearHiddenSessionPanel,
  hideSessionPanel,
} from "@/components/task/dockview-hidden-session-panels";
import { useUnreadTaskDotDisplay } from "./use-unread-task-dot-display";

const viewport = vi.hoisted(() => ({ isMobile: false }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => viewport,
}));
const SESSION_ID = "hidden-selected-session";

// @covers AC-TASKS-TASK-UNREAD-DOT-001.12
it.each([false, true])("keeps desktop hiding separate from phone display (phone: %s)", (phone) => {
  viewport.isMobile = phone;
  const listeners = new Set<() => void>();
  let visible = true;
  const api = {
    getPanel: () => ({
      api: {
        get isVisible() {
          return visible;
        },
        onDidVisibilityChange: (notify: () => void) => {
          listeners.add(notify);
          return { dispose: () => listeners.delete(notify) };
        },
      },
    }),
    removePanel: () => undefined,
    onDidLayoutChange: (notify: () => void) => {
      listeners.add(notify);
      return {
        dispose: () => {
          listeners.delete(notify);
        },
      };
    },
  } as unknown as DockviewApi;
  const original = useDockviewStore.getState();
  useDockviewStore.setState({ api, currentLayoutEnvId: null });
  const tasks = {
    ...createAppStore().getState().tasks,
    activeTaskId: "t1",
    activeSessionId: SESSION_ID,
  };
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider initialState={{ tasks }}>{children}</StateProvider>
  );
  const { result, rerender, unmount } = renderHook(() => useUnreadTaskDotDisplay(), { wrapper });
  try {
    expect(result.current.displayedSessionId).toBe(SESSION_ID);
    if (!phone) {
      act(() => {
        visible = false;
        for (const notify of listeners) notify();
      });
      expect(result.current.displayedSessionId).toBeNull();
      act(() => {
        visible = true;
        for (const notify of listeners) notify();
      });
      expect(result.current.displayedSessionId).toBe(SESSION_ID);
    }
    act(() => {
      hideSessionPanel(api, SESSION_ID, "t1");
    });
    expect(result.current.displayedSessionId).toBe(phone ? SESSION_ID : null);
    viewport.isMobile = !phone;
    rerender();
    expect(result.current.displayedSessionId).toBe(phone ? null : SESSION_ID);
    viewport.isMobile = phone;
    rerender();
    act(() => {
      useDockviewStore.setState({ currentLayoutEnvId: "different-layout" });
    });
    expect(result.current.displayedSessionId).toBe(SESSION_ID);
    act(() => {
      useDockviewStore.setState({ currentLayoutEnvId: null });
    });
    expect(result.current.displayedSessionId).toBe(phone ? SESSION_ID : null);
    act(() => {
      clearHiddenSessionPanel(api, SESSION_ID);
      for (const notify of listeners) notify();
    });
    expect(result.current.displayedSessionId).toBe(SESSION_ID);
  } finally {
    unmount();
    expect(listeners.size).toBe(0);
    useDockviewStore.setState(original);
  }
});

it.each(["chat", "plan", "changes", "files", "terminal"] as const)(
  "excludes a phone conversation only on its Chat surface, not %s",
  (panel) => {
    viewport.isMobile = true;
    const base = createAppStore().getState();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <StateProvider
        initialState={{
          tasks: { ...base.tasks, activeTaskId: "t1", activeSessionId: SESSION_ID },
          mobileSession: { ...base.mobileSession, activePanelBySessionId: { [SESSION_ID]: panel } },
        }}
      >
        {children}
      </StateProvider>
    );
    const { result, unmount } = renderHook(
      () => ({
        display: useUnreadTaskDotDisplay(),
        store: useAppStoreApi(),
      }),
      { wrapper },
    );
    try {
      expect(result.current.display.displayedSessionId).toBe(panel === "chat" ? SESSION_ID : null);
      act(() => result.current.store.getState().setMobileSessionPanel(SESSION_ID, "chat"));
      expect(result.current.display.displayedSessionId).toBe(SESSION_ID);
    } finally {
      unmount();
      viewport.isMobile = false;
    }
  },
);
