import { useEffect, useRef } from "react";
import type { TurnFileChange } from "@/lib/types/turn-changes";

type ReadyContentRetry = {
  selectedFile: TurnFileChange | null;
  contentError: unknown;
  contentLoading: boolean;
  whitespace: boolean;
  repositorySettled: boolean;
  retry: () => void;
};

export function useReadyContentRetry({
  selectedFile,
  contentError,
  contentLoading,
  whitespace,
  repositorySettled,
  retry,
}: ReadyContentRetry) {
  const retried = useRef(new Set<string>());
  useEffect(() => {
    if (!selectedFile || !contentError || contentLoading) return;
    const readyFile = selectedFile.content_availability === "ready";
    const settledSynthetic = selectedFile.synthetic === true && repositorySettled;
    if (!readyFile && !settledSynthetic) return;
    const key = `${selectedFile.id}:${whitespace}`;
    if (retried.current.has(key)) return;
    retried.current.add(key);
    retry();
  }, [contentError, contentLoading, repositorySettled, retry, selectedFile, whitespace]);
}
