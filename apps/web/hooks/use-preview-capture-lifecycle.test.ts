import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { INSPECTOR_PROTOCOL_VERSION } from "@/lib/preview-inspect-bridge";
import type { TaskPreviewFeedback } from "@/lib/types/http";

const INSPECTOR_SOURCE = "kandev-inspector";
const TOTAL_SELECTOR = "#total";
const SCREENSHOT_ATTACHMENT_ID = "screenshot-attachment-1";
const PNG_MIME_TYPE = "image/png";
const SCREENSHOT_EVENT_TYPE = "screenshot-region-selected";
const MARKER_ACTIVATED = "marker-activated";
const PREVIEW_URL = "blob:preview-screenshot";
const SOURCE_LABEL = "http://localhost:3000";
const PREVIEW_ORIGIN = new URL(SOURCE_LABEL).origin;
const CREATED_AT = "2026-09-15T00:00:00Z";

const feedback = vi.hoisted(() => ({
  items: [] as TaskPreviewFeedback[],
  snapshot: { task_id: "task-1", revision: 0, items: [] as TaskPreviewFeedback[] },
  create: vi.fn(),
  update: vi.fn(),
  replaceScreenshot: vi.fn(),
  remove: vi.fn(),
  clear: vi.fn(),
  isMutating: false,
  mutationError: null as string | null,
}));
vi.mock("@/hooks/domains/comments/use-preview-feedback", () => ({
  usePreviewFeedback: () => feedback,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      kanban: { tasks: [{ id: "task-1", workspaceId: "workspace-1" }] },
      kanbanMulti: { snapshots: {} },
      workspaces: { activeId: "workspace-1" },
    }),
}));

const attachments = vi.hoisted(() => ({
  uploadAttachment: vi.fn(),
  deleteAttachment: vi.fn(),
}));
vi.mock("@/lib/api/domains/attachment-api", () => attachments);

const html2canvas = vi.hoisted(() => vi.fn());
vi.mock("html2canvas-pro", () => ({ default: html2canvas }));

const bridge = vi.hoisted(() => ({
  sendSetPreviewCaptureMode: vi.fn(),
  sendSetAnnotationActive: vi.fn(),
  sendCaptureSelection: vi.fn(),
  sendProjectPreviewMarkers: vi.fn(),
}));
vi.mock("@/lib/preview-inspect-bridge", async () => {
  const actual = await vi.importActual<typeof import("@/lib/preview-inspect-bridge")>(
    "@/lib/preview-inspect-bridge",
  );
  return { ...actual, ...bridge };
});

import { usePreviewCapture } from "./use-preview-capture";

const source = {
  kind: "browser" as const,
  sessionId: "session-1",
  label: SOURCE_LABEL,
};

function setup() {
  const iframe = document.createElement("iframe");
  iframe.src = SOURCE_LABEL;
  document.body.appendChild(iframe);
  const iframeRef = { current: iframe };
  const rendered = renderHook(() =>
    usePreviewCapture({ taskId: "task-1", iframeRef, source, enabled: true }),
  );
  return { iframe, iframeRef, ...rendered };
}

function dispatch(sourceWindow: MessageEventSource | null, data: unknown, origin = PREVIEW_ORIGIN) {
  window.dispatchEvent(new MessageEvent("message", { source: sourceWindow, data, origin }));
}

function bridgeMessage(type: string, payload: unknown) {
  return { source: INSPECTOR_SOURCE, version: INSPECTOR_PROTOCOL_VERSION, type, payload };
}

function regionSelected(rect = { x: 0, y: 0, width: 20, height: 20 }) {
  return bridgeMessage(SCREENSHOT_EVENT_TYPE, {
    page_route: "/",
    page_title: "Home",
    capture_rect: rect,
  });
}

function mockRaster(width = 20, height = 20) {
  const png = new Blob([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], {
    type: PNG_MIME_TYPE,
  });
  html2canvas.mockResolvedValue({
    width,
    height,
    toBlob: (callback: BlobCallback) => callback(png),
  });
}

