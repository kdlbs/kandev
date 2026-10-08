"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { listTurnChangeFiles } from "@/lib/api/domains/turn-changes-api";
import { decodeTurnChangeContent, readTurnChangeContent } from "@/lib/api/domains/turn-changes-api";
import type {
  TurnChangeContentVariant,
  TurnChangeSetSummary,
  TurnFileChange,
} from "@/lib/types/turn-changes";
import type { HistoricalTurnDiffTarget } from "@/lib/state/diff-target-types";

const FILE_PAGE_SIZE = 100;

export function useTurnChangeFiles(sessionId: string, summary: TurnChangeSetSummary) {
  const [filesByRepository, setFilesByRepository] = useState<Record<string, TurnFileChange[]>>({});
  const [fileTotalsByRepository, setFileTotalsByRepository] = useState<Record<string, number>>({});
  const [nextOffsets, setNextOffsets] = useState<Record<string, number | undefined>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const repositories = useMemo(() => summary.repositories ?? [], [summary.repositories]);

  useEffect(() => {
    let canceled = false;
    setFilesByRepository({});
    setFileTotalsByRepository({});
    setNextOffsets({});
    if (repositories.length === 0) return;
    setLoading(true);
    setError(null);
    void Promise.all(
      repositories.map(async (repository) => {
        const page = await listTurnChangeFiles(
          sessionId,
          summary.id,
          repository.id,
          { offset: 0, limit: FILE_PAGE_SIZE },
          { cache: "no-store" },
        );
        return [repository.id, page] as const;
      }),
    )
      .then((pages) => {
        if (canceled) return;
        setFilesByRepository(Object.fromEntries(pages.map(([id, page]) => [id, page.files])));
        setFileTotalsByRepository(Object.fromEntries(pages.map(([id, page]) => [id, page.total])));
        setNextOffsets(Object.fromEntries(pages.map(([id, page]) => [id, page.next_offset])));
      })
      .catch(setError)
      .finally(() => {
        if (!canceled) setLoading(false);
      });
    return () => {
      canceled = true;
    };
  }, [repositories, sessionId, summary.id]);

  const loadMore = useCallback(async () => {
    const pages = repositories.filter((repository) => nextOffsets[repository.id] !== undefined);
    if (pages.length === 0) return;
    setLoading(true);
    setError(null);
    try {
      const loaded = await Promise.all(
        pages.map(async (repository) => {
          const offset = nextOffsets[repository.id] ?? 0;
          const page = await listTurnChangeFiles(
            sessionId,
            summary.id,
            repository.id,
            { offset, limit: FILE_PAGE_SIZE },
            { cache: "no-store" },
          );
          return [repository.id, page] as const;
        }),
      );
      setFilesByRepository((current) => {
        const next = { ...current };
        for (const [id, page] of loaded) next[id] = [...(next[id] ?? []), ...page.files];
        return next;
      });
      setNextOffsets((current) => ({
        ...current,
        ...Object.fromEntries(loaded.map(([id, page]) => [id, page.next_offset])),
      }));
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  }, [nextOffsets, repositories, sessionId, summary.id]);

  return {
    filesByRepository,
    fileTotalsByRepository,
    loading,
    error,
    hasMore: Object.values(nextOffsets).some((offset) => offset !== undefined),
    loadMore,
  };
}

export function useTurnChangeContent(
  target: HistoricalTurnDiffTarget,
  fileChangeId: string | null,
  whitespace: boolean,
  retryKey = 0,
) {
  const [patch, setPatch] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const variant: TurnChangeContentVariant = whitespace ? "filtered_patch" : "canonical_patch";

  useEffect(() => {
    let canceled = false;
    setPatch(null);
    setError(null);
    if (!fileChangeId) return;
    setLoading(true);
    void readTurnChangeContent(target.sessionId, target.changeSetId, fileChangeId, variant, {
      cache: "no-store",
    })
      .then((response) => {
        if (!canceled) setPatch(decodeTurnChangeContent(response.content));
      })
      .catch((err) => {
        if (!canceled) setError(err);
      })
      .finally(() => {
        if (!canceled) setLoading(false);
      });
    return () => {
      canceled = true;
    };
  }, [fileChangeId, retryKey, target.changeSetId, target.sessionId, variant]);

  return { patch, loading, error };
}
