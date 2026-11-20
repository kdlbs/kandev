"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { usePreviewFeedback } from "@/hooks/domains/comments/use-preview-feedback";
import {
  sendCaptureSelection,
  sendProjectPreviewMarkers,
  type PreviewCaptureMode,
  type PreviewHighlightedLayout,
  type PreviewMarkerProjection,
  type PreviewRegionAnchor,
  type PreviewRegionLayout,
} from "@/lib/preview-inspect-bridge";
import {
  captureNumbers,
  closeCommentStep,
  type PreviewAnnotationTool,
  isScreenshotLimitReached,
  nextEscapeStep,
} from "@/lib/preview-annotation";
import { sanitizePreviewPageRoute } from "@/lib/preview-feedback-source";
import { anchorBelowRect, floatingBounds } from "@/components/task/floating-selection-position";
import type { PreviewCaptureRect, TaskPreviewFeedback } from "@/lib/types/http";
import { usePreviewAnnotationMode } from "./use-preview-annotation-mode";
import { usePreviewCommentPanel } from "./use-preview-comment-panel";
import { usePreviewDraftSave, type PreviewCaptureSource } from "./use-preview-draft-save";
import { usePreviewDrafts, type PreviewDraftEntry } from "./use-preview-drafts";
import { usePreviewInspectorEvents } from "./use-preview-inspector-events";
import { usePreviewScreenshots } from "./use-preview-screenshots";
import {
  editingItemRect,
  openDraftRect,
  regionSources,
  regionViews,
  type PreviewRegionView,
} from "./preview-region-views";

export type { PreviewCaptureSource } from "./use-preview-draft-save";
export type {
  PreviewDraftEntry,
  PreviewFeedbackDraft,
  PreviewScreenshotDraft,
} from "./use-preview-drafts";
export type { PreviewCaptureError } from "./use-preview-screenshots";
export type { PreviewRegionView } from "./preview-region-views";

type UsePreviewCaptureOptions = {
  taskId: string | null | undefined;
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  source: PreviewCaptureSource;
  enabled: boolean;
};

function belongsToSource(item: TaskPreviewFeedback, source: PreviewCaptureSource) {
  return (
    item.source_kind === source.kind &&
    (item.source_session_id ?? undefined) === source.sessionId &&
    (item.source_path ?? undefined) === source.path &&
    item.source_label === source.label
  );
}

/** Enough of a comment's text for the page to recognize it again. */
const MARKER_TEXT_LIMIT = 4096;

