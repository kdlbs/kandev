"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import html2canvas from "html2canvas-pro";
import { useAppStore } from "@/components/state-provider";
import { usePreviewFeedback } from "@/hooks/domains/comments/use-preview-feedback";
import { deleteAttachment, uploadAttachment } from "@/lib/api/domains/attachment-api";
import {
  isInspectorMessage,
  sendProjectPreviewMarkers,
  sendSetPreviewCaptureMode,
  type PreviewCaptureDraft,
  type PreviewCaptureMode,
  type PreviewMarkerProjection,
  type PreviewScreenshotRegion,
} from "@/lib/preview-inspect-bridge";
import { rasterizePreviewRegion } from "@/lib/preview-screenshot";
import type { TaskPreviewFeedback } from "@/lib/types/http";

export type PreviewCaptureSource = {
  kind: "browser" | "html_file";
  label: string;
  sessionId?: string;
  path?: string;
};

export type PreviewScreenshotDraft = PreviewScreenshotRegion & {
  kind: "screenshot";
  selected_text?: undefined;
  text_anchor?: undefined;
  element_snapshot?: undefined;
  screenshot: {
    blob: Blob;
    width: number;
    height: number;
    previewUrl: string;
    fileName: string;
    attachmentId?: string;
  };
};

export type PreviewFeedbackDraft = PreviewCaptureDraft | PreviewScreenshotDraft;
export type PreviewCaptureError = "raster" | "upload" | "workspace";

type UsePreviewCaptureOptions = {
  taskId: string | null | undefined;
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  source: PreviewCaptureSource;
  enabled: boolean;
};

type InspectorEventOptions = {
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  enabled: boolean;
  projectMarkers: () => void;
  setMode: React.Dispatch<React.SetStateAction<PreviewCaptureMode | null>>;
  setDraft: (draft: PreviewFeedbackDraft) => void;
  setCandidateLabel: React.Dispatch<React.SetStateAction<string | null>>;
  setPageRoute: React.Dispatch<React.SetStateAction<string>>;
  setPageTitle: React.Dispatch<React.SetStateAction<string>>;
  captureScreenshot: (region: PreviewScreenshotRegion) => void;
};

