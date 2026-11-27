import type {
  PreviewCaptureRect,
  PreviewElementSnapshot,
  PreviewTextAnchor,
} from "@/lib/types/http-agents";
import { generateUUID } from "@/lib/uuid";

export const INSPECTOR_SOURCE = "kandev-inspector" as const;
export const INSPECTOR_PROTOCOL_VERSION = 3 as const;

const MAX_ROUTE_LENGTH = 4_096;
const MAX_TITLE_LENGTH = 1_024;
const MAX_LABEL_LENGTH = 1_024;
const MAX_SELECTED_TEXT_LENGTH = 262_144;
const MAX_OUTER_HTML_LENGTH = 65_536;
const MAX_ELEMENT_TEXT_LENGTH = 16_384;
const MAX_SELECTOR_LENGTH = 4_096;
const MAX_RECTS = 512;
const MAX_NODE_PATH_LENGTH = 128;
const MAX_REGION_ID_LENGTH = 128;
export const MAX_PROJECTED_REGIONS = 32;

/** Returns the configured HTTP(S) origin for this preview iframe. */
export function previewOrigin(iframe: HTMLIFrameElement): string | null {
  const base = typeof window === "undefined" ? undefined : window.location.href;
  const source = iframe.src || base;
  if (!source) return null;
  try {
    const origin = new URL(source, base);
    if (origin.protocol !== "http:" && origin.protocol !== "https:") return null;
    return origin.origin;
  } catch {
    return null;
  }
}

export type PreviewCaptureMode = "text" | "element" | "screenshot";

export type PreviewCaptureDraft = {
  kind: "text" | "element";
  page_route: string;
  page_title: string;
  selected_text?: string;
  text_anchor?: PreviewTextAnchor;
  element_snapshot?: PreviewElementSnapshot;
  capture_rect?: PreviewCaptureRect;
};

export type PreviewMarkerProjection = {
  id: string;
  kind: "text" | "element" | "screenshot";
  page_route: string;
  text_anchor?: PreviewTextAnchor;
  element_snapshot?: PreviewElementSnapshot;
  capture_rect?: PreviewCaptureRect;
  /** Unsaved draft, drawn dashed. */
  pending?: boolean;
  /** Label on the badge, shared with the comment list. */
  number?: number;
  /** Start of a text comment's selection, to check its text is still on the page. */
  selected_text?: string;
};

/** A screenshot region the bridge tracks through document and container scrolling. */
export type PreviewRegionAnchor = {
  id: string;
  page_route: string;
  capture_rect: PreviewCaptureRect;
};

export type PreviewRegionLayout = {
  id: string;
  rect: { x: number; y: number; width: number; height: number };
  visible: boolean;
  /** The outline no longer lines up reliably with the content it was drawn over. */
  drifted?: boolean;
  /** The scroll container's box, when the region is scrolled partly outside it. */
  clip?: { x: number; y: number; width: number; height: number };
};

/** Viewport rectangle of the highlighted marker, used to place its comment box. */
export type PreviewHighlightedLayout = {
  id: string;
  rect: { x: number; y: number; width: number; height: number };
};

/** The element a region follows, as stored on its capture rectangle. */
export type PreviewContentAnchor = Required<
  Pick<
    PreviewCaptureRect,
    "anchor_selector" | "anchor_offset_x" | "anchor_offset_y" | "anchor_width" | "anchor_height"
  >
>;

/** The scroll container a region sits in, as stored on its capture rectangle. */
export type PreviewScrollContainer = Required<
  Pick<
    PreviewCaptureRect,
    "scroll_container_selector" | "container_scroll_x" | "container_scroll_y"
  >
>;

/** What the page reports about a moved region's new position. */
export type PreviewRegionPlacement = {
  anchor: PreviewContentAnchor | null;
  container: PreviewScrollContainer | null;
};

/** How long a moved region waits for its new anchor before saving without one. */
export const REGION_ANCHOR_TIMEOUT_MS = 500;

interface InspectorReadyMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "inspector-ready";
  payload: { page_route: string; page_title: string };
}

interface PreviewRouteChangedMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "route-changed";
  payload: { page_route: string; page_title: string };
}

interface PreviewCandidateChangedMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "candidate-changed";
  payload: { label: string | null };
}

interface PreviewCaptureModeChangedMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "capture-mode-changed";
  payload: { mode: PreviewCaptureMode | null };
}

interface PreviewCaptureCompletedMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "capture-completed";
  payload: PreviewCaptureDraft;
}

interface PreviewEscapeMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "escape";
  payload: Record<string, never>;
}

interface PreviewMarkerActivatedMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "marker-activated";
  /** The rect is where the marker's target is, so its comment opens beside it at once. */
  payload: { id: string; rect?: PreviewHighlightedLayout["rect"] };
}

interface PreviewPagePointerDownMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "page-pointerdown";
  payload: Record<string, never>;
}

interface PreviewSelectionChangedMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "selection-changed";
  payload: { rect: PreviewCaptureRect | null };
}

interface PreviewRegionLayoutMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "region-layout";
  payload: { regions: PreviewRegionLayout[]; highlighted?: PreviewHighlightedLayout | null };
}

interface PreviewRegionAnchoredMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "region-anchored";
  payload: {
    request_id: string;
    anchor: PreviewContentAnchor | null;
    container?: PreviewScrollContainer | null;
  };
}

export type PreviewScreenshotRegion = {
  page_route: string;
  page_title: string;
  capture_rect: PreviewCaptureRect;
};

interface PreviewScreenshotRegionSelectedMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "screenshot-region-selected";
  payload: PreviewScreenshotRegion;
}

interface PreviewConsoleReadyMessage {
  source: typeof INSPECTOR_SOURCE;
  type: "console-ready";
  payload: Record<string, never>;
}

export type PreviewConsoleLevel = "log" | "warn" | "error" | "info" | "debug";
const PREVIEW_CONSOLE_LEVELS: ReadonlySet<string> = new Set([
  "log",
  "warn",
  "error",
  "info",
  "debug",
]);
const LEGACY_INSPECTOR_TYPES: ReadonlySet<unknown> = new Set(["console-ready", "console"]);

interface PreviewConsoleMessage {
  source: typeof INSPECTOR_SOURCE;
  type: "console";
  payload: { level: PreviewConsoleLevel; args: unknown[] };
}

export type InspectorMessage =
  | InspectorReadyMessage
  | PreviewRouteChangedMessage
  | PreviewCandidateChangedMessage
  | PreviewCaptureModeChangedMessage
  | PreviewCaptureCompletedMessage
  | PreviewEscapeMessage
  | PreviewMarkerActivatedMessage
  | PreviewPagePointerDownMessage
  | PreviewSelectionChangedMessage
  | PreviewRegionLayoutMessage
  | PreviewRegionAnchoredMessage
  | PreviewScreenshotRegionSelectedMessage
  | PreviewConsoleReadyMessage
  | PreviewConsoleMessage;

// Narrows past the union — and validates that `level` is one we handle and
// `args` is an array. Without the Array.isArray check, a malformed payload
// with non-iterable `args` would throw inside the consumer's `...args` spread
// and kill the message listener for the rest of the session.
export function isPreviewConsoleMessage(msg: InspectorMessage): msg is PreviewConsoleMessage {
  if (msg.type !== "console") return false;
  const p = msg.payload as { level?: unknown; args?: unknown };
  return (
    typeof p.level === "string" && PREVIEW_CONSOLE_LEVELS.has(p.level) && Array.isArray(p.args)
  );
}

export function isPreviewConsoleReadyMessage(
  msg: InspectorMessage,
): msg is PreviewConsoleReadyMessage {
  return msg.type === "console-ready";
}

export function isInspectorMessage(data: unknown): data is InspectorMessage {
  if (!isRecord(data) || data.source !== INSPECTOR_SOURCE || !isRecord(data.payload)) return false;

  // The console shim predates the capture protocol and is injected separately.
  // Keep accepting its narrow legacy messages until both scripts share a version.
  if (LEGACY_INSPECTOR_TYPES.has(data.type)) return isLegacyInspectorMessage(data);

  if (data.version !== INSPECTOR_PROTOCOL_VERSION || typeof data.type !== "string") return false;
  const validate = PAYLOAD_VALIDATORS.get(data.type);
  return validate ? validate(data.payload) : false;
}

