"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { searchEntityReferences } from "@/lib/api/domains/mentions-api";
import type { EntityReferenceSearchGroup } from "@/lib/types/entity-reference";
import { t } from "@/lib/i18n";

const SEARCH_DEBOUNCE_MS = 250;

export type UseEntityReferenceSearchOptions = {
  workspaceId: string | null | undefined;
  sessionId?: string | null;
  query: string;
  limit?: number;
  enabled?: boolean;
};

type EntityReferenceSearchState = {
  groups: EntityReferenceSearchGroup[];
  isSearching: boolean;
  error: EntityReferenceSearchError | null;
};

export type EntityReferenceSearchError = {
  message: string;
  retryable: true;
};

const EMPTY_STATE: EntityReferenceSearchState = {
  groups: [],
  isSearching: false,
  error: null,
};

// Catalog key, resolved where the error is surfaced: a module-scope `t()` here
// would pin the message to the boot locale.
const RETRYABLE_SEARCH_ERROR_KEY = "task:referenceSearchFailedRetry";

export function useEntityReferenceSearch({
  workspaceId,
  sessionId,
  query,
  limit,
  enabled = true,
}: UseEntityReferenceSearchOptions) {
  const [state, setState] = useState<EntityReferenceSearchState>(EMPTY_STATE);
  const [retrySequence, setRetrySequence] = useState(0);
  const generationRef = useRef(0);

  useEffect(() => {
    const generation = ++generationRef.current;
    setState(EMPTY_STATE);
    const normalizedQuery = query.trim();
    if (!enabled || !workspaceId || !normalizedQuery) return;

    const controller = new AbortController();
    const timeout = window.setTimeout(() => {
      setState({ groups: [], isSearching: true, error: null });
      searchEntityReferences(
        {
          workspaceId,
          query: normalizedQuery,
          limit,
        },
        { cache: "no-store", init: { signal: controller.signal } },
      ).then(
        (response) => {
          if (generationRef.current !== generation || controller.signal.aborted) return;
          setState({ groups: response.groups, isSearching: false, error: null });
        },
        () => {
          if (generationRef.current !== generation || controller.signal.aborted) return;
          setState({
            groups: [],
            isSearching: false,
            error: { message: t(RETRYABLE_SEARCH_ERROR_KEY), retryable: true },
          });
        },
      );
    }, SEARCH_DEBOUNCE_MS);

    return () => {
      generationRef.current += 1;
      window.clearTimeout(timeout);
      controller.abort();
    };
  }, [enabled, limit, query, retrySequence, sessionId, workspaceId]);

  const retry = useCallback(() => setRetrySequence((sequence) => sequence + 1), []);
  return { ...state, retry };
}
