"use client";

import { useCallback } from "react";
import { useToast } from "@/components/toast-provider";
import type { OpenFileTab } from "@/lib/types/backend";
import { fetchAndOpenFile } from "../file-browser-hooks";

export function useTabletReviewPreview(
  sessionId: string | null,
  onOpenFile: (file: OpenFileTab) => void,
) {
  const { toast } = useToast();

  return useCallback(
    (path: string, repo?: string) => {
      if (!sessionId) return;
      void fetchAndOpenFile(
        sessionId,
        path,
        (file) => onOpenFile({ ...file, markdownPreview: true }),
        toast,
        { repo },
      );
    },
    [onOpenFile, sessionId, toast],
  );
}