function itemMarker(item: TaskPreviewFeedback): PreviewMarkerProjection {
  return {
    id: item.id,
    kind: item.kind,
    page_route: sanitizePreviewPageRoute(item.page_route),
    text_anchor: item.text_anchor,
    element_snapshot: item.element_snapshot,
    capture_rect: item.capture_rect,
    selected_text: item.selected_text?.slice(0, MARKER_TEXT_LIMIT),
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
    selected_text: draft.payload.selected_text?.slice(0, MARKER_TEXT_LIMIT),
    pending: true,
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

/** Viewport point below a rectangle in iframe coordinates, clamped to the visible iframe. */
function anchorBelow(iframe: HTMLIFrameElement | null, rect: PreviewRegionView["rect"] | null) {
  if (!iframe || !rect) return null;
  const bounds = iframe.getBoundingClientRect();
  return anchorBelowRect(rect, bounds, floatingBounds(bounds));
}

/** Where the open draft's and the saved item's comment boxes are anchored. */
type CommentAnchorPage = {
  highlighted: PreviewHighlightedLayout | null;
  activated: PreviewHighlightedLayout | null;
};

function commentAnchors(
  iframeRef: React.RefObject<HTMLIFrameElement | null>,
  page: CommentAnchorPage,
  regions: PreviewRegionView[],
  openDraft: PreviewDraftEntry | null,
  editingItem: TaskPreviewFeedback | null,
) {
  const { highlighted, activated } = page;
  const iframe = iframeRef.current;
  const editingRect = editingItemRect(editingItem, regions, highlighted, activated);
  return {
    draftAnchor: anchorBelow(iframe, openDraftRect(openDraft, regions, activated)),
    editingAnchor: anchorBelow(iframe, editingRect),
  };
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
  const numbers = useMemo(
    () =>
      captureNumbers([
        ...sourceItems.map((item) => ({
          id: item.id,
          route: sanitizePreviewPageRoute(item.page_route),
        })),
        ...drafts.map((draft) => ({ id: draft.id, route: draft.payload.page_route })),
      ]),
    [drafts, sourceItems],
  );
  const markers = useMemo(
    () =>
      [...sourceItems.map(itemMarker), ...drafts.map(draftMarker)].map((marker) => ({
        ...marker,
        number: numbers.get(marker.id),
      })),
    [drafts, numbers, sourceItems],
  );
  const sources = useMemo(() => regionSources(sourceItems, drafts), [drafts, sourceItems]);
  const savedScreenshots = feedback.items.filter((item) => item.kind === "screenshot").length;
  const unsavedScreenshots = drafts.filter((draft) => draft.payload.kind === "screenshot").length;
  return {
    sourceItems,
    numbers,
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
  const [highlighted, setHighlighted] = useState<PreviewHighlightedLayout | null>(null);
  const [activated, setActivated] = useState<PreviewHighlightedLayout | null>(null);
  return {
    pageRoute,
    pageTitle,
    candidateLabel,
    acknowledgedMode,
    selectionRect,
    layouts,
    highlighted,
    setHighlighted,
    /** Where the last clicked marker's target was, until the page reports its layout. */
    activated,
    setActivated,
    setCandidateLabel,
    setAcknowledgedMode,
    setSelectionRect,
    setLayouts,
    /** A new document has none of the old page's layout. */
    resetLayout: () => {
      setLayouts({});
      setHighlighted(null);
      setActivated(null);
    },
    changePage: (route: string, title: string) => {
      setPageRoute(route);
      setPageTitle(title);
      setSelectionRect(null);
      setCandidateLabel(null);
    },
  };
}

/** Which comment is open: a draft or a saved item's editor. */
function usePreviewSurfaces(
  drafts: ReturnType<typeof usePreviewDrafts>,
  annotation: ReturnType<typeof usePreviewAnnotationMode>,
  editing: ReturnType<typeof useEditingItem>,
  commentPanel: ReturnType<typeof usePreviewCommentPanel>,
) {
  const { editingItem, open: openEditingItem, close: closeEditingItem } = editing;
  const openDraft = drafts.openDraft;
  const surfaceOpen = openDraft !== null || editingItem !== null;

  const closeComment = useCallback(() => {
    const step = closeCommentStep({
      draftComment: openDraft ? openDraft.comment : null,
      editing: editingItem !== null,
    });
    if (step === "discard-draft" && openDraft) drafts.discard(openDraft.id);
    else if (step === "keep-draft") drafts.close();
    else if (step === "close-editor") closeEditingItem();
  }, [closeEditingItem, drafts, editingItem, openDraft]);

  const handleEscape = useCallback(() => {
    const step = nextEscapeStep({ surfaceOpen, tool: annotation.tool });
    if (step === "close-surface") {
      closeComment();
    } else if (step === "interact") {
      annotation.setTool("interact");
    } else {
      annotation.exit();
    }
  }, [annotation, closeComment, surfaceOpen]);

  /** Opens a draft or a saved item's comment, closing any other open comment. */
  const { touch, setOpen: setPanelOpen } = commentPanel;
  const openCapture = useCallback(
    (id: string) => {
      if (touch) setPanelOpen(false);
      if (drafts.find(id)) {
        closeEditingItem();
        drafts.open(id);
      } else {
        closeComment();
        openEditingItem(id);
      }
    },
    [closeComment, closeEditingItem, drafts, openEditingItem, setPanelOpen, touch],
  );

  /** Choosing another tool closes the open comment, so the new tool works at once. */
  const { tool, setTool } = annotation;
  const selectTool = useCallback(
    (next: PreviewAnnotationTool) => {
      if (next === tool) return;
      if (surfaceOpen) closeComment();
      setTool(next);
    },
    [closeComment, setTool, surfaceOpen, tool],
  );

  return {
    editingItem,
    editingItemGeneration: editing.generation,
    closeItemEditor: () => closeEditingItem(editing.generation ?? undefined),
    closeComment,
    handleEscape,
    openCapture,
    selectTool,
  };
}

function useEditingItem(sourceItems: TaskPreviewFeedback[]) {
  const [session, setSession] = useState<{ id: string; generation: number } | null>(null);
  const sessionRef = useRef(session);
  const nextGeneration = useRef(0);
  const open = useCallback((id: string) => {
    const next = { id, generation: ++nextGeneration.current };
    sessionRef.current = next;
    setSession(next);
  }, []);
  const close = useCallback((expectedGeneration?: number) => {
    const current = sessionRef.current;
    if (
      !current ||
      (expectedGeneration !== undefined && current.generation !== expectedGeneration)
    ) {
      return;
    }
    sessionRef.current = null;
    setSession(null);
  }, []);
  const editingItem = sourceItems.find((item) => item.id === session?.id) ?? null;
  return { editingItem, generation: session?.generation ?? null, open, close };
}

/** A text or element comment pauses picking so the captured target stays highlighted. */
function isPausingComment(
  openDraft: PreviewDraftEntry | null,
  editingItem: TaskPreviewFeedback | null,
) {
  const kind = openDraft?.payload.kind ?? editingItem?.kind;
  return kind === "text" || kind === "element";
}

function useRegionProjection(
  iframeRef: React.RefObject<HTMLIFrameElement | null>,
  collections: ReturnType<typeof useSourceCollections>,
  drafts: PreviewDraftEntry[],
  page: ReturnType<typeof usePreviewPageState>,
  view: {
    active: boolean;
    feedbackEnabled: boolean;
    highlightedId: string | null;
    revealId: string | null;
  },
) {
  const { markers, sources, sourceItems, numbers } = collections;
  const { active, feedbackEnabled, highlightedId, revealId } = view;
  const anchors = useMemo<PreviewRegionAnchor[]>(
    () =>
      active && feedbackEnabled
        ? sources.map((region) => ({
            id: region.id,
            page_route: region.route,
            capture_rect: region.rect,
          }))
        : [],
    [active, feedbackEnabled, sources],
  );
  // Region captures are drawn and edited only in annotation mode; text and
  // element markers stay on the page so their comments can be reopened.
  const visibleMarkers = useMemo(() => {
    if (!feedbackEnabled) return [];
    return active ? markers : markers.filter((marker) => marker.kind !== "screenshot");
  }, [active, feedbackEnabled, markers]);
  const projectMarkers = useCallback(() => {
    if (iframeRef.current) {
      sendProjectPreviewMarkers(
        iframeRef.current,
        visibleMarkers,
        anchors,
        feedbackEnabled ? highlightedId : null,
        feedbackEnabled ? revealId : null,
      );
    }
  }, [anchors, feedbackEnabled, highlightedId, iframeRef, revealId, visibleMarkers]);
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
    () => (active ? regionViews(sources, page.pageRoute, page.layouts, { comments, numbers }) : []),
    [active, comments, numbers, page.layouts, page.pageRoute, sources],
  );
  return { projectMarkers, regions };
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
    onRegionLayout: (regions, highlighted) => {
      page.setLayouts(Object.fromEntries(regions.map((region) => [region.id, region])));
      page.setHighlighted(highlighted);
    },
    onEscape: surfaces.handleEscape,
    onMarkerActivated: (id, rect) => {
      page.setActivated(rect ? { id, rect } : null);
      surfaces.openCapture(id);
    },
    onPagePointerDown: surfaces.closeComment,
  });
}

