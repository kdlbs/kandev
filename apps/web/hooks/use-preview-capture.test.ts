import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { INSPECTOR_PROTOCOL_VERSION } from "@/lib/preview-inspect-bridge";
import type { TaskPreviewFeedback } from "@/lib/types/http";

const INSPECTOR_SOURCE = "kandev-inspector";
const TOTAL_SELECTOR = "#total";
const SCREENSHOT_ATTACHMENT_ID = "screenshot-attachment-1";
const PNG_MIME_TYPE = "image/png";
const SCREENSHOT_EVENT_TYPE = "screenshot-region-selected";
const PREVIEW_URL = "blob:preview-screenshot";
const SOURCE_LABEL = "http://localhost:3000";
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

function setup(enabled = true) {
  const iframe = document.createElement("iframe");
  iframe.src = SOURCE_LABEL;
  document.body.appendChild(iframe);
  const iframeRef = { current: iframe };
  const rendered = renderHook(() =>
    usePreviewCapture({ taskId: "task-1", iframeRef, source, enabled }),
  );
  return { iframe, iframeRef, ...rendered };
}

function dispatch(
  sourceWindow: MessageEventSource | null,
  data: unknown,
  origin = new URL(SOURCE_LABEL).origin,
) {
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

describe("usePreviewCapture text", () => {
  it("keeps the inspector message listener stable across preview route changes", () => {
    const addListener = vi.spyOn(window, "addEventListener");
    const removeListener = vi.spyOn(window, "removeEventListener");
    const { iframe, result } = setup();
    const messageAddsBeforeRoute = addListener.mock.calls.filter(([type]) => type === "message");

    act(() =>
      dispatch(
        iframe.contentWindow,
        bridgeMessage("route-changed", { page_route: "/details", page_title: "Details" }),
      ),
    );

    expect(result.current.pageRoute).toBe("/details");
    expect(addListener.mock.calls.filter(([type]) => type === "message")).toHaveLength(
      messageAddsBeforeRoute.length,
    );
    expect(removeListener.mock.calls.filter(([type]) => type === "message")).toHaveLength(0);
  });

  it("reports the bridge mode only after the preview inspector confirms it", () => {
    const { iframe, result } = setup();

    act(() => result.current.annotation.enter());
    act(() => result.current.annotation.setTool("text"));
    expect(result.current.acknowledgedMode).toBeNull();
    expect(bridge.sendSetPreviewCaptureMode).toHaveBeenLastCalledWith(iframe, "text");

    act(() =>
      dispatch(iframe.contentWindow, bridgeMessage("capture-mode-changed", { mode: "text" })),
    );
    expect(result.current.acknowledgedMode).toBe("text");
  });

  it("keeps exact generated text evidence in a draft and persists it with a comment", async () => {
    const { iframe, result } = setup();

    act(() => dispatch(iframe.contentWindow, textCapture()));
    const draft = result.current.openDraft?.payload;
    if (draft?.kind !== "text") throw new Error("expected text draft");
    expect(draft.selected_text).toBe("$42.00");
    expect(draft.text_anchor?.start.node_path).toEqual([0]);

    await act(() => result.current.saveDraft("Keep this total visible"));
    expect(feedback.create).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "text",
        comment: "Keep this total visible",
        sourceKind: "browser",
        sourceSessionId: "session-1",
        sourceLabel: SOURCE_LABEL,
        pageRoute: "/checkout",
        selectedText: "$42.00",
        textAnchor: expect.objectContaining({ scroll_y: 120 }),
      }),
    );
    expect(result.current.drafts).toHaveLength(0);
    expect(result.current.saveCount).toBe(1);
  });

  it("ignores valid captures from any window except its iframe", () => {
    const { result } = setup();
    act(() => dispatch(window, textCapture()));
    expect(result.current.drafts).toHaveLength(0);
  });

  it("preserves the draft when persistence fails", async () => {
    feedback.create.mockResolvedValue(null);
    const { iframe, result } = setup();
    act(() => dispatch(iframe.contentWindow, textCapture()));
    await act(() => result.current.saveDraft("Retry this"));
    expect(result.current.openDraft?.payload.selected_text).toBe("$42.00");
    expect(result.current.saveCount).toBe(0);
  });

  it("sanitizes page routes before retaining or saving captured evidence", async () => {
    const { iframe, result } = setup();
    const capture = textCapture();
    capture.payload.page_route = "/checkout?step=shipping&access_token=secret#payment";

    act(() => dispatch(iframe.contentWindow, capture));
    expect(result.current.openDraft?.payload.page_route).toBe("/checkout?step=shipping");

    await act(() => result.current.saveDraft("Keep this route"));
    expect(feedback.create).toHaveBeenCalledWith(
      expect.objectContaining({ pageRoute: "/checkout?step=shipping" }),
    );
  });
});

