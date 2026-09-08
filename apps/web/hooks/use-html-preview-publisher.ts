"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import {
  buildHtmlPreviewProxyUrl,
  publishHtmlPreview,
  type HtmlPreviewPublishRequest,
} from "@/lib/api/domains/process-api";

export type HtmlPreviewPublishErrorCode = "session-unavailable" | "too-large" | "publish-failed";

export type HtmlPreviewPublishState = {
  status: "idle" | "publishing" | "ready" | "error";
  url: string | null;
  error: HtmlPreviewPublishErrorCode | null;
};

const IDLE_STATE: HtmlPreviewPublishState = {
  status: "idle",
  url: null,
  error: null,
};

export function getHtmlPreviewPublishErrorCode(error: unknown): HtmlPreviewPublishErrorCode {
  if (error instanceof ApiError && error.status === 413) return "too-large";
  if (error instanceof ApiError && [404, 409, 503].includes(error.status)) {
    return "session-unavailable";
  }
  return "publish-failed";
}

export function getHtmlPreviewPublishErrorKey(
  error: HtmlPreviewPublishErrorCode,
): "task:htmlPreviewTooLarge" | "task:htmlPreviewUnavailable" | "task:htmlPreviewPublishFailed" {
  if (error === "too-large") return "task:htmlPreviewTooLarge";
  if (error === "session-unavailable") return "task:htmlPreviewUnavailable";
  return "task:htmlPreviewPublishFailed";
}

export async function publishHtmlPreviewUrl(
  sessionId: string,
  payload: HtmlPreviewPublishRequest,
): Promise<string> {
  const response = await publishHtmlPreview(sessionId, payload);
  return buildHtmlPreviewProxyUrl(sessionId, response);
}

export function useHtmlPreviewPublisher(sessionId: string | null) {
  const [state, setState] = useState<HtmlPreviewPublishState>(IDLE_STATE);
  const requestIdRef = useRef(0);

  useEffect(() => {
    requestIdRef.current += 1;
    setState(IDLE_STATE);
  }, [sessionId]);

  const reset = useCallback(() => {
    requestIdRef.current += 1;
    setState(IDLE_STATE);
  }, []);

  const publish = useCallback(
    async (payload: HtmlPreviewPublishRequest): Promise<string | null> => {
      const requestId = requestIdRef.current + 1;
      requestIdRef.current = requestId;
      if (!sessionId) {
        setState({ status: "error", url: null, error: "session-unavailable" });
        return null;
      }

      setState({ status: "publishing", url: null, error: null });
      try {
        const url = await publishHtmlPreviewUrl(sessionId, payload);
        if (requestIdRef.current !== requestId) return null;
        setState({ status: "ready", url, error: null });
        return url;
      } catch (error) {
        if (requestIdRef.current !== requestId) return null;
        setState({
          status: "error",
          url: null,
          error: getHtmlPreviewPublishErrorCode(error),
        });
        return null;
      }
    },
    [sessionId],
  );

  return {
    ...state,
    isPublishing: state.status === "publishing",
    publish,
    reset,
  };
}
