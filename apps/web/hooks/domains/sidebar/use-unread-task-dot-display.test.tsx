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
import { useTabletSessionTab } from "@/hooks/domains/session/use-tablet-session-tab";
import { requestTaskRouteFocus, hasTaskRouteFocus } from "@/lib/state/task-navigation-focus";

const viewport = vi.hoisted(() => ({
  isMobile: false,
  isTablet: false,
  get usesDesktopWorkbench() {
    return !this.isMobile && !this.isTablet;
  },
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => viewport,
}));
const SESSION_ID = "hidden-selected-session";
const FILE_TAB = "file:README.md";

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

it.each(["chat", "changes", FILE_TAB, "pr"])(
  "uses the coarse tablet's actual tab for unread exclusion: %s",
  (tab) => {
    viewport.isTablet = true;
    const base = createAppStore().getState();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <StateProvider
        initialState={{
          tasks: { ...base.tasks, activeTaskId: "t1", activeSessionId: SESSION_ID },
          taskPRs: {
            ...base.taskPRs,
            byTaskId: { t1: [{ id: 1, number: 1, title: "PR", state: "open" } as never] },
          },
          tabletSession: { activeTabBySessionId: { [SESSION_ID]: tab } },
          mobileSession: {
            ...base.mobileSession,
            activePanelBySessionId: { [SESSION_ID]: "plan" },
          },
        }}
      >
        {children}
      </StateProvider>
    );
    const hook = renderHook(
      () => ({
        display: useUnreadTaskDotDisplay(),
        tab: useTabletSessionTab(SESSION_ID),
        store: useAppStoreApi(),
      }),
      { wrapper },
    );
    try {
      expect(hook.result.current.display.displayedSessionId).toBe(
        tab === "chat" ? SESSION_ID : null,
      );
      act(() => {
        const store = hook.result.current.store;
        store.getState().setActiveSession("t1", SESSION_ID);
        requestTaskRouteFocus(store, "t1", SESSION_ID);
      });
      expect(hook.result.current.tab[0]).toBe("chat");
      expect(hook.result.current.display.displayedSessionId).toBe(SESSION_ID);
      expect(hasTaskRouteFocus(hook.result.current.store, "t1", SESSION_ID)).toBe(false);
      act(() => hook.result.current.tab[1](FILE_TAB));
      hook.rerender();
      expect(hook.result.current.tab[0]).toBe(FILE_TAB);
      expect(hook.result.current.display.displayedSessionId).toBeNull();
    } finally {
      hook.unmount();
      viewport.isTablet = false;
    }
  },
);

it("delivers and acknowledges committed tablet workflow focus without replaying it", () => {
  viewport.isTablet = true;
  const base = createAppStore().getState();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider
      initialState={{
        tasks: { ...base.tasks, activeTaskId: "t1", activeSessionId: SESSION_ID },
        tabletSession: { activeTabBySessionId: { [SESSION_ID]: FILE_TAB } },
      }}
    >
      {children}
    </StateProvider>
  );
  const hook = renderHook(
    () => ({
      tab: useTabletSessionTab(SESSION_ID),
      store: useAppStoreApi(),
    }),
    { wrapper },
  );
  try {
    act(() => {
      const state = hook.result.current.store.getState();
      state.setTaskSession({ id: SESSION_ID, task_id: "t1", state: "WAITING_FOR_INPUT" } as never);
      const requestId = state.beginWorkflowSessionFocus({
        taskId: "t1",
        workflowId: "wf1",
        destinationStepId: "review",
        presentationToken: 1,
        navigationRevision: state.taskRemoval.navigationRevision,
      })!;
      state.bindWorkflowSessionFocus({
        requestId,
        presentationToken: 1,
        entryIdentity: "entry:tablet",
      });
      state.reconcileWorkflowSessionFocus("t1", {
        workflowStepId: "review",
        updatedAt: "2026-11-21T18:00:00Z",
        entryIdentity: "entry:tablet",
        metadata: {
          workflow_session_route: {
            operation_id: "tablet",
            destination_step_id: "review",
            entry_identity: "entry:tablet",
            destination_session_id: SESSION_ID,
            phase: "committed",
          },
        },
      });
    });
    expect(hook.result.current.tab[0]).toBe("chat");
    expect(hook.result.current.store.getState().workflowSessionFocus.request).toBeNull();
    act(() => hook.result.current.tab[1](FILE_TAB));
    hook.rerender();
    expect(hook.result.current.tab[0]).toBe(FILE_TAB);
  } finally {
    hook.unmount();
    viewport.isTablet = false;
  }
});