describe("usePreviewCapture comment on selection", () => {
  it("tracks the page selection and captures it on request", () => {
    const { iframe, result } = setup();
    const rect = { x: 10, y: 20, width: 40, height: 16 };

    act(() => dispatch(iframe.contentWindow, bridgeMessage("selection-changed", { rect })));
    expect(result.current.selectionRect).toEqual(rect);

    act(() => result.current.captureSelection());
    expect(bridge.sendCaptureSelection).toHaveBeenCalledWith(iframe);

    act(() => dispatch(iframe.contentWindow, bridgeMessage("selection-changed", { rect: null })));
    expect(result.current.selectionRect).toBeNull();
  });
});

describe("usePreviewCapture marker projection", () => {
  it("projects saved markers and dashed unsaved drafts for the same source", async () => {
    feedback.items = [
      {
        id: "feedback-1",
        task_id: "task-1",
        kind: "element",
        comment: "Move this",
        source_kind: "browser",
        source_session_id: "session-1",
        source_label: SOURCE_LABEL,
        page_route: "/account",
        page_title: "Account",
        element_snapshot: {
          tag: "button",
          classes: [],
          selector: "#save",
          outer_html: '<button id="save">Save</button>',
        },
        capture_rect: { x: 1, y: 2, width: 80, height: 30 },
        version: 1,
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      },
    ];
    const { iframe, result } = setup();

    act(() =>
      dispatch(
        iframe.contentWindow,
        bridgeMessage("route-changed", { page_route: "/account", page_title: "Account" }),
      ),
    );
    expect(result.current.pageRoute).toBe("/account");
    act(() => dispatch(iframe.contentWindow, textCapture()));

    await waitFor(() =>
      expect(bridge.sendProjectPreviewMarkers).toHaveBeenLastCalledWith(
        iframe,
        [
          expect.objectContaining({ id: "feedback-1", page_route: "/account" }),
          expect.objectContaining({ kind: "text", pending: true }),
        ],
        [],
        expect.any(String),
        null,
      ),
    );
  });

  it("does not send marker evidence while preview feedback is disabled", () => {
    feedback.items = [
      {
        id: "private-feedback",
        task_id: "task-1",
        kind: "text",
        comment: "Private draft text",
        selected_text: "Account balance",
        source_kind: "browser",
        source_session_id: "session-1",
        source_label: SOURCE_LABEL,
        page_route: "/account",
        page_title: "Account",
        version: 1,
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      },
    ];
    const { iframe } = setup(false);

    expect(bridge.sendProjectPreviewMarkers).toHaveBeenLastCalledWith(iframe, [], [], null, null);
  });
});