function usePreviewInspectorEvents(options: InspectorEventOptions) {
  useEffect(() => {
    function handleMessage(event: MessageEvent) {
      if (
        event.source !== options.iframeRef.current?.contentWindow ||
        !isInspectorMessage(event.data)
      ) {
        return;
      }
      const message = event.data;
      switch (message.type) {
        case "inspector-ready":
        case "route-changed":
          options.setPageRoute(message.payload.page_route);
          options.setPageTitle(message.payload.page_title);
          options.projectMarkers();
          break;
        case "candidate-changed":
          options.setCandidateLabel(message.payload.label);
          break;
        case "capture-completed":
          if (!options.enabled) return;
          options.setDraft(message.payload);
          options.setMode(null);
          options.setCandidateLabel(null);
          break;
        case "capture-cancelled":
          options.setMode(null);
          options.setCandidateLabel(null);
          break;
        case "screenshot-region-selected":
          if (options.enabled) options.captureScreenshot(message.payload);
          break;
        default:
          break;
      }
    }
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [options]);
}

function belongsToSource(item: TaskPreviewFeedback, source: PreviewCaptureSource) {
  return (
    item.source_kind === source.kind &&
    (item.source_session_id ?? undefined) === source.sessionId &&
    (item.source_path ?? undefined) === source.path &&
    item.source_label === source.label
  );
}

function markerProjection(item: TaskPreviewFeedback): PreviewMarkerProjection {
  return {
    id: item.id,
    kind: item.kind,
    page_route: item.page_route,
    text_anchor: item.text_anchor,
    element_snapshot: item.element_snapshot,
    capture_rect: item.capture_rect,
  };
}

function taskWorkspaceID(
  taskID: string | null | undefined,
  state: Parameters<Parameters<typeof useAppStore>[0]>[0],
) {
  if (!taskID) return state.workspaces.activeId ?? null;
  const task =
    state.kanban.tasks.find((item) => item.id === taskID) ??
    Object.values(state.kanbanMulti.snapshots)
      .flatMap((snapshot) => snapshot.tasks)
      .find((item) => item.id === taskID);
  return task?.workspaceId ?? state.workspaces.activeId ?? null;
}

function releaseScreenshotDraft(draft: PreviewFeedbackDraft | null) {
  if (draft?.kind !== "screenshot") return;
  URL.revokeObjectURL(draft.screenshot.previewUrl);
  if (draft.screenshot.attachmentId) {
    void deleteAttachment(draft.screenshot.attachmentId).catch(() => undefined);
  }
}

function usePreviewDraftState(iframeRef: React.RefObject<HTMLIFrameElement | null>) {
  const [draft, setDraft] = useState<PreviewFeedbackDraft | null>(null);
  const [draftComment, setDraftComment] = useState("");
  const [captureError, setCaptureError] = useState<PreviewCaptureError | null>(null);
  const [isRasterizing, setIsRasterizing] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
  const generationRef = useRef(0);
  const draftRef = useRef<PreviewFeedbackDraft | null>(null);

  const setNewDraft = useCallback((next: PreviewFeedbackDraft) => {
    releaseScreenshotDraft(draftRef.current);
    draftRef.current = next;
    setDraft(next);
    setDraftComment("");
    generationRef.current += 1;
  }, []);

  const updateDraft = useCallback((next: React.SetStateAction<PreviewFeedbackDraft | null>) => {
    const current = draftRef.current;
    const updated = typeof next === "function" ? next(current) : next;
    draftRef.current = updated;
    setDraft(updated);
  }, []);

  const captureScreenshot = useCallback(
    (region: PreviewScreenshotRegion) => {
      const documentElement = iframeRef.current?.contentDocument?.documentElement;
      if (!documentElement) {
        setCaptureError("raster");
        return;
      }
      const generation = ++generationRef.current;
      setCaptureError(null);
      setIsRasterizing(true);
      void rasterizePreviewRegion(documentElement, region.capture_rect, (element, options) =>
        html2canvas(element, options),
      )
        .then((image) => {
          if (generationRef.current !== generation) return;
          const previewUrl = URL.createObjectURL(image.blob);
          setIsRasterizing(false);
          setNewDraft({
            ...region,
            kind: "screenshot",
            screenshot: {
              ...image,
              previewUrl,
              fileName: `preview-${Date.now()}.png`,
            },
          });
        })
        .catch(() => {
          if (generationRef.current === generation) setCaptureError("raster");
        })
        .finally(() => {
          if (generationRef.current === generation) setIsRasterizing(false);
        });
    },
    [iframeRef, setNewDraft],
  );

  const clearForCapture = useCallback(() => {
    generationRef.current += 1;
    releaseScreenshotDraft(draftRef.current);
    draftRef.current = null;
    setDraft(null);
    setDraftComment("");
    setCaptureError(null);
    setIsRasterizing(false);
  }, []);

  const discardDraft = clearForCapture;

  const completeDraft = useCallback((savedGeneration: number) => {
    if (generationRef.current !== savedGeneration || !draftRef.current) return false;
    if (draftRef.current.kind === "screenshot") {
      URL.revokeObjectURL(draftRef.current.screenshot.previewUrl);
    }
    draftRef.current = null;
    setDraft(null);
    setDraftComment("");
    setCaptureError(null);
    generationRef.current += 1;
    return true;
  }, []);

  return {
    draft,
    draftComment,
    setDraftComment,
    setDraft: setNewDraft,
    updateDraft,
    captureError,
    setCaptureError,
    isRasterizing,
    isUploading,
    setIsUploading,
    generationRef,
    captureScreenshot,
    clearForCapture,
    discardDraft,
    completeDraft,
  };
}

type SaveDraftOptions = {
  draftState: ReturnType<typeof usePreviewDraftState>;
  workspaceId: string | null;
  source: PreviewCaptureSource;
  createFeedback: ReturnType<typeof usePreviewFeedback>["create"];
  projectMarkers: () => void;
};

function useSavePreviewDraft({
  draftState,
  workspaceId,
  source,
  createFeedback,
  projectMarkers,
}: SaveDraftOptions) {
  return useCallback(
    async (comment: string) => {
      const draft = draftState.draft;
      if (!draft || !comment.trim()) return false;
      const saveGeneration = draftState.generationRef.current;
      let screenshotAttachmentId: string | undefined;
      if (draft.kind === "screenshot") {
        if (!workspaceId) {
          draftState.setCaptureError("workspace");
          return false;
        }
        screenshotAttachmentId = draft.screenshot.attachmentId;
        if (!screenshotAttachmentId) {
          draftState.setIsUploading(true);
          draftState.setCaptureError(null);
          try {
            const file = new File([draft.screenshot.blob], draft.screenshot.fileName, {
              type: "image/png",
            });
            const uploaded = await uploadAttachment(file, {
              workspaceId,
              kind: "image",
              deliveryMode: "prompt",
            });
            screenshotAttachmentId = uploaded.attachment_id;
            if (draftState.generationRef.current !== saveGeneration) {
              void deleteAttachment(uploaded.attachment_id).catch(() => undefined);
              return false;
            }
            draftState.updateDraft((current) =>
              current?.kind === "screenshot"
                ? {
                    ...current,
                    screenshot: { ...current.screenshot, attachmentId: uploaded.attachment_id },
                  }
                : current,
            );
          } catch {
            draftState.setCaptureError("upload");
            return false;
          } finally {
            draftState.setIsUploading(false);
          }
        }
      }
      const result = await createFeedback({
        kind: draft.kind,
        comment: comment.trim(),
        sourceKind: source.kind,
        sourceSessionId: source.sessionId,
        sourceLabel: source.label,
        sourcePath: source.path,
        pageRoute: draft.page_route,
        pageTitle: draft.page_title,
        selectedText: draft.selected_text,
        textAnchor: draft.text_anchor,
        elementSnapshot: draft.element_snapshot,
        captureRect: draft.capture_rect,
        screenshotAttachmentId,
      });
      if (!result) return false;
      if (draftState.completeDraft(saveGeneration)) projectMarkers();
      return true;
    },
    [createFeedback, draftState, projectMarkers, source, workspaceId],
  );
}

/** Connects one preview iframe to the task-owned pending feedback collection. */
export function usePreviewCapture({
  taskId,
  iframeRef,
  source,
  enabled,
}: UsePreviewCaptureOptions) {
  const feedback = usePreviewFeedback(taskId);
  const workspaceId = useAppStore((state) => taskWorkspaceID(taskId, state));
  const [mode, setMode] = useState<PreviewCaptureMode | null>(null);
  const [candidateLabel, setCandidateLabel] = useState<string | null>(null);
  const [pageRoute, setPageRoute] = useState("");
  const [pageTitle, setPageTitle] = useState("");
  const draftState = usePreviewDraftState(iframeRef);

  const markers = useMemo(
    () => feedback.items.filter((item) => belongsToSource(item, source)).map(markerProjection),
    [feedback.items, source.kind, source.label, source.path, source.sessionId],
  );

  const projectMarkers = useCallback(() => {
    if (iframeRef.current) sendProjectPreviewMarkers(iframeRef.current, markers);
  }, [iframeRef, markers]);

  useEffect(() => {
    projectMarkers();
  }, [projectMarkers]);

  useEffect(() => {
    if (enabled) return;
    setMode(null);
    setCandidateLabel(null);
    if (iframeRef.current) sendSetPreviewCaptureMode(iframeRef.current, null);
  }, [enabled, iframeRef]);

  usePreviewInspectorEvents({
    iframeRef,
    enabled,
    projectMarkers,
    setMode,
    setDraft: draftState.setDraft,
    setCandidateLabel,
    setPageRoute,
    setPageTitle,
    captureScreenshot: (region) => {
      setMode(null);
      draftState.captureScreenshot(region);
    },
  });

  const startCapture = useCallback(
    (nextMode: PreviewCaptureMode) => {
      if (!enabled || !iframeRef.current) return;
      draftState.clearForCapture();
      setMode(nextMode);
      sendSetPreviewCaptureMode(iframeRef.current, nextMode);
    },
    [draftState, enabled, iframeRef],
  );

  const cancelCapture = useCallback(() => {
    setMode(null);
    setCandidateLabel(null);
    if (iframeRef.current) sendSetPreviewCaptureMode(iframeRef.current, null);
  }, [iframeRef]);

  const saveDraft = useSavePreviewDraft({
    draftState,
    workspaceId,
    source,
    createFeedback: feedback.create,
    projectMarkers,
  });

  const handleIframeLoad = useCallback(() => {
    if (!iframeRef.current) return;
    sendSetPreviewCaptureMode(iframeRef.current, enabled ? mode : null);
    sendProjectPreviewMarkers(iframeRef.current, markers);
  }, [enabled, iframeRef, markers, mode]);

  return {
    ...feedback,
    mode,
    draft: draftState.draft,
    draftComment: draftState.draftComment,
    setDraftComment: draftState.setDraftComment,
    candidateLabel,
    captureError: draftState.captureError,
    isRasterizing: draftState.isRasterizing,
    isUploading: draftState.isUploading,
    pageRoute,
    pageTitle,
    markers,
    startCapture,
    cancelCapture,
    discardDraft: draftState.discardDraft,
    saveDraft,
    handleIframeLoad,
  };
}