const PAYLOAD_VALIDATORS = new Map<string, (payload: Record<string, unknown>) => boolean>([
  ["inspector-ready", (payload) => isPageIdentity(payload)],
  ["route-changed", (payload) => isPageIdentity(payload)],
  [
    "candidate-changed",
    (payload) => payload.label === null || isBoundedString(payload.label, MAX_LABEL_LENGTH),
  ],
  ["capture-mode-changed", (payload) => isCaptureModePayload(payload)],
  ["capture-completed", (payload) => isCaptureDraft(payload)],
  ["escape", () => true],
  [
    "marker-activated",
    (payload) =>
      isBoundedString(payload.id, MAX_REGION_ID_LENGTH) &&
      payload.id.length > 0 &&
      (payload.rect === undefined || isCaptureRect(payload.rect)),
  ],
  ["page-pointerdown", () => true],
  ["selection-changed", (payload) => payload.rect === null || isCaptureRect(payload.rect)],
  [
    "region-layout",
    (payload) =>
      isRegionLayoutList(payload.regions) && isOptionalHighlightedLayout(payload.highlighted),
  ],
  [
    "region-anchored",
    (payload) =>
      isBoundedString(payload.request_id, MAX_REGION_ID_LENGTH) &&
      (payload.anchor === null || isContentAnchor(payload.anchor)) &&
      (payload.container === undefined ||
        payload.container === null ||
        isScrollContainer(payload.container)),
  ],
  [
    "screenshot-region-selected",
    (payload) => isPageIdentity(payload) && isCaptureRect(payload.capture_rect),
  ],
]);

