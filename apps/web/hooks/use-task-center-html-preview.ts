"use client";

import { useCallback, useEffect, useRef, useState } from "react";
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
  const activeSessionIdRef = useRef(activeSessionId);
  const requestGenerationRef = useRef(0);

  useEffect(() => {
    activeSessionIdRef.current = activeSessionId;
    requestGenerationRef.current += 1;
    setIsPublishingHtmlPreview(false);
    return () => {
      requestGenerationRef.current += 1;
    };
  }, [activeSessionId]);

  const handlePreviewHtml = useCallback(
    async (path: string, repo: string | undefined, content: string) => {
      const sessionId = activeSessionIdRef.current;
      if (!sessionId) {
        toast({
          title: t("task:htmlPreviewError"),
          description: t(getHtmlPreviewPublishErrorKey("session-unavailable")),
          variant: "error",
        });
        return;
      }
      const requestGeneration = requestGenerationRef.current + 1;
      requestGenerationRef.current = requestGeneration;
      const isCurrentRequest = () =>
        requestGenerationRef.current === requestGeneration &&
        activeSessionIdRef.current === sessionId;

      setIsPublishingHtmlPreview(true);
      try {
        const url = await publishHtmlPreviewUrl(sessionId, { path, repo, content });
        if (!isCurrentRequest()) return;
        openBrowserPanel(url);
      } catch (error) {
        if (!isCurrentRequest()) return;
        toast({
          title: t("task:htmlPreviewError"),
          description: t(getHtmlPreviewPublishErrorKey(getHtmlPreviewPublishErrorCode(error))),
          variant: "error",
        });
      } finally {
        if (isCurrentRequest()) setIsPublishingHtmlPreview(false);
      }
    },
    [openBrowserPanel, t, toast],
  );

  return { handlePreviewHtml, isPublishingHtmlPreview };
}
