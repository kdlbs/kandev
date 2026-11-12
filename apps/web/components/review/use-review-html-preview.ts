"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import {
  useHtmlPreviewPublisher,
  type HtmlPreviewPublishErrorCode,
} from "@/hooks/use-html-preview-publisher";
import type { HtmlPreviewPublishRequest } from "@/lib/api/domains/process-api";
import { isReviewHtmlPreviewEligible, type ReviewFile } from "./types";
import type { ReviewHtmlDiffPreviewProps } from "./review-html-diff-preview";

type ReviewHtmlPreviewFile = Pick<
  ReviewFile,
  "path" | "status" | "diff_skip_reason" | "source" | "repository_name" | "repository_id"
>;

export type UseReviewHtmlPreviewArgs = {
  file: ReviewHtmlPreviewFile;
  sessionId: string;
  taskId?: string | null;
  suppressAutoMark?: RefObject<boolean>;
};

export type ReviewHtmlPreviewState = {
  canPreview: boolean;
  isActive: boolean;
  isLoading: boolean;
  previewUrl: string | null;
  error: HtmlPreviewPublishErrorCode | null;
  readError: boolean;
  previewProps: ReviewHtmlDiffPreviewProps | null;
  toggle: () => void;
  refresh: () => void;
  retry: () => void;
  showDiff: () => void;
};

type PreviewPublisherState = Pick<
  ReturnType<typeof useHtmlPreviewPublisher>,
  "url" | "error" | "isPublishing"
>;

function buildPreviewProps(args: {
  file: ReviewHtmlPreviewFile;
  sessionId: string;
  taskId?: string | null;
  repo?: string;
  publisher: PreviewPublisherState;
  isReading: boolean;
  readError: boolean;
  load: () => void;
  showDiff: () => void;
}): ReviewHtmlDiffPreviewProps {
  const { file, sessionId, taskId, repo, publisher, isReading, readError, load, showDiff } = args;
  return {
    path: file.path,
    sessionId,
    taskId,
    repositoryId: file.repository_id,
    repositoryName: repo,
    previewUrl: publisher.url,
    isLoading: isReading || publisher.isPublishing,
    error: publisher.error,
    readError,
    onRefresh: load,
    onRetry: load,
    onShowDiff: showDiff,
  };
}

type ReadOutcome = "ready" | "stale" | "failed";

async function readAndPublishCurrentFile(args: {
  sessionId: string;
  path: string;
  repo?: string;
  publish: (payload: HtmlPreviewPublishRequest) => Promise<unknown>;
  isCurrent: () => boolean;
}): Promise<ReadOutcome> {
  const client = getWebSocketClient();
  if (!client) return "failed";
  const response = await requestFileContent(client, args.sessionId, args.path, args.repo);
  if (!args.isCurrent()) return "stale";
  if (response.error || response.is_binary) return "failed";
  await args.publish({
    path: args.path,
    content: response.content,
    ...(args.repo ? { repo: args.repo } : {}),
  });
  return args.isCurrent() ? "ready" : "stale";
}

/**
 * Owns one diff row's transient native HTML preview.
 *
 * The preview always publishes the complete current workspace file, never the
 * selected patch. Reads and publications carry a request generation so a
 * dismissed or replaced row cannot publish late content into another preview.
 */
export function useReviewHtmlPreview({
  file,
  sessionId,
  taskId,
  suppressAutoMark,
}: UseReviewHtmlPreviewArgs): ReviewHtmlPreviewState {
  const publisher = useHtmlPreviewPublisher(sessionId);
  const [previewIdentity, setPreviewIdentity] = useState<string | null>(null);
  const [isReading, setIsReading] = useState(false);
  const [readError, setReadError] = useState(false);
  const generationRef = useRef(0);
  const repo = file.repository_name;
  const identity = [sessionId, repo ?? "", file.path, file.source].join("\u0000");
  const canPreview = isReviewHtmlPreviewEligible(file);
  const isActive = previewIdentity === identity;

  useEffect(() => {
    generationRef.current += 1;
    setPreviewIdentity(null);
    setIsReading(false);
    setReadError(false);
    publisher.reset();
  }, [identity, publisher.reset]);

  useEffect(() => {
    return () => {
      generationRef.current += 1;
      publisher.reset();
    };
  }, [publisher.reset]);

  const load = useCallback(() => {
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    setIsReading(true);
    setReadError(false);
    const settle = (outcome: ReadOutcome) => {
      if (outcome === "stale") return;
      setIsReading(false);
      if (outcome === "failed") setReadError(true);
    };
    void readAndPublishCurrentFile({
      sessionId,
      path: file.path,
      repo,
      publish: publisher.publish,
      isCurrent: () => generationRef.current === generation,
    })
      .then(settle)
      .catch(() => {
        if (generationRef.current === generation) settle("failed");
      });
  }, [file.path, repo, sessionId, publisher.publish]);

  const showDiff = useCallback(() => {
    generationRef.current += 1;
    setPreviewIdentity(null);
    setIsReading(false);
    setReadError(false);
    if (suppressAutoMark) suppressAutoMark.current = true;
    publisher.reset();
  }, [publisher.reset, suppressAutoMark]);

  const start = useCallback(() => {
    if (!canPreview) return;
    if (suppressAutoMark) suppressAutoMark.current = true;
    setPreviewIdentity(identity);
    load();
  }, [canPreview, identity, load, suppressAutoMark]);

  const toggle = useCallback(() => {
    if (isActive) showDiff();
    else start();
  }, [isActive, showDiff, start]);

  const previewProps = isActive
    ? buildPreviewProps({
        file,
        sessionId,
        taskId,
        repo,
        publisher,
        isReading,
        readError,
        load,
        showDiff,
      })
    : null;

  return {
    canPreview,
    isActive,
    isLoading: previewProps?.isLoading ?? false,
    previewUrl: previewProps?.previewUrl ?? null,
    error: previewProps?.error ?? null,
    readError: previewProps?.readError ?? false,
    previewProps,
    toggle,
    refresh: load,
    retry: load,
    showDiff,
  };
}