function isLegacyInspectorMessage(data: Record<string, unknown>): boolean {
  if (data.type === "console-ready") return true;
  if (data.type !== "console" || !isRecord(data.payload)) return false;
  return (
    typeof data.payload.level === "string" &&
    PREVIEW_CONSOLE_LEVELS.has(data.payload.level) &&
    Array.isArray(data.payload.args)
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isBoundedString(value: unknown, maximum: number): value is string {
  return typeof value === "string" && value.length <= maximum;
}

function isPreviewCaptureMode(value: unknown): value is PreviewCaptureMode {
  return value === "text" || value === "element" || value === "screenshot";
}

function isCaptureModePayload(payload: Record<string, unknown>): boolean {
  return payload.mode === null || isPreviewCaptureMode(payload.mode);
}

function isPageIdentity(value: Record<string, unknown>): boolean {
  return (
    isBoundedString(value.page_route, MAX_ROUTE_LENGTH) &&
    isBoundedString(value.page_title, MAX_TITLE_LENGTH)
  );
}

function isOptionalFiniteNumber(value: unknown): boolean {
  return value === undefined || (typeof value === "number" && Number.isFinite(value));
}

function hasFiniteNumberFields(value: Record<string, unknown>, fields: string[]) {
  return fields.every((field) => typeof value[field] === "number" && Number.isFinite(value[field]));
}

function hasOptionalFiniteNumberFields(value: Record<string, unknown>, fields: string[]) {
  return fields.every((field) => isOptionalFiniteNumber(value[field]));
}

function isCaptureRect(value: unknown): value is PreviewCaptureRect {
  if (!isRecord(value)) return false;
  if (!hasFiniteNumberFields(value, ["x", "y", "width", "height"])) return false;
  if ((value.width as number) < 0 || (value.height as number) < 0) return false;
  return (
    hasOptionalFiniteNumberFields(value, [
      "document_x",
      "document_y",
      "scroll_x",
      "scroll_y",
      "viewport_width",
      "viewport_height",
      "device_pixel_ratio",
      "container_scroll_x",
      "container_scroll_y",
      "anchor_offset_x",
      "anchor_offset_y",
      "anchor_width",
      "anchor_height",
    ]) &&
    isOptionalBoundedString(value.scroll_container_selector, MAX_SELECTOR_LENGTH) &&
    isOptionalBoundedString(value.anchor_selector, MAX_SELECTOR_LENGTH)
  );
}

function isScrollContainer(value: unknown): value is PreviewScrollContainer {
  return (
    isRecord(value) &&
    isBoundedString(value.scroll_container_selector, MAX_SELECTOR_LENGTH) &&
    hasFiniteNumberFields(value, ["container_scroll_x", "container_scroll_y"])
  );
}

function isContentAnchor(value: unknown): value is PreviewContentAnchor {
  return (
    isRecord(value) &&
    isBoundedString(value.anchor_selector, MAX_SELECTOR_LENGTH) &&
    hasFiniteNumberFields(value, [
      "anchor_offset_x",
      "anchor_offset_y",
      "anchor_width",
      "anchor_height",
    ])
  );
}

function isOptionalHighlightedLayout(value: unknown): boolean {
  if (value === undefined || value === null) return true;
  return (
    isRecord(value) && isBoundedString(value.id, MAX_REGION_ID_LENGTH) && isCaptureRect(value.rect)
  );
}

function isRegionLayout(value: unknown): value is PreviewRegionLayout {
  return (
    isRecord(value) &&
    isBoundedString(value.id, MAX_REGION_ID_LENGTH) &&
    typeof value.visible === "boolean" &&
    (value.drifted === undefined || typeof value.drifted === "boolean") &&
    (value.clip === undefined || isCaptureRect(value.clip)) &&
    isCaptureRect(value.rect)
  );
}

function isRegionLayoutList(value: unknown): value is PreviewRegionLayout[] {
  return (
    Array.isArray(value) && value.length <= MAX_PROJECTED_REGIONS && value.every(isRegionLayout)
  );
}

function isOptionalBoundedString(value: unknown, maximum: number) {
  return value === undefined || isBoundedString(value, maximum);
}

function isElementSnapshot(value: unknown): value is PreviewElementSnapshot {
  if (!isRecord(value)) return false;
  return (
    isBoundedString(value.tag, 128) &&
    value.tag.length > 0 &&
    isOptionalBoundedString(value.id, MAX_LABEL_LENGTH) &&
    Array.isArray(value.classes) &&
    value.classes.length <= 128 &&
    value.classes.every((item) => isBoundedString(item, MAX_LABEL_LENGTH)) &&
    isOptionalBoundedString(value.role, MAX_LABEL_LENGTH) &&
    isOptionalBoundedString(value.accessible_label, MAX_ELEMENT_TEXT_LENGTH) &&
    isOptionalBoundedString(value.visible_text, MAX_ELEMENT_TEXT_LENGTH) &&
    isOptionalBoundedString(value.selector, MAX_SELECTOR_LENGTH) &&
    isBoundedString(value.outer_html, MAX_OUTER_HTML_LENGTH)
  );
}

function isTextEndpoint(value: unknown): boolean {
  if (!isRecord(value)) return false;
  return (
    (value.selector === undefined || isBoundedString(value.selector, MAX_SELECTOR_LENGTH)) &&
    Array.isArray(value.node_path) &&
    value.node_path.length <= MAX_NODE_PATH_LENGTH &&
    value.node_path.every((part) => Number.isInteger(part) && part >= 0) &&
    Number.isInteger(value.offset) &&
    (value.offset as number) >= 0
  );
}

function isTextAnchor(value: unknown): value is PreviewTextAnchor {
  if (!isRecord(value) || !isTextEndpoint(value.start) || !isTextEndpoint(value.end)) return false;
  if (
    value.rects !== undefined &&
    (!Array.isArray(value.rects) ||
      value.rects.length > MAX_RECTS ||
      !value.rects.every(isCaptureRect))
  ) {
    return false;
  }
  return (
    (value.union_rect === undefined || isCaptureRect(value.union_rect)) &&
    isOptionalFiniteNumber(value.scroll_x) &&
    isOptionalFiniteNumber(value.scroll_y) &&
    isOptionalFiniteNumber(value.viewport_width) &&
    isOptionalFiniteNumber(value.viewport_height) &&
    isOptionalFiniteNumber(value.device_pixel_ratio) &&
    isElementSnapshot(value.containing_element)
  );
}

function isCaptureDraft(value: Record<string, unknown>): value is PreviewCaptureDraft {
  if (!isPageIdentity(value)) return false;
  if (value.kind === "text") {
    return (
      isBoundedString(value.selected_text, MAX_SELECTED_TEXT_LENGTH) &&
      value.selected_text.length > 0 &&
      isTextAnchor(value.text_anchor)
    );
  }
  if (value.kind === "element") {
    return isElementSnapshot(value.element_snapshot) && isCaptureRect(value.capture_rect);
  }
  return false;
}

interface SetPreviewCaptureModeCommand {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "set-capture-mode";
  payload: { mode: PreviewCaptureMode | null };
}

interface ProjectPreviewMarkersCommand {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "project-markers";
  payload: {
    markers: PreviewMarkerProjection[];
    regions: PreviewRegionAnchor[];
    highlighted_id: string | null;
    /** Scrolled into view once; set only when a capture is picked from the list. */
    reveal_id: string | null;
  };
}

function postCommand(iframe: HTMLIFrameElement, type: string, payload: Record<string, unknown>) {
  const targetOrigin = previewOrigin(iframe);
  if (!targetOrigin) return;
  iframe.contentWindow?.postMessage(
    { source: INSPECTOR_SOURCE, version: INSPECTOR_PROTOCOL_VERSION, type, payload },
    targetOrigin,
  );
}

/**
 * Tells the bridge whether Escape inside the page belongs to Kandev, and
 * whether page presses should be reported as outside clicks for an open comment.
 */
export function sendSetAnnotationActive(
  iframe: HTMLIFrameElement,
  active: boolean,
  surfaceOpen = false,
): void {
  postCommand(iframe, "set-annotation-active", { active, surface_open: surfaceOpen });
}

/** Captures the page's current text selection as a text draft. */
export function sendCaptureSelection(iframe: HTMLIFrameElement): void {
  postCommand(iframe, "capture-selection", {});
}

export function sendSetPreviewCaptureMode(
  iframe: HTMLIFrameElement,
  mode: PreviewCaptureMode | null,
): void {
  const targetOrigin = previewOrigin(iframe);
  if (!targetOrigin) return;
  iframe.contentWindow?.postMessage(
    {
      source: INSPECTOR_SOURCE,
      version: INSPECTOR_PROTOCOL_VERSION,
      type: "set-capture-mode",
      payload: { mode },
    } satisfies SetPreviewCaptureModeCommand,
    targetOrigin,
  );
}

export function sendProjectPreviewMarkers(
  iframe: HTMLIFrameElement,
  markers: PreviewMarkerProjection[],
  regions: PreviewRegionAnchor[] = [],
  highlightedId: string | null = null,
  revealId: string | null = null,
): void {
  const targetOrigin = previewOrigin(iframe);
  if (!targetOrigin) return;
  iframe.contentWindow?.postMessage(
    {
      source: INSPECTOR_SOURCE,
      version: INSPECTOR_PROTOCOL_VERSION,
      type: "project-markers",
      payload: {
        markers,
        regions: regions.slice(0, MAX_PROJECTED_REGIONS),
        highlighted_id: highlightedId,
        reveal_id: revealId,
      },
    } satisfies ProjectPreviewMarkersCommand,
    targetOrigin,
  );
}

/**
 * Asks the page for the element and scroll container a moved or resized
 * region now sits in. Resolves `null` when the page does not answer in time.
 */
export function requestRegionAnchor(
  iframe: HTMLIFrameElement,
  rect: { x: number; y: number; width: number; height: number },
): Promise<PreviewRegionPlacement | null> {
  const target = iframe.contentWindow;
  const targetOrigin = previewOrigin(iframe);
  if (!target || !targetOrigin) return Promise.resolve(null);
  const requestId = generateUUID();
  return new Promise((resolve) => {
    const finish = (placement: PreviewRegionPlacement | null) => {
      window.removeEventListener("message", onMessage);
      window.clearTimeout(timer);
      resolve(placement);
    };
    const onMessage = (event: MessageEvent) => {
      if (
        event.source !== target ||
        event.origin !== targetOrigin ||
        !isInspectorMessage(event.data)
      ) {
        return;
      }
      const data = event.data;
      if (data.type === "region-anchored" && data.payload.request_id === requestId) {
        finish({ anchor: data.payload.anchor, container: data.payload.container ?? null });
      }
    };
    const timer = window.setTimeout(() => finish(null), REGION_ANCHOR_TIMEOUT_MS);
    window.addEventListener("message", onMessage);
    postCommand(iframe, "anchor-region", { request_id: requestId, rect });
  });
}

export function sendConsoleBind(iframe: HTMLIFrameElement, targetOrigin: string): void {
  iframe.contentWindow?.postMessage(
    {
      source: INSPECTOR_SOURCE,
      type: "console-bind",
      payload: {},
    },
    targetOrigin,
  );
}
