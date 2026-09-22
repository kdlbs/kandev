import type {
  PreviewCaptureRect,
  PreviewElementSnapshot,
  PreviewTextAnchor,
} from "@/lib/types/http-agents";

export const INSPECTOR_SOURCE = "kandev-inspector" as const;
export const INSPECTOR_PROTOCOL_VERSION = 2 as const;

const MAX_ROUTE_LENGTH = 4_096;
const MAX_TITLE_LENGTH = 1_024;
const MAX_LABEL_LENGTH = 1_024;
const MAX_SELECTED_TEXT_LENGTH = 262_144;
const MAX_OUTER_HTML_LENGTH = 65_536;
const MAX_ELEMENT_TEXT_LENGTH = 16_384;
const MAX_SELECTOR_LENGTH = 4_096;
const MAX_RECTS = 512;
const MAX_NODE_PATH_LENGTH = 128;

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
};

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

interface PreviewCaptureCompletedMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "capture-completed";
  payload: PreviewCaptureDraft;
}

interface PreviewCaptureCancelledMessage {
  source: typeof INSPECTOR_SOURCE;
  version: typeof INSPECTOR_PROTOCOL_VERSION;
  type: "capture-cancelled";
  payload: Record<string, never>;
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
  | PreviewCaptureCompletedMessage
  | PreviewCaptureCancelledMessage
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

  if (data.version !== INSPECTOR_PROTOCOL_VERSION) return false;
  switch (data.type) {
    case "inspector-ready":
    case "route-changed":
      return isPageIdentity(data.payload);
    case "candidate-changed":
      return data.payload.label === null || isBoundedString(data.payload.label, MAX_LABEL_LENGTH);
    case "capture-completed":
      return isCaptureDraft(data.payload);
    case "capture-cancelled":
      return true;
    case "screenshot-region-selected":
      return isPageIdentity(data.payload) && isCaptureRect(data.payload.capture_rect);
    default:
      return false;
  }
}

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
  return hasOptionalFiniteNumberFields(value, [
    "document_x",
    "document_y",
    "scroll_x",
    "scroll_y",
    "viewport_width",
    "viewport_height",
    "device_pixel_ratio",
  ]);
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
  payload: { markers: PreviewMarkerProjection[] };
}

export function sendSetPreviewCaptureMode(
  iframe: HTMLIFrameElement,
  mode: PreviewCaptureMode | null,
): void {
  iframe.contentWindow?.postMessage(
    {
      source: INSPECTOR_SOURCE,
      version: INSPECTOR_PROTOCOL_VERSION,
      type: "set-capture-mode",
      payload: { mode },
    } satisfies SetPreviewCaptureModeCommand,
    "*",
  );
}

export function sendProjectPreviewMarkers(
  iframe: HTMLIFrameElement,
  markers: PreviewMarkerProjection[],
): void {
  iframe.contentWindow?.postMessage(
    {
      source: INSPECTOR_SOURCE,
      version: INSPECTOR_PROTOCOL_VERSION,
      type: "project-markers",
      payload: { markers },
    } satisfies ProjectPreviewMarkersCommand,
    "*",
  );
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