describe("usePreviewCapture iframe origin boundary", () => {
  it("ignores inspector messages after the iframe navigates away from its configured origin", () => {
    const iframe = document.createElement("iframe");
    iframe.src = SOURCE_LABEL;
    iframe.src = "https://preview.example/page";
    document.body.appendChild(iframe);
    const rendered = renderHook(() =>
      usePreviewCapture({
        taskId: "task-1",
        iframeRef: { current: iframe },
        source,
        enabled: true,
      }),
    );

    act(() =>
      dispatch(
        iframe.contentWindow,
        bridgeMessage("route-changed", { page_route: "/attacker", page_title: "Attacker" }),
        "https://attacker.example",
      ),
    );

    expect(rendered.result.current.pageRoute).toBe("");
  });
});

describe("usePreviewCapture screenshots", () => {
  it("rasterizes a selected region for review, then uploads and claims it on save", async () => {
    mockRaster(600, 360);
    const { iframe, result } = setup();

    act(() =>
      dispatch(
        iframe.contentWindow,
        bridgeMessage(SCREENSHOT_EVENT_TYPE, {
          page_route: "/checkout",
          page_title: "Checkout",
          capture_rect: {
            x: 20,
            y: 40,
            width: 300,
            height: 180,
            document_x: 20,
            document_y: 640,
            device_pixel_ratio: 2,
          },
        }),
      ),
    );

    await waitFor(() => expect(result.current.isRasterizing).toBe(false));
    const draft = result.current.openDraft?.payload;
    if (draft?.kind !== "screenshot") throw new Error("expected screenshot draft");
    expect(draft.screenshot?.previewUrl).toBe(PREVIEW_URL);
    expect(html2canvas).toHaveBeenCalledWith(
      iframe.contentDocument?.documentElement,
      expect.objectContaining({ x: 20, y: 640, width: 300, height: 180 }),
    );

    await act(() => result.current.saveDraft("The form clips here"));
    expect(attachments.uploadAttachment).toHaveBeenCalledWith(
      expect.objectContaining({ type: PNG_MIME_TYPE }),
      { workspaceId: "workspace-1", kind: "image", deliveryMode: "prompt" },
    );
    expect(feedback.create).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "screenshot",
        comment: "The form clips here",
        captureRect: expect.objectContaining({ document_y: 640 }),
        screenshotAttachmentId: SCREENSHOT_ATTACHMENT_ID,
      }),
    );
    expect(result.current.drafts).toHaveLength(0);
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4
  it("keeps earlier drafts when another region is captured", async () => {
    mockRaster();
    const { iframe, result } = setup();

    act(() => dispatch(iframe.contentWindow, regionSelected()));
    await waitFor(() => expect(result.current.drafts).toHaveLength(1));
    act(() => result.current.setDraftComment("first"));
    act(() =>
      dispatch(iframe.contentWindow, regionSelected({ x: 30, y: 30, width: 20, height: 20 })),
    );
    await waitFor(() => expect(result.current.drafts).toHaveLength(2));

    expect(result.current.drafts.map((draft) => draft.open)).toEqual([false, true]);
    expect(result.current.drafts[0].comment).toBe("first");
    expect(attachments.deleteAttachment).not.toHaveBeenCalled();
  });
});

