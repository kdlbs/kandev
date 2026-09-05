"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  getHtmlPreviewPublishErrorCode,
  getHtmlPreviewPublishErrorKey,
  publishHtmlPreviewUrl,
} from "./use-html-preview-publisher";

type UseTaskCenterHtmlPreviewOptions = {
  activeSessionId: string | null;
  openBrowserPanel: (url: string) => void;
  toast: (options: { title: string; description: string; variant: "error" }) => void;
};

export function useTaskCenterHtmlPreview({
  activeSessionId,
  openBrowserPanel,
  toast,
}: UseTaskCenterHtmlPreviewOptions) {
  const { t } = useTranslation();
  const [isPublishingHtmlPreview, setIsPublishingHtmlPreview] = useState(false);

  const handlePreviewHtml = useCallback(
    async (path: string, repo: string | undefined, content: string) => {
      if (!activeSessionId) {
        toast({
          title: t("task:htmlPreviewError"),
          description: t(getHtmlPreviewPublishErrorKey("session-unavailable")),
          variant: "error",
        });
        return;
      }
      setIsPublishingHtmlPreview(true);
      try {
        const url = await publishHtmlPreviewUrl(activeSessionId, { path, repo, content });
        openBrowserPanel(url);
      } catch (error) {
        toast({
          title: t("task:htmlPreviewError"),
          description: t(getHtmlPreviewPublishErrorKey(getHtmlPreviewPublishErrorCode(error))),
          variant: "error",
        });
      } finally {
        setIsPublishingHtmlPreview(false);
      }
    },
    [activeSessionId, openBrowserPanel, t, toast],
  );

  return { handlePreviewHtml, isPublishingHtmlPreview };
}
