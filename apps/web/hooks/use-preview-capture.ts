"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { usePreviewFeedback } from "@/hooks/domains/comments/use-preview-feedback";
import {
  sendCaptureSelection,
  sendProjectPreviewMarkers,
  type PreviewCaptureMode,
  type PreviewMarkerProjection,
  type PreviewRegionAnchor,
  type PreviewRegionLayout,
} from "@/lib/preview-inspect-bridge";
import { isScreenshotLimitReached, nextEscapeStep } from "@/lib/preview-annotation";
import { sanitizePreviewPageRoute } from "@/lib/preview-feedback-source";
import { anchorBelowRect, floatingBounds } from "@/components/task/floating-selection-position";
import type { PreviewCaptureRect, TaskPreviewFeedback } from "@/lib/types/http";
import { usePreviewAnnotationMode } from "./use-preview-annotation-mode";
import { usePreviewDraftSave, type PreviewCaptureSource } from "./use-preview-draft-save";
import { usePreviewDrafts, type PreviewDraftEntry } from "./use-preview-drafts";
import { usePreviewInspectorEvents } from "./use-preview-inspector-events";
import { usePreviewScreenshots } from "./use-preview-screenshots";

export type { PreviewCaptureSource } from "./use-preview-draft-save";
export type {
  PreviewDraftEntry,
  PreviewFeedbackDraft,
  PreviewScreenshotDraft,
} from "./use-preview-drafts";
export type { PreviewCaptureError } from "./use-preview-screenshots";

type UsePreviewCaptureOptions = {
  taskId: string | null | undefined;
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  source: PreviewCaptureSource;
  enabled: boolean;
};

/** A screenshot region drawn over the preview, in iframe-viewport CSS pixels. */
export type PreviewRegionView = {
  id: string;
  index: number;
  rect: { x: number; y: number; width: number; height: number };
  pending: boolean;
  title?: string;
};

function belongsToSource(item: TaskPreviewFeedback, source: PreviewCaptureSource) {
  return (
    item.source_kind === source.kind &&
    (item.source_session_id ?? undefined) === source.sessionId &&
    (item.source_path ?? undefined) === source.path &&
    item.source_label === source.label
  );
}

function itemMarker(item: TaskPreviewFeedback): PreviewMarkerProjection {
  return {
    id: item.id,
    kind: item.kind,
    page_route: sanitizePreviewPageRoute(item.page_route),
    text_anchor: item.text_anchor,
    element_snapshot: item.element_snapshot,
    capture_rect: item.capture_rect,
  };
}

function draftMarker(draft: PreviewDraftEntry): PreviewMarkerProjection {
  return {
    id: draft.id,
    kind: draft.payload.kind,
    page_route: draft.payload.page_route,
    text_anchor: draft.payload.text_anchor,
    element_snapshot: draft.payload.element_snapshot,
    capture_rect: draft.payload.capture_rect,
    pending: true,
  };
}

type RegionSource = { id: string; route: string; rect: PreviewCaptureRect; pending: boolean };

function regionSources(items: TaskPreviewFeedback[], drafts: PreviewDraftEntry[]) {
  const saved: RegionSource[] = items.flatMap((item) =>
    item.kind === "screenshot" && item.capture_rect
      ? [
          {
            id: item.id,
            route: sanitizePreviewPageRoute(item.page_route),
            rect: item.capture_rect,
            pending: false,
          },
        ]
      : [],
  );
  const unsaved: RegionSource[] = drafts.flatMap((draft) =>
    draft.payload.kind === "screenshot"
      ? [
          {
            id: draft.id,
            route: draft.payload.page_route,
            rect: draft.payload.capture_rect,
            pending: true,
          },
        ]
      : [],
  );
  return [...saved, ...unsaved];
}