describe("usePreviewCapture screenshot limits and failures", () => {
  it("ignores new regions once saved and unsaved screenshots reach the limit", async () => {
    mockRaster();
    feedback.items = Array.from({ length: 10 }, (_, index) => ({
      id: `shot-${index}`,
      task_id: "task-1",
      kind: "screenshot" as const,
      comment: "c",
      source_kind: "browser" as const,
      source_label: "other",
      page_route: "/",
      page_title: "",
      capture_rect: { x: 0, y: 0, width: 5, height: 5 },
      version: 1,
      created_at: CREATED_AT,
      updated_at: CREATED_AT,
    }));
    const { iframe, result } = setup();
    expect(result.current.limitReached).toBe(true);

    act(() => dispatch(iframe.contentWindow, regionSelected()));

    expect(html2canvas).not.toHaveBeenCalled();
    expect(result.current.drafts).toHaveLength(0);
  });

  it("keeps the screenshot and staged attachment available when create fails", async () => {
    mockRaster();
    feedback.create.mockResolvedValue(null);
    const { iframe, result } = setup();
    act(() => dispatch(iframe.contentWindow, regionSelected()));
    await waitFor(() => {
      const payload = result.current.openDraft?.payload;
      expect(payload?.kind === "screenshot" && payload.screenshot).toBeTruthy();
    });
    await act(() => result.current.saveDraft("Retry me"));
    const draft = result.current.openDraft?.payload;
    if (draft?.kind !== "screenshot") throw new Error("expected screenshot draft");
    expect(draft.screenshot?.attachmentId).toBe(SCREENSHOT_ATTACHMENT_ID);
    expect(attachments.deleteAttachment).not.toHaveBeenCalled();
  });

  it("deletes an upload that finishes after the screenshot draft is discarded", async () => {
    mockRaster();
    let finishUpload: ((value: { attachment_id: string }) => void) | undefined;
    attachments.uploadAttachment.mockImplementation(
      () => new Promise((resolve) => (finishUpload = resolve)),
    );
    const { iframe, result } = setup();
    act(() => dispatch(iframe.contentWindow, regionSelected()));
    await waitFor(() => {
      const payload = result.current.openDraft?.payload;
      expect(payload?.kind === "screenshot" && payload.screenshot).toBeTruthy();
    });
    const id = result.current.openDraft?.id ?? "";

    let saveResult: Promise<boolean> | undefined;
    act(() => {
      saveResult = result.current.saveDraft("Cancel me");
    });
    await waitFor(() => expect(result.current.isUploading).toBe(true));
    act(() => result.current.discardDraft(id));
    await act(async () => finishUpload?.({ attachment_id: "late-screenshot" }));

    await expect(saveResult).resolves.toBe(false);
    expect(attachments.deleteAttachment).toHaveBeenCalledWith("late-screenshot");
    expect(feedback.create).not.toHaveBeenCalled();
  });

  it("releases unsaved screenshots when the preview is disabled", async () => {
    mockRaster();
    const iframe = document.createElement("iframe");
    document.body.appendChild(iframe);
    const iframeRef = { current: iframe };
    const rendered = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        usePreviewCapture({ taskId: "task-1", iframeRef, source, enabled }),
      { initialProps: { enabled: true } },
    );
    act(() => dispatch(iframe.contentWindow, regionSelected()));
    await waitFor(() => expect(rendered.result.current.drafts).toHaveLength(1));

    rendered.rerender({ enabled: false });

    await waitFor(() => expect(rendered.result.current.drafts).toHaveLength(0));
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(PREVIEW_URL);
    rendered.unmount();
  });
});

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6
describe("usePreviewCapture region recapture", () => {
  it("replaces a draft's image and keeps its comment", async () => {
    mockRaster();
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => dispatch(iframe.contentWindow, regionSelected()));
    await waitFor(() => expect(result.current.drafts).toHaveLength(1));
    const id = result.current.drafts[0].id;
    act(() => result.current.setDraftComment("Keep me"));

    let ok = false;
    await act(async () => {
      ok = await result.current.recaptureRegion(id, { x: 5, y: 6, width: 30, height: 40 });
    });

    expect(ok).toBe(true);
    const draft = result.current.drafts[0];
    expect(draft.comment).toBe("Keep me");
    expect(draft.payload.capture_rect).toMatchObject({ x: 5, y: 6, width: 30, height: 40 });
  });

  it("keeps the previous rectangle and image when recapture fails", async () => {
    mockRaster();
    const { iframe, result } = setup();
    act(() => dispatch(iframe.contentWindow, regionSelected()));
    await waitFor(() => expect(result.current.isRasterizing).toBe(false));
    const before = result.current.drafts[0].payload;
    html2canvas.mockRejectedValue(new Error("raster failed"));

    let ok = true;
    await act(async () => {
      ok = await result.current.recaptureRegion(result.current.drafts[0].id, {
        x: 5,
        y: 6,
        width: 30,
        height: 40,
      });
    });

    expect(ok).toBe(false);
    expect(result.current.recaptureFailed).toBe(true);
    expect(result.current.drafts[0].payload).toBe(before);
  });

  it("uploads and replaces a saved screenshot's image", async () => {
    mockRaster();
    feedback.items = [
      {
        id: "saved-shot",
        task_id: "task-1",
        kind: "screenshot",
        comment: "Clipped",
        source_kind: "browser",
        source_session_id: "session-1",
        source_label: SOURCE_LABEL,
        page_route: "/",
        page_title: "Home",
        capture_rect: { x: 0, y: 0, width: 20, height: 20 },
        screenshot_attachment_id: "old-shot",
        version: 3,
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      },
    ];
    feedback.replaceScreenshot.mockResolvedValue({ task_id: "task-1", revision: 2, items: [] });
    const { result } = setup();

    await act(async () => {
      await result.current.recaptureRegion("saved-shot", { x: 1, y: 2, width: 30, height: 40 });
    });

    expect(feedback.replaceScreenshot).toHaveBeenCalledWith(
      "saved-shot",
      3,
      expect.objectContaining({ x: 1, y: 2, width: 30, height: 40 }),
      SCREENSHOT_ATTACHMENT_ID,
    );
  });
});

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5
describe("usePreviewCapture Escape", () => {
  it("closes the surface, then drops to Interact, then exits", () => {
    const { iframe, result } = setup();
    act(() => result.current.annotation.enter());
    act(() => dispatch(iframe.contentWindow, textCapture()));
    expect(result.current.openDraft).not.toBeNull();

    act(() => result.current.setDraftComment("keep me"));
    act(() => dispatch(iframe.contentWindow, bridgeMessage("escape", {})));
    expect(result.current.openDraft).toBeNull();
    expect(result.current.drafts).toHaveLength(1);
    expect(result.current.annotation.tool).toBe("region");

    act(() => dispatch(iframe.contentWindow, bridgeMessage("escape", {})));
    expect(result.current.annotation.tool).toBe("interact");
    expect(result.current.annotation.active).toBe(true);

    act(() => dispatch(iframe.contentWindow, bridgeMessage("escape", {})));
    expect(result.current.annotation.active).toBe(false);
    expect(result.current.drafts).toHaveLength(1);
  });
});

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-004.6
describe("usePreviewCapture comment surface", () => {
  it("opens the comment surface below the capture and keeps the draft when dismissed", () => {
    const { iframe, result } = setup();
    iframe.getBoundingClientRect = () =>
      ({ left: 200, top: 100, right: 800, bottom: 700, width: 600, height: 600 }) as DOMRect;

    act(() => dispatch(iframe.contentWindow, textCapture()));
    expect(result.current.openDraft).not.toBeNull();
    expect(result.current.draftAnchor).toEqual({ x: 210, y: 138 });

    act(() => result.current.setDraftComment("half-written note"));
    act(() => result.current.closeComment());
    expect(result.current.openDraft).toBeNull();
    expect(result.current.drafts[0].comment).toBe("half-written note");

    act(() => result.current.openCapture(result.current.drafts[0].id));
    expect(result.current.openDraft?.comment).toBe("half-written note");
  });

  it("closes the surface when the draft is discarded or saved", async () => {
    const { iframe, result } = setup();
    act(() => dispatch(iframe.contentWindow, textCapture()));
    act(() => result.current.discardDraft(result.current.drafts[0].id));
    expect(result.current.openDraft).toBeNull();
    expect(result.current.drafts).toHaveLength(0);

    act(() => dispatch(iframe.contentWindow, textCapture()));
    await act(() => result.current.saveDraft("Saved"));
    expect(result.current.openDraft).toBeNull();
  });
});
