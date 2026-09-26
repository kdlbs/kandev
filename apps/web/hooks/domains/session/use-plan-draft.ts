import { useCallback, useEffect, useRef, useState } from "react";
import type { RefObject } from "react";
import type { PlanSaveError } from "./use-task-plan";

/** Debounce delay for auto-saving plan content (ms). */
const AUTO_SAVE_DELAY = 1500;

type PlanDraft = { content?: string; title?: string };

type UsePlanDraftOptions<T> = {
  plan: PlanDraft | null | undefined;
  isSaving: boolean;
  savePlan: (content: string, title?: string) => Promise<T>;
  editorWrapperRef: RefObject<HTMLDivElement | null>;
  taskId: string | null;
  saveError?: PlanSaveError | null;
};

/** Draft content, editor key, focus tracking, and auto-save for one task. */
// eslint-disable-next-line max-lines-per-function -- this hook owns one cohesive draft lifecycle
export function usePlanDraft<T>({
  plan,
  isSaving,
  savePlan,
  editorWrapperRef,
  taskId,
  saveError = null,
}: UsePlanDraftOptions<T>) {
  const [draftContent, setDraftContent] = useState(plan?.content ?? "");
  const draftContentRef = useRef(draftContent);
  const [editorKey, setEditorKey] = useState(0);
  const lastPlanContentRef = useRef<string | undefined>(undefined);
  const previousTaskIdRef = useRef(taskId);
  const isExternalUpdateRef = useRef(false);
  const [isEditorFocused, setIsEditorFocused] = useState(false);
  const autoSaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Content of the most recent save attempt. A size rejection suppresses
  // only an unchanged retry; a generic failure remains eligible for retry.
  const lastAttemptContentRef = useRef<string | null>(null);

  // Single entry point for dispatching a savePlan call, used by both the
  // autosave timer and the explicit Ctrl/Cmd+S shortcut. Record the content
  // before the request starts so the rejection remains suppressed across the
  // saving-state transition.
  const attemptSave = useCallback(
    (content: string, title?: string) => {
      lastAttemptContentRef.current = content;
      return savePlan(content, title).then((saved) => {
        // Two attempts can overlap. Only a success for the currently tracked
        // content may clear the suppression set by the latest attempt.
        if (saved && lastAttemptContentRef.current === content) {
          lastAttemptContentRef.current = null;
        }
        return saved;
      });
    },
    [savePlan],
  );

  const handleEmptyStateClick = useCallback(() => {
    const el = editorWrapperRef.current?.querySelector(".ProseMirror");
    if (el) (el as HTMLElement).focus();
  }, [editorWrapperRef]);

  // Track focus state.
  useEffect(() => {
    const checkFocus = () => {
      const wrapper = editorWrapperRef.current;
      if (!wrapper) return;
      setIsEditorFocused(wrapper.contains(document.activeElement));
    };
    document.addEventListener("focusin", checkFocus);
    document.addEventListener("focusout", checkFocus);
    checkFocus();
    return () => {
      document.removeEventListener("focusin", checkFocus);
      document.removeEventListener("focusout", checkFocus);
    };
  }, [editorWrapperRef]);

  useEffect(() => {
    draftContentRef.current = draftContent;
  }, [draftContent]);

  // Sync external plan updates. A task identity change is an external update
  // even when both tasks have no plan (or equal content), because the local
  // draft belongs to the previous task and must never cross that boundary.
  useEffect(() => {
    const taskChanged = previousTaskIdRef.current !== taskId;
    previousTaskIdRef.current = taskId;

    const prevContent = lastPlanContentRef.current;
    const newContent = plan?.content;
    lastPlanContentRef.current = newContent;

    if (taskChanged) {
      const resolved = newContent ?? "";
      lastAttemptContentRef.current = null;
      isExternalUpdateRef.current = true;
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing task-scoped editor data
      setDraftContent(resolved);
      setEditorKey((key) => key + 1);
      return;
    }

    if (newContent !== prevContent) {
      const resolved = newContent ?? "";
      if (resolved === draftContentRef.current) return;
      isExternalUpdateRef.current = true;
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing external plan data to local editor state
      setDraftContent(resolved);
      setEditorKey((key) => key + 1);
    }
  }, [plan?.content, taskId]);

  // Auto-save with debounce.
  useEffect(() => {
    if (isExternalUpdateRef.current) {
      isExternalUpdateRef.current = false;
      return;
    }
    const hasChanges = plan ? draftContent !== plan.content : draftContent.length > 0;
    if (!hasChanges || isSaving) return;
    // Only a known size rejection suppresses an unchanged retry. Generic
    // transport/server failures can be transient and must remain retryable.
    if (saveError?.kind === "content-too-large" && draftContent === lastAttemptContentRef.current) {
      return;
    }
    if (autoSaveTimerRef.current) clearTimeout(autoSaveTimerRef.current);
    autoSaveTimerRef.current = setTimeout(() => {
      autoSaveTimerRef.current = null;
      attemptSave(draftContent, plan?.title);
    }, AUTO_SAVE_DELAY);
    return () => {
      if (autoSaveTimerRef.current) {
        clearTimeout(autoSaveTimerRef.current);
        autoSaveTimerRef.current = null;
      }
    };
  }, [draftContent, plan, isSaving, attemptSave, saveError, taskId]);

  const hasUnsavedChanges = plan ? draftContent !== plan.content : draftContent.length > 0;
  return {
    draftContent,
    attemptSave,
    setDraftContent,
    editorKey,
    isEditorFocused,
    handleEmptyStateClick,
    hasUnsavedChanges,
  };
}
