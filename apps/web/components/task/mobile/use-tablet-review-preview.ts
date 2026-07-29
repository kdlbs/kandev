"use client";

import { useCallback, useEffect, useRef } from "react";
import { useToast } from "@/components/toast-provider";
import type { OpenFileTab } from "@/lib/types/backend";
import { fetchAndOpenFile } from "../file-browser-hooks";

export function useTabletReviewPreview(
  sessionId: string | null,
  onOpenFile: (file: OpenFileTab) => void,
) {
  const { toast } = useToast();
  const latestRequestIdRef = useRef(0);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    latestRequestIdRef.current += 1;
    abortRef.current?.abort();
    abortRef.current = null;
    return () => {
      abortRef.current?.abort();
      abortRef.current = null;
    };
  }, [sessionId]);

  return useCallback(
    (path: string, repo?: string) => {
      if (!sessionId) return;
      const requestId = (latestRequestIdRef.current += 1);
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;
      void Promise.resolve(
        fetchAndOpenFile(
          sessionId,
          path,
          (file) => {
            if (requestId !== latestRequestIdRef.current || controller.signal.aborted) return;
            onOpenFile({ ...file, markdownPreview: true });
          },
          toast,
          { repo, signal: controller.signal },
        ),
      ).finally(() => {
        if (abortRef.current === controller) abortRef.current = null;
      });
    },
    [onOpenFile, sessionId, toast],
  );
}