function textCapture() {
  return {
    source: INSPECTOR_SOURCE,
    version: INSPECTOR_PROTOCOL_VERSION,
    type: "capture-completed",
    payload: {
      kind: "text",
      page_route: "/checkout",
      page_title: "Checkout",
      selected_text: "$42.00",
      text_anchor: {
        start: { selector: TOTAL_SELECTOR, node_path: [0], offset: 0 },
        end: { selector: TOTAL_SELECTOR, node_path: [0], offset: 6 },
        rects: [{ x: 10, y: 20, width: 50, height: 18 }],
        union_rect: { x: 10, y: 20, width: 50, height: 18 },
        scroll_x: 0,
        scroll_y: 120,
        viewport_width: 390,
        viewport_height: 844,
        device_pixel_ratio: 3,
        containing_element: {
          tag: "span",
          classes: ["total"],
          selector: TOTAL_SELECTOR,
          outer_html: '<span id="total">$42.00</span>',
        },
      },
    },
  };
}

beforeEach(() => {
  document.body.innerHTML = "";
  vi.clearAllMocks();
  feedback.items = [];
  feedback.snapshot = { task_id: "task-1", revision: 0, items: [] };
  feedback.create.mockResolvedValue({ task_id: "task-1", revision: 1, items: [] });
  attachments.uploadAttachment.mockResolvedValue({
    attachment_id: SCREENSHOT_ATTACHMENT_ID,
    name: "preview.png",
    mime_type: PNG_MIME_TYPE,
    kind: "image",
    delivery_mode: "prompt",
    size_bytes: 128,
    state: "staged",
  });
  attachments.deleteAttachment.mockResolvedValue(undefined);
  Object.defineProperty(URL, "createObjectURL", {
    configurable: true,
    value: vi.fn(() => PREVIEW_URL),
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    configurable: true,
    value: vi.fn(),
  });
});

function savedText(id: string, route = "/checkout"): TaskPreviewFeedback {
  return {
    id,
    task_id: "task-1",
    kind: "text",
    comment: "Saved note",
    source_kind: "browser",
    source_session_id: "session-1",
    source_label: SOURCE_LABEL,
    page_route: route,
    page_title: "Checkout",
    selected_text: "$42.00",
    version: 1,
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

describe("usePreviewCapture closing a comment", () => {
  it("removes an empty capture closed by Close or Escape", () => {
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => dispatch(iframe.contentWindow, textCapture()));
    act(() => result.current.closeComment());
    expect(result.current.drafts).toHaveLength(0);

    act(() => dispatch(iframe.contentWindow, textCapture()));
    act(() => dispatch(iframe.contentWindow, bridgeMessage("escape", {})));
    expect(result.current.drafts).toHaveLength(0);
  });

  it("keeps a typed capture as an unsaved draft", () => {
    const { iframe, result } = setup();
    act(() => dispatch(iframe.contentWindow, textCapture()));
    act(() => result.current.setDraftComment("bigger"));
    act(() => result.current.closeComment());
    expect(result.current.openDraft).toBeNull();
    expect(result.current.drafts).toHaveLength(1);
  });

  it("treats a press inside the page as an outside click while the panel is hidden", () => {
    window.localStorage.setItem("kandev:preview-comment-panel-open", "false");
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => dispatch(iframe.contentWindow, textCapture()));
    expect(bridge.sendSetAnnotationActive).toHaveBeenLastCalledWith(iframe, true, true);

    act(() => dispatch(iframe.contentWindow, bridgeMessage("page-pointerdown", {})));
    expect(result.current.drafts).toHaveLength(0);
    expect(bridge.sendSetAnnotationActive).toHaveBeenLastCalledWith(iframe, true, false);
    window.localStorage.clear();
  });

  it("treats a press inside the page as an outside click while the panel is open", () => {
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    expect(result.current.commentPanel.open).toBe(true);
    act(() => dispatch(iframe.contentWindow, textCapture()));
    expect(bridge.sendSetAnnotationActive).toHaveBeenLastCalledWith(iframe, true, true);

    act(() => dispatch(iframe.contentWindow, bridgeMessage("page-pointerdown", {})));
    expect(result.current.drafts).toHaveLength(0);
  });
});

