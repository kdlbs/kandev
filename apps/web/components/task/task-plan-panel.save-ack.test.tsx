import { act, cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import type { Editor, TiptapEditorHTMLElement } from "@tiptap/core";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { ResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AppState } from "@/lib/state/store";
import type { TaskPlan } from "@/lib/types/http";
import { TooltipProvider } from "@kandev/ui/tooltip";

const fixture = vi.hoisted(() => ({
  update: vi.fn<(taskId: string, content: string, title?: string) => Promise<TaskPlan>>(),
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

vi.mock("@/lib/api/domains/plan-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/plan-api")>()),
  updateTaskPlan: fixture.update,
}));
vi.mock("@/hooks/use-responsive-breakpoint", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/use-responsive-breakpoint")>()),
  useResponsiveBreakpoint: () => fixture.breakpoint,
}));
vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({ resolvedTheme: "light" }),
}));
vi.mock("@/components/shared/mermaid-error-toast", () => ({
  useMermaidErrorToast: () => undefined,
}));
vi.mock("@/hooks/domains/comments/use-plan-comment-migration", () => ({
  usePlanCommentMigration: () => ({ needsAttention: false }),
}));
// Formatting chrome is unrelated to save acknowledgement. The panel,
// dynamic adapter, TipTap editor, draft/save hooks, and provider remain real.
vi.mock("@/components/editors/tiptap/plan-bubble-menu", () => ({
  PLAN_FORMATTING_TOOLBAR_HEIGHT_PX: 48,
  PlanBubbleMenu: () => null,
}));
vi.mock("@/components/editors/tiptap/plan-drag-handle", () => ({ PlanDragHandle: () => null }));
vi.mock("@/components/editors/tiptap/plan-slash-menu", () => ({ PlanSlashMenu: () => null }));

import { TaskPlanPanel } from "./task-plan-panel";

const TASK = "panel-save-ack-task";
const BASELINE = "Original plan outline";
const SUBMITTED = "Updated plan outline";
const NEWER = "Updated plan outline plus another sentence";
const AUTO_SAVE_DELAY = 1500;
let store: StoreApi<AppState>;

function planFor(content: string): TaskPlan {
  return {
    id: "panel-save-ack-plan",
    task_id: TASK,
    title: "Plan",
    content,
    created_by: "user",
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:01Z",
  };
}

function CaptureStore() {
  store = useAppStoreApi();
  return null;
}

function deferredSave() {
  let resolve!: (plan: TaskPlan) => void;
  const promise = new Promise<TaskPlan>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

function mountedEditor(container: HTMLElement) {
  const element = container.querySelector<TiptapEditorHTMLElement>(".ProseMirror");
  if (!element?.editor) throw new Error("The real TipTap editor has not mounted");
  return { element, editor: element.editor };
}

async function mountPanel() {
  const view = render(
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <CaptureStore />
          <TaskPlanPanel taskId={TASK} />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  act(() => store.getState().setTaskPlan(TASK, planFor(BASELINE)));
  await waitFor(() => expect(mountedEditor(view.container).editor.isInitialized).toBe(true));
  const mounted = mountedEditor(view.container);
  expect(mounted.editor.getText()).toBe(BASELINE);
  vi.useFakeTimers();
  return { ...view, ...mounted };
}

function typeOutline(editor: Editor, text: string) {
  act(() => {
    expect(
      editor.commands.setContent({
        type: "doc",
        content: [{ type: "paragraph", content: [{ type: "text", text }] }],
      }),
    ).toBe(true);
    editor.view.focus();
    editor.commands.setTextSelection({ from: 3, to: 9 });
  });
}

async function advanceAutosave(intervals = 1) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(AUTO_SAVE_DELAY * intervals);
  });
}

beforeEach(() => {
  fixture.update.mockReset().mockImplementation(async (_taskId, content) => planFor(content));
});

afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("real Plan panel save continuity", () => {
  // @covers AC-TASKS-DOCUMENTS-003.1, AC-TASKS-DOCUMENTS-003.2
  it.each([false, true])(
    "preserves the real editor while an autosave acknowledges on phone: %s",
    async (phone) => {
      Object.assign(fixture.breakpoint, {
        breakpoint: phone ? "mobile" : "desktop",
        isMobile: phone,
        isDesktop: !phone,
        isFullDesktop: !phone,
        isFinePointer: !phone,
        usesDesktopWorkbench: !phone,
      });
      const request = deferredSave();
      fixture.update.mockImplementationOnce(() => request.promise);
      const view = await mountPanel();
      typeOutline(view.editor, SUBMITTED);
      await advanceAutosave();
      expect(fixture.update).toHaveBeenCalledExactlyOnceWith(TASK, SUBMITTED, "Plan");
      expect(store.getState().taskPlans.savingByTaskId[TASK]).toBe(true);
      expect(view.element.getAttribute("contenteditable")).toBe("true");

      typeOutline(view.editor, NEWER);
      expect(document.activeElement).toBe(view.element);
      await act(async () => {
        request.resolve(planFor(SUBMITTED));
        await Promise.resolve();
      });

      expect(store.getState().taskPlans.byTaskId[TASK]?.content).toBe(SUBMITTED);
      expect(view.container.querySelector(".ProseMirror")).toBe(view.element);
      expect(view.editor.isDestroyed).toBe(false);
      expect(view.editor.getText()).toBe(NEWER);
      expect({
        from: view.editor.state.selection.from,
        to: view.editor.state.selection.to,
      }).toEqual({ from: 3, to: 9 });
      expect(document.activeElement).toBe(view.element);
      await advanceAutosave();
      expect(fixture.update).toHaveBeenCalledTimes(2);
      expect(fixture.update).toHaveBeenLastCalledWith(TASK, NEWER, "Plan");
      expect(store.getState().taskPlans.byTaskId[TASK]?.content).toBe(NEWER);
      await advanceAutosave(3);
      expect(fixture.update).toHaveBeenCalledTimes(2);
    },
  );

  // @covers AC-TASKS-DOCUMENTS-003.5, AC-TASKS-DOCUMENTS-003.3
  it("routes Ctrl+S through the same protected draft lifecycle", async () => {
    Object.assign(fixture.breakpoint, {
      breakpoint: "desktop",
      isMobile: false,
      isDesktop: true,
      isFullDesktop: true,
      isFinePointer: true,
      usesDesktopWorkbench: true,
    });
    const request = deferredSave();
    fixture.update.mockImplementationOnce(() => request.promise);
    const view = await mountPanel();
    typeOutline(view.editor, SUBMITTED);
    fireEvent.keyDown(window, { key: "s", ctrlKey: true });
    expect(fixture.update).toHaveBeenCalledExactlyOnceWith(TASK, SUBMITTED, "Plan");
    typeOutline(view.editor, NEWER);
    await act(async () => {
      request.resolve(planFor(SUBMITTED));
      await Promise.resolve();
    });
    expect(view.container.querySelector(".ProseMirror")).toBe(view.element);
    expect(view.editor.getText()).toBe(NEWER);
    await advanceAutosave();
    expect(fixture.update).toHaveBeenCalledTimes(2);
    expect(fixture.update).toHaveBeenLastCalledWith(TASK, NEWER, "Plan");

    fireEvent.keyDown(window, { key: "s", ctrlKey: true });
    await advanceAutosave(3);
    expect(fixture.update).toHaveBeenCalledTimes(2);
  });
});
