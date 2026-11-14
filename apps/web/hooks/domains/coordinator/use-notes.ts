"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getNote,
  getNoteReferences,
  listNotes,
  type NoteDetail,
  type NoteReference,
  type NoteView,
} from "@/lib/api/domains/coordinator-notes-api";

export type NotesLoadStatus = "loading" | "ready" | "error";

/**
 * A coordinator's notes. The response of the last request sent wins; a failed
 * reload keeps the list already loaded, so `error` means there is nothing to show.
 */
export function useNotes(workspaceId: string, coordinatorId: string, includeDeleted: boolean) {
  const [notes, setNotes] = useState<NoteView[]>([]);
  const [status, setStatus] = useState<NotesLoadStatus>("loading");
  const sequenceRef = useRef(0);
  const loadedRef = useRef(false);

  const reload = useCallback((): Promise<void> => {
    const sequence = ++sequenceRef.current;
    return listNotes(workspaceId, coordinatorId, includeDeleted)
      .then((next) => {
        if (sequence !== sequenceRef.current) return;
        loadedRef.current = true;
        setNotes(next);
        setStatus("ready");
      })
      .catch(() => {
        if (sequence !== sequenceRef.current || loadedRef.current) return;
        setStatus("error");
      });
  }, [workspaceId, coordinatorId, includeDeleted]);

  const retry = useCallback(() => {
    setStatus("loading");
    void reload();
  }, [reload]);

  useEffect(() => {
    loadedRef.current = false;
    setNotes([]);
    setStatus("loading");
    void reload();
    return () => {
      sequenceRef.current += 1;
    };
  }, [reload]);

  return { notes, status, reload, retry };
}

export type NoteDetailState = {
  status: NotesLoadStatus;
  detail: NoteDetail | null;
  history: NoteReference[];
};

/** One note with its versions and reference history; `refresh` rereads both. */
export function useNoteDetail(workspaceId: string, coordinatorId: string, noteId: string) {
  const [state, setState] = useState<NoteDetailState>({
    status: "loading",
    detail: null,
    history: [],
  });
  const sequenceRef = useRef(0);

  const refresh = useCallback((): Promise<void> => {
    const sequence = ++sequenceRef.current;
    return Promise.all([
      getNote(workspaceId, coordinatorId, noteId),
      getNoteReferences(workspaceId, coordinatorId, noteId),
    ])
      .then(([detail, history]) => {
        if (sequence === sequenceRef.current) setState({ status: "ready", detail, history });
      })
      .catch(() => {
        if (sequence === sequenceRef.current) {
          setState((prev) => (prev.detail ? prev : { ...prev, status: "error" }));
        }
      });
  }, [workspaceId, coordinatorId, noteId]);

  useEffect(() => {
    setState({ status: "loading", detail: null, history: [] });
    void refresh();
    return () => {
      sequenceRef.current += 1;
    };
  }, [refresh]);

  return { ...state, refresh };
}
