"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { listTurnChangeFiles } from "@/lib/api/domains/turn-changes-api";
import { decodeTurnChangeContent, readTurnChangeContent } from "@/lib/api/domains/turn-changes-api";
import type {
  TurnChangeContentVariant,
  TurnChangeSetSummary,
  TurnFileChange,
} from "@/lib/types/turn-changes";
import type { HistoricalTurnDiffTarget } from "@/lib/state/diff-target-types";

const FILE_PAGE_SIZE = 100;

async function fetchFilePage(
  sessionId: string,
  changeSetId: string,
  repositoryId: string,
  offset: number,
) {
  const page = await listTurnChangeFiles(
    sessionId,
    changeSetId,
    repositoryId,
    { offset, limit: FILE_PAGE_SIZE },
    { cache: "no-store" },
  );
  return [repositoryId, page] as const;
}

export function useTurnChangeFiles(sessionId: string, summary: TurnChangeSetSummary) {
  const [filesByRepository, setFilesByRepository] = useState<Record<string, TurnFileChange[]>>({});
  const [fileTotalsByRepository, setFileTotalsByRepository] = useState<Record<string, number>>({});
  const [nextOffsets, setNextOffsets] = useState<Record<string, number | undefined>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [retryKey, setRetryKey] = useState(0);
  const reload = useCallback(() => setRetryKey((current) => current + 1), []);
  const generation = useRef(0);
  const busy = useRef(false);
  const initialFailed = useRef(false);
  const repositories = useMemo(() => summary.repositories ?? [], [summary.repositories]);

  useEffect(() => {
    const requestGeneration = ++generation.current;
    setFilesByRepository({});
    setFileTotalsByRepository({});
    setNextOffsets({});
    busy.current = repositories.length > 0;
    setLoading(busy.current);
    setError(null);
    initialFailed.current = false;
    if (repositories.length === 0) return;
    void Promise.allSettled(
      repositories.map((repository) => fetchFilePage(sessionId, summary.id, repository.id, 0)),
    )
      .then((results) => {
        if (requestGeneration !== generation.current) return;
        const pages = results.flatMap((result) =>
          result.status === "fulfilled" ? [result.value] : [],
        );
        const failure = results.find((result) => result.status === "rejected");
        initialFailed.current = Boolean(failure);
        if (failure?.status === "rejected") setError(failure.reason);
        setFilesByRepository(Object.fromEntries(pages.map(([id, page]) => [id, page.files])));
        setFileTotalsByRepository(Object.fromEntries(pages.map(([id, page]) => [id, page.total])));
        setNextOffsets(Object.fromEntries(pages.map(([id, page]) => [id, page.next_offset])));
      })
      .catch((err) => {
        if (requestGeneration === generation.current) {
          initialFailed.current = true;
          setError(err);
        }
      })
      .finally(() => {
        if (requestGeneration === generation.current) {
          busy.current = false;
          setLoading(false);
        }
      });
    return () => {
      generation.current++;
    };
  }, [repositories, retryKey, sessionId, summary.id]);

  const loadMore = useCallback(async () => {
    const pages = repositories.filter((repository) => nextOffsets[repository.id] !== undefined);
    if (pages.length === 0 || busy.current) return;
    const requestGeneration = generation.current;
    busy.current = true;
    setLoading(true);
    setError(null);
    try {
      const loaded = await Promise.all(
        pages.map((repository) =>
          fetchFilePage(sessionId, summary.id, repository.id, nextOffsets[repository.id] ?? 0),
        ),
      );
      if (requestGeneration !== generation.current) return;
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
      if (requestGeneration === generation.current) setError(err);
    } finally {
      if (requestGeneration === generation.current) {
        busy.current = false;
        setLoading(false);
      }
    }
  }, [nextOffsets, repositories, sessionId, summary.id]);

  const retry = useCallback(async () => {
    if (initialFailed.current) reload();
    else await loadMore();
  }, [loadMore, reload]);

  return {
    filesByRepository,
    fileTotalsByRepository,
    loading,
    error,
    hasMore: Object.values(nextOffsets).some((offset) => offset !== undefined),
    loadMore,
    reload,
    retry,
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
    setLoading(Boolean(fileChangeId));
    if (!fileChangeId) return;
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
