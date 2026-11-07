import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn(), search: "sessionId=s1" }));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => router,
  useSearchParams: () => new URLSearchParams(router.search),
}));
const breakpoint = vi.hoisted(() => ({ isMobile: false, usesDesktopWorkbench: true }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({ useResponsiveBreakpoint: () => breakpoint }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (s: unknown) => unknown) =>
    sel({ tasks: { activeTaskId: "current" }, workspaces: { activeId: "ws" } }),
}));
const storedView = vi.hoisted(() => ({ value: "" }));
vi.mock("@/lib/task-listing/view-preference", () => ({
  setStoredTaskListingView: (view: string) => {
    storedView.value = view;
  },
}));

import { takePendingTaskGraphFocusTarget, useOpenTaskGraph } from "./use-open-task-graph";

beforeEach(() => {
  breakpoint.isMobile = false;
  breakpoint.usesDesktopWorkbench = true;
  takePendingTaskGraphFocusTarget();
  storedView.value = "";
  router.push.mockReset();
  router.replace.mockReset();
});

describe("useOpenTaskGraph", () => {
  it("navigates to another task with the graph requested", () => {
    renderHook(() => useOpenTaskGraph()).result.current.openTab("other");
    expect(router.push).toHaveBeenCalledWith("/t/other?panel=task-graph");
  });

  it("requests the graph on the current task without losing its query", () => {
    renderHook(() => useOpenTaskGraph()).result.current.openTab("current");
    expect(router.replace).toHaveBeenCalledWith("/t/current?sessionId=s1&panel=task-graph");
    expect(router.push).not.toHaveBeenCalled();
  });

  it("shows a task in the board Graph view on desktop", () => {
    renderHook(() => useOpenTaskGraph()).result.current.showInGraph("t9", "wf1");
    expect(storedView.value).toBe("graph");
    expect(router.push).toHaveBeenCalledWith(
      "/?home=overview&workspaceId=ws&workflowId=wf1&graphFocus=t9",
    );
  });

  it("falls back to the task sheet on phones", () => {
    breakpoint.isMobile = true;
    breakpoint.usesDesktopWorkbench = false;
    renderHook(() => useOpenTaskGraph()).result.current.showInGraph("other");
    expect(storedView.value).toBe("");
    expect(router.push).toHaveBeenCalledWith("/t/other?panel=task-graph");
  });

  it("captures the active opener for phone sheet focus return", () => {
    breakpoint.isMobile = true;
    breakpoint.usesDesktopWorkbench = false;
    const opener = document.createElement("button");
    document.body.append(opener);
    opener.focus();

    renderHook(() => useOpenTaskGraph()).result.current.openTab("other");

    expect(takePendingTaskGraphFocusTarget()).toBe(opener);
    expect(takePendingTaskGraphFocusTarget()).toBeNull();
    opener.remove();
  });

  it("captures the opener for tablet layouts that also use the task sheet", () => {
    breakpoint.isMobile = false;
    breakpoint.usesDesktopWorkbench = false;
    const opener = document.createElement("button");
    document.body.append(opener);
    opener.focus();

    renderHook(() => useOpenTaskGraph()).result.current.openTab("other");

    expect(takePendingTaskGraphFocusTarget()).toBe(opener);
    opener.remove();
  });
});