function regionViews(
  sources: RegionSource[],
  route: string,
  layouts: Record<string, PreviewRegionLayout>,
  comments: Record<string, string>,
): PreviewRegionView[] {
  return sources
    .filter((region) => region.route === route)
    .flatMap((region, index) => {
      const layout = layouts[region.id];
      if (layout && !layout.visible) return [];
      const { x, y, width, height } = layout?.rect ?? region.rect;
      return [
        {
          id: region.id,
          index,
          rect: { x, y, width, height },
          pending: region.pending,
          title: comments[region.id],
        },
      ];
    });
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

function lastTextRect(draft: PreviewDraftEntry["payload"]) {
  const rects = draft.text_anchor?.rects;
  return rects?.length ? rects[rects.length - 1] : null;
}

/** Viewport point below a rectangle in iframe coordinates, clamped to the visible iframe. */
function anchorBelow(iframe: HTMLIFrameElement | null, rect: PreviewRegionView["rect"] | null) {
  if (!iframe || !rect) return null;
  const bounds = iframe.getBoundingClientRect();
  return anchorBelowRect(rect, bounds, floatingBounds(bounds));
}

function useSourceCollections(
  feedback: ReturnType<typeof usePreviewFeedback>,
  drafts: PreviewDraftEntry[],
  source: PreviewCaptureSource,
) {
  const sourceItems = useMemo(
    () => feedback.items.filter((item) => belongsToSource(item, source)),
    [feedback.items, source.kind, source.label, source.path, source.sessionId],
  );
  const markers = useMemo(
    () => [...sourceItems.map(itemMarker), ...drafts.map(draftMarker)],
    [drafts, sourceItems],
  );
  const sources = useMemo(() => regionSources(sourceItems, drafts), [drafts, sourceItems]);
  const savedScreenshots = feedback.items.filter((item) => item.kind === "screenshot").length;
  const unsavedScreenshots = drafts.filter((draft) => draft.payload.kind === "screenshot").length;
  return {
    sourceItems,
    markers,
    sources,
    limitReached: isScreenshotLimitReached(savedScreenshots, unsavedScreenshots),
  };
}

function useResetDrafts(
  drafts: ReturnType<typeof usePreviewDrafts>,
  enabled: boolean,
  resetKey: string,
) {
  const { clear } = drafts;
  useEffect(() => {
    if (!enabled) clear();
  }, [clear, enabled]);
  useEffect(() => clear, [clear, resetKey]);
}

function usePreviewPageState() {
  const [pageRoute, setPageRoute] = useState("");
  const [pageTitle, setPageTitle] = useState("");
  const [candidateLabel, setCandidateLabel] = useState<string | null>(null);
  const [acknowledgedMode, setAcknowledgedMode] = useState<PreviewCaptureMode | null>(null);
  const [selectionRect, setSelectionRect] = useState<PreviewCaptureRect | null>(null);
  const [layouts, setLayouts] = useState<Record<string, PreviewRegionLayout>>({});
  return {
    pageRoute,
    pageTitle,
    candidateLabel,
    acknowledgedMode,
    selectionRect,
    layouts,
    setCandidateLabel,
    setAcknowledgedMode,
    setSelectionRect,
    setLayouts,
    changePage: (route: string, title: string) => {
      setPageRoute(route);
      setPageTitle(title);
      setSelectionRect(null);
      setCandidateLabel(null);
    },
  };
}

/** Which comment surface is open: a draft, a saved region's editor, or the pending panel. */
function usePreviewSurfaces(
  drafts: ReturnType<typeof usePreviewDrafts>,
  annotation: ReturnType<typeof usePreviewAnnotationMode>,
  sourceItems: TaskPreviewFeedback[],
) {
  const [editingItemId, setEditingItemId] = useState<string | null>(null);
  const [pendingPanelOpen, setPendingPanelOpen] = useState(false);
  const editingItem = sourceItems.find((item) => item.id === editingItemId) ?? null;
  const surfaceOpen = drafts.openDraft !== null || editingItem !== null || pendingPanelOpen;

  const handleEscape = useCallback(() => {
    const step = nextEscapeStep({ surfaceOpen, tool: annotation.tool });
    if (step === "close-surface") {
      drafts.close();
      setEditingItemId(null);
      setPendingPanelOpen(false);
    } else if (step === "interact") {
      annotation.setTool("interact");
    } else {
      annotation.exit();
    }
  }, [annotation, drafts, surfaceOpen]);

  const openRegion = useCallback(
    (id: string) => {
      setPendingPanelOpen(false);
      if (drafts.find(id)) {
        setEditingItemId(null);
        drafts.open(id);
      } else {
        drafts.close();
        setEditingItemId(id);
      }
    },
    [drafts],
  );

  return {
    editingItem,
    closeItemEditor: () => setEditingItemId(null),
    pendingPanelOpen,
    setPendingPanelOpen,
    handleEscape,
    openRegion,
  };
}

function useRegionProjection(
  iframeRef: React.RefObject<HTMLIFrameElement | null>,
  collections: ReturnType<typeof useSourceCollections>,
  drafts: PreviewDraftEntry[],
  page: ReturnType<typeof usePreviewPageState>,
  active: boolean,
) {
  const { markers, sources, sourceItems } = collections;
  const anchors = useMemo<PreviewRegionAnchor[]>(
    () =>
      active
        ? sources.map((region) => ({
            id: region.id,
            page_route: region.route,
            capture_rect: region.rect,
          }))
        : [],
    [active, sources],
  );
  const projectMarkers = useCallback(() => {
    if (iframeRef.current) sendProjectPreviewMarkers(iframeRef.current, markers, anchors);
  }, [anchors, iframeRef, markers]);
  useEffect(projectMarkers, [projectMarkers]);

  const comments = useMemo(
    () =>
      Object.fromEntries([
        ...sourceItems.map((item) => [item.id, item.comment] as const),
        ...drafts.map((draft) => [draft.id, draft.comment] as const),
      ]),
    [drafts, sourceItems],
  );
  const regions = useMemo(
    () => (active ? regionViews(sources, page.pageRoute, page.layouts, comments) : []),
    [active, comments, page.layouts, page.pageRoute, sources],
  );
  return { projectMarkers, regions };
}

function openDraftRect(openDraft: PreviewDraftEntry | null, regions: PreviewRegionView[]) {
  if (!openDraft) return null;
  return (
    regions.find((region) => region.id === openDraft.id)?.rect ??
    openDraft.payload.capture_rect ??
    lastTextRect(openDraft.payload)
  );
}

type CaptureEventDeps = {
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  enabled: boolean;
  page: ReturnType<typeof usePreviewPageState>;
  annotation: ReturnType<typeof usePreviewAnnotationMode>;
  drafts: ReturnType<typeof usePreviewDrafts>;
  surfaces: ReturnType<typeof usePreviewSurfaces>;
  screenshots: ReturnType<typeof usePreviewScreenshots>;
  projectMarkers: () => void;
  limitReached: boolean;
};

function useCaptureEvents(deps: CaptureEventDeps) {
  const { page, annotation, drafts, surfaces, screenshots } = deps;
  usePreviewInspectorEvents({
    iframeRef: deps.iframeRef,
    enabled: deps.enabled,
    onPageChanged: (route, title, bridgeReloaded) => {
      page.changePage(route, title);
      deps.projectMarkers();
      if (bridgeReloaded) annotation.resend();
    },
    onCandidate: page.setCandidateLabel,
    onModeAcknowledged: (mode) => {
      page.setAcknowledgedMode(mode);
      if (mode === null) page.setCandidateLabel(null);
    },
    onCapture: (draft) => {
      surfaces.closeItemEditor();
      drafts.add(draft);
    },
    onRegionSelected: (region) => {
      if (deps.limitReached) return;
      surfaces.closeItemEditor();
      screenshots.captureRegion(region);
    },
    onSelectionChanged: page.setSelectionRect,
    onRegionLayout: (regions) =>
      page.setLayouts(Object.fromEntries(regions.map((region) => [region.id, region]))),
    onEscape: surfaces.handleEscape,
  });
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
  const drafts = usePreviewDrafts();
  const page = usePreviewPageState();
  const collections = useSourceCollections(feedback, drafts.drafts, source);
  const annotation = usePreviewAnnotationMode({
    iframeRef,
    enabled,
    regionBlocked: collections.limitReached,
  });
  useResetDrafts(
    drafts,
    enabled,
    [taskId, source.kind, source.label, source.path, source.sessionId].join("\u0000"),
  );
  const screenshots = usePreviewScreenshots({
    iframeRef,
    drafts,
    workspaceId,
    items: collections.sourceItems,
    replaceSaved: feedback.replaceScreenshot,
  });
  const save = usePreviewDraftSave({
    drafts,
    workspaceId,
    source,
    createFeedback: feedback.create,
    setCaptureError: screenshots.setCaptureError,
  });
  const projection = useRegionProjection(
    iframeRef,
    collections,
    drafts.drafts,
    page,
    annotation.active,
  );
  const surfaces = usePreviewSurfaces(drafts, annotation, collections.sourceItems);

  useCaptureEvents({
    iframeRef,
    enabled,
    page,
    annotation,
    drafts,
    surfaces,
    screenshots,
    projectMarkers: projection.projectMarkers,
    limitReached: collections.limitReached,
  });

  const { regions } = projection;
  const openDraft = drafts.openDraft;
  const editingRect = regions.find((region) => region.id === surfaces.editingItem?.id)?.rect;
  return {
    ...feedback,
    ...surfaces,
    annotation,
    acknowledgedMode: page.acknowledgedMode,
    candidateLabel: page.candidateLabel,
    pageRoute: page.pageRoute,
    pageTitle: page.pageTitle,
    markers: collections.markers,
    drafts: drafts.drafts,
    openDraft,
    draftAnchor: anchorBelow(iframeRef.current, openDraftRect(openDraft, regions)),
    setDraftComment: (comment: string) => {
      if (openDraft) drafts.setComment(openDraft.id, comment);
    },
    closeDraftSurface: drafts.close,
    resumeDraft: surfaces.openRegion,
    discardDraft: drafts.discard,
    saveDraft: save.saveDraft,
    editingAnchor: anchorBelow(iframeRef.current, editingRect ?? null),
    regions,
    recaptureRegion: screenshots.recaptureRegion,
    recaptureFailed: screenshots.recaptureFailed,
    limitReached: collections.limitReached,
    selectionRect: enabled ? page.selectionRect : null,
    captureSelection: () => {
      if (iframeRef.current) sendCaptureSelection(iframeRef.current);
    },
    captureError: screenshots.captureError,
    isRasterizing: screenshots.isRasterizing,
    isUploading: save.isUploading,
    handleIframeLoad: () => {
      page.setLayouts({});
      annotation.resend();
      projection.projectMarkers();
    },
  };
}

export type PreviewCaptureController = ReturnType<typeof usePreviewCapture>;