describe("usePreviewCapture markers", () => {
  it("numbers saved items then drafts per route and opens a capture from its marker", async () => {
    feedback.items = [savedText("saved-1")];
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => dispatch(iframe.contentWindow, textCapture()));
    act(() => result.current.setDraftComment("draft note"));
    act(() => result.current.closeComment());

    await waitFor(() =>
      expect(bridge.sendProjectPreviewMarkers).toHaveBeenLastCalledWith(
        iframe,
        [
          expect.objectContaining({ id: "saved-1", number: 1 }),
          expect.objectContaining({ pending: true, number: 2 }),
        ],
        [],
        null,
        null,
      ),
    );
    expect(result.current.captureNumbers.get("saved-1")).toBe(1);

    act(() => dispatch(iframe.contentWindow, bridgeMessage(MARKER_ACTIVATED, { id: "saved-1" })));
    expect(result.current.editingItem?.id).toBe("saved-1");

    const draftId = result.current.drafts[0].id;
    act(() => dispatch(iframe.contentWindow, bridgeMessage(MARKER_ACTIVATED, { id: draftId })));
    expect(result.current.openDraft?.id).toBe(draftId);
    expect(result.current.editingItem).toBeNull();
  });

  it("keeps text and element markers outside annotation mode, and regions inside it", async () => {
    const screenshot: TaskPreviewFeedback = {
      ...savedText("shot-1"),
      kind: "screenshot",
      selected_text: undefined,
      capture_rect: { x: 0, y: 0, width: 40, height: 40 },
    };
    feedback.items = [savedText("saved-1"), screenshot];
    const { iframe, result } = setup();
    const lastMarkerIds = () =>
      ((bridge.sendProjectPreviewMarkers.mock.lastCall?.[1] ?? []) as { id: string }[]).map(
        (marker) => marker.id,
      );
    await waitFor(() => expect(lastMarkerIds()).toEqual(["saved-1"]));

    act(() => result.current.annotation.enter());
    await waitFor(() => expect(lastMarkerIds()).toEqual(["saved-1", "shot-1"]));

    act(() => result.current.annotation.exit());
    await waitFor(() => expect(lastMarkerIds()).toEqual(["saved-1"]));

    act(() => dispatch(iframe.contentWindow, bridgeMessage(MARKER_ACTIVATED, { id: "saved-1" })));
    expect(result.current.editingItem?.id).toBe("saved-1");
    expect(result.current.annotation.active).toBe(false);
  });

  it("closes a comment opened outside annotation mode on a press in the page", () => {
    feedback.items = [savedText("saved-1")];
    const { iframe, result } = setup();
    act(() => dispatch(iframe.contentWindow, bridgeMessage(MARKER_ACTIVATED, { id: "saved-1" })));
    expect(bridge.sendSetAnnotationActive).toHaveBeenLastCalledWith(iframe, false, true);

    act(() => dispatch(iframe.contentWindow, bridgeMessage("page-pointerdown", {})));
    expect(result.current.editingItem).toBeNull();
    expect(bridge.sendSetAnnotationActive).toHaveBeenLastCalledWith(iframe, false, false);
  });
});

describe("usePreviewCapture pausing the tool", () => {
  it("pauses picking while a text or element comment is open and highlights the capture", async () => {
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => result.current.annotation.setTool("element"));
    expect(bridge.sendSetPreviewCaptureMode).toHaveBeenLastCalledWith(iframe, "element");

    act(() => dispatch(iframe.contentWindow, textCapture()));
    expect(bridge.sendSetPreviewCaptureMode).toHaveBeenLastCalledWith(iframe, null);
    expect(result.current.annotation.tool).toBe("element");
    const draftId = result.current.drafts[0].id;
    await waitFor(() =>
      expect(bridge.sendProjectPreviewMarkers).toHaveBeenLastCalledWith(
        iframe,
        expect.any(Array),
        [],
        draftId,
        null,
      ),
    );

    act(() => result.current.setDraftComment("note"));
    act(() => result.current.closeComment());
    expect(bridge.sendSetPreviewCaptureMode).toHaveBeenLastCalledWith(iframe, "element");
  });

  it("closes an open comment and resumes picking when another tool is chosen", () => {
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => result.current.selectTool("text"));
    act(() => dispatch(iframe.contentWindow, textCapture()));
    act(() => result.current.setDraftComment("keep me"));
    expect(bridge.sendSetPreviewCaptureMode).toHaveBeenLastCalledWith(iframe, null);

    act(() => result.current.selectTool("region"));
    expect(result.current.openDraft).toBeNull();
    expect(result.current.drafts).toHaveLength(1);
    expect(result.current.annotation.tool).toBe("region");
    expect(bridge.sendSetPreviewCaptureMode).toHaveBeenLastCalledWith(iframe, "screenshot");
  });

  it("discards an empty capture when another tool is chosen", () => {
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => result.current.selectTool("text"));
    act(() => dispatch(iframe.contentWindow, textCapture()));
    act(() => result.current.selectTool("element"));
    expect(result.current.drafts).toHaveLength(0);
    expect(bridge.sendSetPreviewCaptureMode).toHaveBeenLastCalledWith(iframe, "element");
  });

  it("keeps the open comment when the current tool is chosen again", () => {
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => result.current.selectTool("text"));
    act(() => dispatch(iframe.contentWindow, textCapture()));
    act(() => result.current.selectTool("text"));
    expect(result.current.openDraft).not.toBeNull();
  });

  it("keeps the Region tool active while a region comment is open", () => {
    mockRaster();
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => dispatch(iframe.contentWindow, regionSelected()));
    expect(bridge.sendSetPreviewCaptureMode).toHaveBeenLastCalledWith(iframe, "screenshot");
  });
});

