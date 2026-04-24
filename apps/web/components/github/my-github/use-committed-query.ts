"use client";

import { useCallback, useState } from "react";

// Track a user-editable string where the draft only commits when the caller
// explicitly asks. Typing updates `draft` for responsive input display but
// does not trigger downstream fetches on `committed`; Enter or blur in the
// input calls `setImmediate` to commit and run the query.
export function useCommittedQuery(initial: string) {
  const [draft, setDraft] = useState(initial);
  const [committed, setCommitted] = useState(initial);

  const setImmediate = useCallback((value: string) => {
    setDraft(value);
    setCommitted(value);
  }, []);

  const commit = useCallback(() => setCommitted(draft), [draft]);

  return { draft, committed, setDraft, setImmediate, commit };
}
