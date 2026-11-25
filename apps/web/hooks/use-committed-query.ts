"use client";

import { useCallback, useState } from "react";

// Keep the editable draft separate from the committed value that drives the search.
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
