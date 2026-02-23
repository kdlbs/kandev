"use client";

import { useState, useEffect, useCallback, useReducer } from "react";
import { getPRFeedback } from "@/lib/api/domains/github-api";
import type { PRFeedback } from "@/lib/types/github";

type State = {
  feedback: PRFeedback | null;
  loading: boolean;
  error: string | null;
};

type Action =
  | { type: "fetch" }
  | { type: "success"; feedback: PRFeedback }
  | { type: "error"; message: string };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null };
    case "success":
      return { feedback: action.feedback, loading: false, error: null };
    case "error":
      return { ...state, loading: false, error: action.message };
  }
}

/**
 * Fetch live PR feedback (reviews, comments, checks) from GitHub.
 * This is not stored in the global store since it's session-scoped
 * and fetched on demand.
 */
export function usePRFeedback(owner: string | null, repo: string | null, prNumber: number | null) {
  const [state, dispatch] = useReducer(reducer, { feedback: null, loading: false, error: null });
  const [fetchCount, setFetchCount] = useState(0);

  const refresh = useCallback(() => {
    setFetchCount((c) => c + 1);
  }, []);

  useEffect(() => {
    if (!owner || !repo || !prNumber) return;
    let cancelled = false;
    dispatch({ type: "fetch" });
    getPRFeedback(owner, repo, prNumber, { cache: "no-store" })
      .then((response) => {
        if (!cancelled) dispatch({ type: "success", feedback: response });
      })
      .catch((err) => {
        if (!cancelled)
          dispatch({
            type: "error",
            message: err instanceof Error ? err.message : "Failed to fetch PR feedback",
          });
      });
    return () => {
      cancelled = true;
    };
  }, [owner, repo, prNumber, fetchCount]);

  return { feedback: state.feedback, loading: state.loading, error: state.error, refresh };
}
