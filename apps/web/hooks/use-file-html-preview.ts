"use client";

import { useCallback } from "react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import { t } from "@/lib/i18n";
import {
  getHtmlPreviewPublishErrorCode,
  getHtmlPreviewPublishErrorKey,
  publishHtmlPreviewUrl,
} from "./use-html-preview-publisher";

type UseFileHtmlPreviewOptions = {
  activeSessionIdRef: React.MutableRefObject<string | null>;
  setPublishingHtmlPreview: React.Dispatch<React.SetStateAction<boolean>>;
  openBrowserPanel: (url: string) => void;
  toast: (options: { title: string; description: string; variant: "error" }) => void;
};

export function useFileHtmlPreview({
  activeSessionIdRef,
  setPublishingHtmlPreview,
  openBrowserPanel,
  toast,
}: UseFileHtmlPreviewOptions) {
  return useCallback(
    async (filePath: string, repo?: string) => {
      const sessionId = activeSessionIdRef.current;
      const fileKey = buildRepoScopedItemId(filePath, repo);
      const file = useDockviewStore.getState().openFiles.get(fileKey);
      if (!sessionId || !file) return;

      setPublishingHtmlPreview(true);
      try {
        const url = await publishHtmlPreviewUrl(sessionId, {
          path: filePath,
          repo: file.repo ?? repo,
          content: file.content,
        });
        if (activeSessionIdRef.current !== sessionId) return;
        if (!useDockviewStore.getState().openFiles.has(fileKey)) return;
        openBrowserPanel(url);
      } catch (error) {
        toast({
          title: t("task:htmlPreviewError"),
          description: t(getHtmlPreviewPublishErrorKey(getHtmlPreviewPublishErrorCode(error))),
          variant: "error",
        });
      } finally {
        setPublishingHtmlPreview(false);
      }
    },
    [activeSessionIdRef, openBrowserPanel, setPublishingHtmlPreview, toast],
  );
}