/** The comment panel and annotation mode, which pauses while a text or element comment is open. */
function useAnnotationModes(options: {
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  enabled: boolean;
  regionBlocked: boolean;
  openDraft: PreviewDraftEntry | null;
  editingItem: TaskPreviewFeedback | null;
}) {
  const { openDraft, editingItem } = options;
  const commentPanel = usePreviewCommentPanel();
  const annotation = usePreviewAnnotationMode({
    iframeRef: options.iframeRef,
    enabled: options.enabled,
    regionBlocked: options.regionBlocked,
    paused: isPausingComment(openDraft, editingItem),
    surfaceOpen: openDraft !== null || editingItem !== null,
  });
  return { commentPanel, annotation };
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
  const editing = useEditingItem(collections.sourceItems);
  const { commentPanel, annotation } = useAnnotationModes({
    iframeRef,
    enabled,
    regionBlocked: collections.limitReached,
    openDraft: drafts.openDraft,
    editingItem: editing.editingItem,
  });
  const [selectedId, setSelectedId] = useState<string | null>(null);
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
  const projection = useRegionProjection(iframeRef, collections, drafts.drafts, page, {
    active: annotation.active,
    feedbackEnabled: enabled,
    highlightedId: drafts.openDraft?.id ?? editing.editingItem?.id ?? selectedId,
    revealId: selectedId,
  });
  const surfaces = usePreviewSurfaces(drafts, annotation, editing, commentPanel);

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
    ...commentAnchors(iframeRef, page, regions, openDraft, surfaces.editingItem),
    setDraftComment: (comment: string) => {
      if (openDraft) drafts.setComment(openDraft.id, comment);
    },
    captureNumbers: collections.numbers,
    commentPanel,
    selectedId,
    /** Highlights a capture's marker from the comment list. */
    selectCapture: setSelectedId,
    discardDraft: drafts.discard,
    ...save,
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
    handleIframeLoad: () => {
      page.resetLayout();
      annotation.resend();
      projection.projectMarkers();
    },
  };
}

export type PreviewCaptureController = ReturnType<typeof usePreviewCapture>;