describe("usePreviewCapture region alignment", () => {
  const layoutRect = { x: 50, y: 60, width: 20, height: 20 };
  const clipRect = { x: 0, y: 55, width: 300, height: 100 };

  it("marks a region drifted when the page reports it", async () => {
    mockRaster();
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() =>
      dispatch(
        iframe.contentWindow,
        bridgeMessage("route-changed", { page_route: "/", page_title: "" }),
      ),
    );
    act(() => dispatch(iframe.contentWindow, regionSelected()));
    const id = result.current.drafts[0].id;
    act(() =>
      dispatch(
        iframe.contentWindow,
        bridgeMessage("region-layout", {
          regions: [{ id, rect: layoutRect, visible: true, drifted: true, clip: clipRect }],
        }),
      ),
    );
    expect(result.current.regions[0]).toMatchObject({
      id,
      rect: layoutRect,
      drifted: true,
      clip: clipRect,
    });
  });

  it("opens a saved item's editor beside its highlighted marker", () => {
    feedback.items = [savedText("saved-1")];
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => dispatch(iframe.contentWindow, bridgeMessage(MARKER_ACTIVATED, { id: "saved-1" })));
    expect(result.current.editingAnchor).toBeNull();

    act(() =>
      dispatch(
        iframe.contentWindow,
        bridgeMessage("region-layout", {
          regions: [],
          highlighted: { id: "saved-1", rect: layoutRect },
        }),
      ),
    );
    expect(result.current.editingAnchor).not.toBeNull();
  });

  it("opens a comment from its marker beside the target it reports, and keeps it there", () => {
    feedback.items = [savedText("saved-1")];
    const { iframe, result } = setup();
    iframe.getBoundingClientRect = () =>
      ({ left: 200, top: 100, right: 800, bottom: 700, width: 600, height: 600 }) as DOMRect;
    const clicked = bridgeMessage(MARKER_ACTIVATED, { id: "saved-1", rect: layoutRect });
    act(() => dispatch(iframe.contentWindow, clicked));
    const opened = result.current.editingAnchor;
    expect(opened).not.toBeNull();

    const layout = (highlighted: unknown) =>
      bridgeMessage("region-layout", { regions: [], highlighted });
    act(() => dispatch(iframe.contentWindow, layout(null)));
    expect(result.current.editingAnchor).toEqual(opened);
    act(() => dispatch(iframe.contentWindow, layout({ id: "saved-1", rect: layoutRect })));
    expect(result.current.editingAnchor).toEqual(opened);
  });

  it("reopens a draft from its marker beside the target it reports", () => {
    const { iframe, result } = setup();
    iframe.getBoundingClientRect = () =>
      ({ left: 200, top: 100, right: 800, bottom: 700, width: 600, height: 600 }) as DOMRect;
    act(() => dispatch(iframe.contentWindow, textCapture()));
    act(() => result.current.setDraftComment("note"));
    act(() => result.current.closeComment());
    const atCapture = result.current.drafts[0];

    const moved = { ...layoutRect, y: layoutRect.y + 200 };
    act(() =>
      dispatch(
        iframe.contentWindow,
        bridgeMessage(MARKER_ACTIVATED, { id: atCapture.id, rect: moved }),
      ),
    );
    expect(result.current.openDraft?.id).toBe(atCapture.id);
    expect(result.current.draftAnchor?.y).toBe(100 + moved.y + moved.height);
  });

  it("tells the page each text comment's text so it can find it again", async () => {
    feedback.items = [savedText("saved-1")];
    const { iframe } = setup();
    await waitFor(() =>
      expect(bridge.sendProjectPreviewMarkers).toHaveBeenLastCalledWith(
        iframe,
        [expect.objectContaining({ id: "saved-1", selected_text: "$42.00" })],
        [],
        null,
        null,
      ),
    );
  });
});
