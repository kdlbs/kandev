"use client";

import { useCallback, useState } from "react";

// Mirror of github's useCommittedQuery: the draft is what the input renders,
// the committed value is what drives the fetch. Typing only updates the draft;
// Enter / blur commits.
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
