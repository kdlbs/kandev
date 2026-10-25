import { act, cleanup, render, waitFor } from "@testing-library/react";
import type { TiptapEditorHTMLElement } from "@tiptap/core";
import type { StoreApi } from "zustand";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AppState } from "@/lib/state/store";
import type { TaskPlan } from "@/lib/types/http";
import { registerTaskPlansHandlers } from "@/lib/ws/handlers/task-plans";

const fixture = vi.hoisted(() => ({
  request: vi.fn(),
  breakpoint: {
    breakpoint: "desktop",
    isMobile: false,
    isTablet: false,
    isDesktop: true,
    isCompactDesktop: false,
    isFullDesktop: true,
    isFinePointer: true,
    usesDesktopWorkbench: true,
  } as ResponsiveBreakpoint,
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => fixture }));
vi.mock("@/hooks/use-responsive-breakpoint", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/use-responsive-breakpoint")>()),
  useResponsiveBreakpoint: () => fixture.breakpoint,
}));
vi.mock("@/components/theme/app-theme", () => ({ useTheme: () => ({ resolvedTheme: "light" }) }));
vi.mock("@/components/shared/mermaid-error-toast", () => ({
  useMermaidErrorToast: () => undefined,
}));
vi.mock("@/hooks/domains/comments/use-plan-comment-migration", () => ({
  usePlanCommentMigration: () => ({ needsAttention: false }),
}));
// Only unrelated formatting chrome is replaced. Panel, adapter, editor, hooks,
// API, notifications and store all run their production implementations.
vi.mock("@/components/editors/tiptap/plan-bubble-menu", () => ({
  PLAN_FORMATTING_TOOLBAR_HEIGHT_PX: 48,
  PlanBubbleMenu: () => null,
}));
vi.mock("@/components/editors/tiptap/plan-drag-handle", () => ({ PlanDragHandle: () => null }));
vi.mock("@/components/editors/tiptap/plan-slash-menu", () => ({ PlanSlashMenu: () => null }));

import { TaskPlanPanel } from "./task-plan-panel";
const TASK = "snapshot-panel-task";
const old: TaskPlan = {
  id: "snapshot-panel-plan",
  task_id: TASK,
  title: "Plan",
  content: "Old outline",
  created_by: "agent",
  created_at: "2026-10-20T00:00:00Z",
  updated_at: "2026-10-20T00:00:00Z",
};
let store: StoreApi<AppState>;
function CaptureStore() {
  store = useAppStoreApi();
  return null;
}
function panel(visible: boolean) {
  return (
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <CaptureStore />
          <TaskPlanPanel taskId={TASK} visible={visible} />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
}
function editor(container: HTMLElement) {
  const element = container.querySelector<TiptapEditorHTMLElement>(".ProseMirror");
  if (!element?.editor) throw new Error("Real TipTap editor has not mounted");
  return element.editor;
}
afterEach(() => {
  cleanup();
  fixture.request.mockReset();
  vi.restoreAllMocks();
  localStorage.clear();
});

// @covers AC-TASKS-DOCUMENTS-003.4, AC-TASKS-DOCUMENTS-003.8
it.each([
  [false, "updated"],
  [false, "deleted"],
  [true, "updated"],
  [true, "deleted"],
] as const)(
  "keeps live %s/%s content in the real Plan editor after the old snapshot settles",
  async (phone, event) => {
    Object.assign(fixture.breakpoint, {
      breakpoint: phone ? "mobile" : "desktop",
      isMobile: phone,
      isDesktop: !phone,
      isFullDesktop: !phone,
      isFinePointer: !phone,
      usesDesktopWorkbench: !phone,
    });
    let resolve!: (value: TaskPlan) => void;
    const reply = new Promise<TaskPlan>((yes) => {
      resolve = yes;
    });
    fixture.request.mockImplementation((action: string) => {
      if (action === "task.plan.get") return reply;
      if (action === "task.plan.revisions.list") return Promise.resolve({ revisions: [] });
      if (action === "task.plan.comments.list")
        return Promise.resolve({ task_id: TASK, plan_id: old.id, revision: 0, comments: [] });
      throw new Error(`Unexpected transport request: ${action}`);
    });
    const view = render(panel(false));
    act(() => store.getState().setTaskPlan(TASK, old));
    await waitFor(() => expect(editor(view.container).isInitialized).toBe(true));
    expect(editor(view.container).getText()).toBe(old.content);
    act(() => store.getState().setConnectionStatus("connected"));
    view.rerender(panel(true));
    expect(fixture.request).toHaveBeenCalledWith("task.plan.get", { task_id: TASK });
    act(() => {
      const handlers = registerTaskPlansHandlers(store);
      if (event === "updated")
        handlers["task.plan.updated"]!({
          type: "notification",
          action: "task.plan.updated",
          payload: { ...old, content: "Live outline", updated_at: "2026-10-21T00:00:00Z" },
        });
      else
        handlers["task.plan.deleted"]!({
          type: "notification",
          action: "task.plan.deleted",
          payload: old,
        });
    });
    const expected = event === "updated" ? "Live outline" : "";
    await waitFor(() => expect(editor(view.container).getText()).toBe(expected));
    await act(async () => {
      resolve(old);
      await reply;
    });
    expect(editor(view.container).getText()).toBe(expected);
    expect(store.getState().taskPlans.byTaskId[TASK]?.content ?? "").toBe(expected);
    expect(store.getState().taskPlans.loadedByTaskId[TASK]).toBe(true);
  },
);
