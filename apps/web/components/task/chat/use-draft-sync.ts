"use client";

import { useRef, useCallback, useEffect } from "react";
import { saveSessionDraft, getSessionDraft } from "@/lib/api/domains/session-api";
import { getChatDraftText } from "@/lib/local-storage";
import type { TipTapInputHandle } from "./tiptap-input";

const DEBOUNCE_MS = 2000;

type DraftSyncResult = {
  /** Debounced save — call on every input change */
  saveToServer: (text: string, content: unknown) => void;
  /** Immediate clear — call on submit */
  clearServerDraft: () => void;
};

/**
 * Syncs chat input drafts to the server for cross-device persistence.
 *
 * On mount / session change: fetches the server draft. If local sessionStorage
 * is empty (cross-device case), populates the editor from the server draft.
 *
 * On input change: debounced save to server (2s after last keystroke).
 * On submit: immediately clears the server draft.
 */
export function useDraftSync(
  sessionId: string | null,
  inputRef: React.RefObject<TipTapInputHandle | null>,
  setValue: (value: string) => void,
): DraftSyncResult {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const sessionIdRef = useRef(sessionId);

  // Track current sessionId for stale-check in async callbacks
  useEffect(() => {
    sessionIdRef.current = sessionId;
  }, [sessionId]);

  // Fetch server draft on session change — restore if local is empty
  useEffect(() => {
    if (!sessionId) return;

    let cancelled = false;

    (async () => {
      try {
        const draft = await getSessionDraft(sessionId);
        if (cancelled || sessionIdRef.current !== sessionId) return;
        if (!draft || !draft.text) return;

        // Only restore from server if local sessionStorage is empty (cross-device case)
        const localDraft = getChatDraftText(sessionId);
        if (localDraft) return;

        // Populate the editor
        if (draft.content && inputRef.current) {
          inputRef.current.setContent(draft.content);
        } else if (draft.text) {
          setValue(draft.text);
        }
      } catch {
        // Best-effort — silently ignore failures
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [sessionId, inputRef, setValue]);

  // Cancel pending debounce on unmount or session change
  useEffect(() => {
    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [sessionId]);

  const saveToServer = useCallback(
    (text: string, content: unknown) => {
      if (!sessionId) return;

      // Clear previous debounce
      if (timerRef.current) {
        clearTimeout(timerRef.current);
      }

      timerRef.current = setTimeout(() => {
        timerRef.current = null;
        if (sessionIdRef.current !== sessionId) return;
        saveSessionDraft(sessionId, text, content).catch(() => {
          // Best-effort — silently ignore save failures
        });
      }, DEBOUNCE_MS);
    },
    [sessionId],
  );

  const clearServerDraft = useCallback(() => {
    if (!sessionId) return;

    // Cancel any pending debounced save
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }

    saveSessionDraft(sessionId, "", null).catch(() => {
      // Best-effort
    });
  }, [sessionId]);

  return { saveToServer, clearServerDraft };
}
