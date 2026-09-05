"use client";

import { useCallback, useEffect, useRef } from "react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import { t } from "@/lib/i18n";
import {
  getHtmlPreviewPublishErrorCode,
  getHtmlPreviewPublishErrorKey,
  publishHtmlPreviewUrl,
} from "./use-html-preview-publisher";

type UseFileHtmlPreviewOptions = {
  activeSessionId: string | null;
  activeSessionIdRef: React.MutableRefObject<string | null>;
  setPublishingHtmlPreview: React.Dispatch<React.SetStateAction<boolean>>;
  openBrowserPanel: (url: string) => void;
  toast: (options: { title: string; description: string; variant: "error" }) => void;
};

export function useFileHtmlPreview({
  activeSessionId,
  activeSessionIdRef,
  setPublishingHtmlPreview,
  openBrowserPanel,
  toast,
}: UseFileHtmlPreviewOptions) {
  const requestGenerationRef = useRef(0);

  useEffect(() => {
    requestGenerationRef.current += 1;
    setPublishingHtmlPreview(false);
    return () => {
      requestGenerationRef.current += 1;
    };
  }, [activeSessionId, setPublishingHtmlPreview]);

  return useCallback(
    async (filePath: string, repo?: string) => {
      const sessionId = activeSessionIdRef.current;
      const fileKey = buildRepoScopedItemId(filePath, repo);
      const file = useDockviewStore.getState().openFiles.get(fileKey);
      if (!sessionId || !file) return;

      const requestGeneration = requestGenerationRef.current + 1;
      requestGenerationRef.current = requestGeneration;
      const isCurrentRequest = () =>
        requestGenerationRef.current === requestGeneration &&
        activeSessionIdRef.current === sessionId;
      const isCurrentTarget = () =>
        isCurrentRequest() && useDockviewStore.getState().openFiles.has(fileKey);

      setPublishingHtmlPreview(true);
      try {
        const url = await publishHtmlPreviewUrl(sessionId, {
          path: filePath,
          repo: file.repo ?? repo,
          content: file.content,
        });
        if (!isCurrentTarget()) return;
        openBrowserPanel(url);
      } catch (error) {
        if (!isCurrentTarget()) return;
        toast({
          title: t("task:htmlPreviewError"),
          description: t(getHtmlPreviewPublishErrorKey(getHtmlPreviewPublishErrorCode(error))),
          variant: "error",
        });
      } finally {
        if (isCurrentRequest()) setPublishingHtmlPreview(false);
      }
    },
    [activeSessionIdRef, openBrowserPanel, setPublishingHtmlPreview, toast],
  );
}
