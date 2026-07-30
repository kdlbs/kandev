import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { deleteTaskNote, getTaskNote, updateTaskNote } from "@/lib/api/domains/note-api";
import type { TaskNote } from "@/lib/types/http";

const AUTO_SAVE_DELAY = 1500;

// eslint-disable-next-line max-lines-per-function -- note state, fetch, and autosave stay together to mirror the panel lifecycle.
export function useTaskNote(taskId: string | null, options?: { visible?: boolean }) {
  const { visible = true } = options ?? {};
  const prevVisibleRef = useRef(visible);
  const note = useAppStore((state) => (taskId ? (state.taskNotes.byTaskId[taskId] ?? null) : null));
  const isLoading = useAppStore((state) =>
    taskId ? (state.taskNotes.loadingByTaskId[taskId] ?? false) : false,
  );
  const isSaving = useAppStore((state) =>
    taskId ? (state.taskNotes.savingByTaskId[taskId] ?? false) : false,
  );
  const setTaskNote = useAppStore((state) => state.setTaskNote);
  const setTaskNoteLoading = useAppStore((state) => state.setTaskNoteLoading);
  const setTaskNoteSaving = useAppStore((state) => state.setTaskNoteSaving);
  const clearTaskNote = useAppStore((state) => state.clearTaskNote);
  const connectionStatus = useAppStore((state) => state.connection.status);

  const [error, setError] = useState<string | null>(null);
  const [draftContent, setDraftContent] = useState(note?.content ?? "");
  const [editorKey, setEditorKey] = useState(0);
  const draftContentRef = useRef(draftContent);
  const lastNoteContentRef = useRef<string | undefined>(undefined);
  const isExternalUpdateRef = useRef(false);
  const autoSaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    draftContentRef.current = draftContent;
  }, [draftContent]);

  const fetchNote = useCallback(async () => {
    if (!taskId) return;
    setTaskNoteLoading(taskId, true);
    setError(null);
    try {
      const fetchedNote = await getTaskNote(taskId);
      setTaskNote(taskId, fetchedNote);
    } catch (err) {
      console.error("Failed to fetch task note:", err);
      setError(err instanceof Error ? err.message : "Failed to fetch note");
    } finally {
      setTaskNoteLoading(taskId, false);
    }
  }, [taskId, setTaskNote, setTaskNoteLoading]);

  useEffect(() => {
    if (connectionStatus !== "connected" || !taskId) return;
    void fetchNote();
  }, [taskId, connectionStatus, fetchNote]);

  useEffect(() => {
    const wasHidden = !prevVisibleRef.current;
    prevVisibleRef.current = visible;
    if (wasHidden && visible && connectionStatus === "connected" && taskId) {
      void fetchNote();
    }
  }, [visible, connectionStatus, taskId, fetchNote]);

  useEffect(() => {
    const prevContent = lastNoteContentRef.current;
    const newContent = note?.content;
    lastNoteContentRef.current = newContent;
    if (newContent !== prevContent) {
      const resolved = newContent ?? "";
      if (resolved === draftContentRef.current) return;
      isExternalUpdateRef.current = true;
      setDraftContent(resolved);
      setEditorKey((key) => key + 1);
    }
  }, [note?.content]);

  const saveNote = useCallback(
    async (content: string, updatedBy?: TaskNote["updated_by"]): Promise<TaskNote | null> => {
      if (!taskId) return null;
      const trimmed = content.trim();
      setTaskNoteSaving(taskId, true);
      setError(null);
      try {
        if (trimmed === "") {
          if (note) {
            await deleteTaskNote(taskId);
            setTaskNote(taskId, null);
          } else {
            clearTaskNote(taskId);
          }
          return null;
        }

        const savedNote = await updateTaskNote(taskId, content, updatedBy);
        setTaskNote(taskId, savedNote);
        return savedNote;
      } catch (err) {
        console.error("Failed to save task note:", err);
        setError(err instanceof Error ? err.message : "Failed to save note");
        return null;
      } finally {
        setTaskNoteSaving(taskId, false);
      }
    },
    [taskId, note, setTaskNote, setTaskNoteSaving, clearTaskNote],
  );

  useEffect(() => {
    if (!taskId) return;
    if (isExternalUpdateRef.current) {
      isExternalUpdateRef.current = false;
      return;
    }
    const trimmed = draftContent.trim();
    const hasChanges = note ? draftContent !== note.content : trimmed.length > 0;
    const shouldDelete = note !== null && trimmed.length === 0 && draftContent !== note.content;
    if ((!hasChanges && !shouldDelete) || isSaving) return;
    if (autoSaveTimerRef.current) clearTimeout(autoSaveTimerRef.current);
    autoSaveTimerRef.current = setTimeout(() => {
      autoSaveTimerRef.current = null;
      void saveNote(draftContent);
    }, AUTO_SAVE_DELAY);
    return () => {
      if (autoSaveTimerRef.current) {
        clearTimeout(autoSaveTimerRef.current);
        autoSaveTimerRef.current = null;
      }
    };
  }, [taskId, draftContent, note, isSaving, saveNote]);

  const hasUnsavedChanges = note ? draftContent !== note.content : draftContent.trim().length > 0;

  return {
    note,
    draftContent,
    setDraftContent,
    editorKey,
    isLoading,
    isSaving,
    error,
    hasUnsavedChanges,
    saveNote,
    refetch: fetchNote,
  };
}
