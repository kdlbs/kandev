import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { INSPECTOR_PROTOCOL_VERSION } from "@/lib/preview-inspect-bridge";
import type { TaskPreviewFeedback } from "@/lib/types/http";

const INSPECTOR_SOURCE = "kandev-inspector";
const TOTAL_SELECTOR = "#total";
const SCREENSHOT_ATTACHMENT_ID = "screenshot-attachment-1";
const PNG_MIME_TYPE = "image/png";
const SCREENSHOT_EVENT_TYPE = "screenshot-region-selected";

const feedback = vi.hoisted(() => ({
  items: [] as TaskPreviewFeedback[],
  snapshot: { task_id: "task-1", revision: 0, items: [] as TaskPreviewFeedback[] },
  create: vi.fn(),
  update: vi.fn(),
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
vi.mock("html2canvas", () => ({ default: html2canvas }));

const bridge = vi.hoisted(() => ({
  sendSetPreviewCaptureMode: vi.fn(),
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
  label: "http://localhost:3000",
};

function setup() {
  const iframe = document.createElement("iframe");
  document.body.appendChild(iframe);
  const iframeRef = { current: iframe };
  const rendered = renderHook(() =>
    usePreviewCapture({ taskId: "task-1", iframeRef, source, enabled: true }),
  );
  return { iframe, iframeRef, ...rendered };
}

function dispatch(sourceWindow: MessageEventSource | null, data: unknown) {
  window.dispatchEvent(new MessageEvent("message", { source: sourceWindow, data }));
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
    value: vi.fn(() => "blob:preview-screenshot"),
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    configurable: true,
    value: vi.fn(),
  });
});

describe("usePreviewCapture text", () => {
  it("keeps exact generated text evidence in a recoverable draft and persists it with a comment", async () => {
    const { iframe, result } = setup();

    act(() => dispatch(iframe.contentWindow, textCapture()));
    if (result.current.draft?.kind !== "text") throw new Error("expected text draft");
    expect(result.current.draft?.selected_text).toBe("$42.00");
    expect(result.current.draft?.text_anchor?.start.node_path).toEqual([0]);
    expect(result.current.draft?.text_anchor?.union_rect?.y).toBe(20);

    await act(() => result.current.saveDraft("Keep this total visible"));
    expect(feedback.create).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "text",
        comment: "Keep this total visible",
        sourceKind: "browser",
        sourceSessionId: "session-1",
        sourceLabel: "http://localhost:3000",
        pageRoute: "/checkout",
        selectedText: "$42.00",
        textAnchor: expect.objectContaining({ scroll_y: 120 }),
      }),
    );
    expect(result.current.draft).toBeNull();
  });

  it("ignores valid captures from any window except its iframe", () => {
    const { result } = setup();
    act(() => dispatch(window, textCapture()));
    expect(result.current.draft).toBeNull();
  });

  it("preserves the draft when persistence fails", async () => {
    feedback.create.mockResolvedValue(null);
    const { iframe, result } = setup();
    act(() => dispatch(iframe.contentWindow, textCapture()));
    await act(() => result.current.saveDraft("Retry this"));
    if (result.current.draft?.kind !== "text") throw new Error("expected text draft");
    expect(result.current.draft?.selected_text).toBe("$42.00");
  });
});

describe("usePreviewCapture marker projection", () => {
  it("projects task markers for the same source and follows route announcements", async () => {
    feedback.items = [
      {
        id: "feedback-1",
        task_id: "task-1",
        kind: "element",
        comment: "Move this",
        source_kind: "browser",
        source_session_id: "session-1",
        source_label: "http://localhost:3000",
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
        created_at: "2026-09-15T00:00:00Z",
        updated_at: "2026-09-15T00:00:00Z",
      },
    ];
    const { iframe, result } = setup();

    act(() =>
      dispatch(iframe.contentWindow, {
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: "route-changed",
        payload: { page_route: "/account", page_title: "Account" },
      }),
    );
    expect(result.current.pageRoute).toBe("/account");
    await waitFor(() =>
      expect(bridge.sendProjectPreviewMarkers).toHaveBeenCalledWith(iframe, [
        expect.objectContaining({ id: "feedback-1", page_route: "/account" }),
      ]),
    );
  });
});

describe("usePreviewCapture screenshots", () => {
  it("rasterizes a selected region for review, then uploads and claims it on save", async () => {
    const png = new Blob([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], {
      type: PNG_MIME_TYPE,
    });
    html2canvas.mockResolvedValue({
      width: 600,
      height: 360,
      toBlob: (callback: BlobCallback) => callback(png),
    });
    const { iframe, result } = setup();

    act(() =>
      dispatch(iframe.contentWindow, {
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: SCREENSHOT_EVENT_TYPE,
        payload: {
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
        },
      }),
    );

    await waitFor(() => expect(result.current.draft?.kind).toBe("screenshot"));
    if (result.current.draft?.kind !== "screenshot") throw new Error("expected screenshot draft");
    expect(result.current.draft?.screenshot?.previewUrl).toBe("blob:preview-screenshot");
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
    expect(result.current.draft).toBeNull();
  });
});

describe("usePreviewCapture failed screenshots", () => {
  it("keeps the screenshot and staged attachment available when create fails", async () => {
    const png = new Blob([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], {
      type: PNG_MIME_TYPE,
    });
    html2canvas.mockResolvedValue({
      width: 20,
      height: 20,
      toBlob: (callback: BlobCallback) => callback(png),
    });
    feedback.create.mockResolvedValue(null);
    const { iframe, result } = setup();
    act(() =>
      dispatch(iframe.contentWindow, {
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: SCREENSHOT_EVENT_TYPE,
        payload: {
          page_route: "/",
          page_title: "Home",
          capture_rect: { x: 0, y: 0, width: 20, height: 20 },
        },
      }),
    );
    await waitFor(() => expect(result.current.draft?.kind).toBe("screenshot"));
    await act(() => result.current.saveDraft("Retry me"));
    if (result.current.draft?.kind !== "screenshot") throw new Error("expected screenshot draft");
    expect(result.current.draft?.screenshot?.attachmentId).toBe(SCREENSHOT_ATTACHMENT_ID);
    expect(attachments.deleteAttachment).not.toHaveBeenCalled();
  });

  it("releases an unsaved screenshot before starting another capture", async () => {
    const png = new Blob([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], {
      type: PNG_MIME_TYPE,
    });
    html2canvas.mockResolvedValue({
      width: 20,
      height: 20,
      toBlob: (callback: BlobCallback) => callback(png),
    });
    feedback.create.mockResolvedValue(null);
    const { iframe, result } = setup();
    act(() =>
      dispatch(iframe.contentWindow, {
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: SCREENSHOT_EVENT_TYPE,
        payload: {
          page_route: "/",
          page_title: "Home",
          capture_rect: { x: 0, y: 0, width: 20, height: 20 },
        },
      }),
    );
    await waitFor(() => expect(result.current.draft?.kind).toBe("screenshot"));
    await act(() => result.current.saveDraft("Retry me"));

    act(() => result.current.startCapture("element"));

    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:preview-screenshot");
    expect(attachments.deleteAttachment).toHaveBeenCalledWith(SCREENSHOT_ATTACHMENT_ID);
    expect(result.current.draft).toBeNull();
  });
});

describe("usePreviewCapture cancelled uploads", () => {
  it("deletes an upload that finishes after the screenshot draft is cancelled", async () => {
    const png = new Blob([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], {
      type: PNG_MIME_TYPE,
    });
    html2canvas.mockResolvedValue({
      width: 20,
      height: 20,
      toBlob: (callback: BlobCallback) => callback(png),
    });
    let finishUpload: ((value: { attachment_id: string }) => void) | undefined;
    attachments.uploadAttachment.mockImplementation(
      () => new Promise((resolve) => (finishUpload = resolve)),
    );
    const { iframe, result } = setup();
    act(() =>
      dispatch(iframe.contentWindow, {
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: SCREENSHOT_EVENT_TYPE,
        payload: {
          page_route: "/",
          page_title: "Home",
          capture_rect: { x: 0, y: 0, width: 20, height: 20 },
        },
      }),
    );
    await waitFor(() => expect(result.current.draft?.kind).toBe("screenshot"));

    let saveResult: Promise<boolean> | undefined;
    act(() => {
      saveResult = result.current.saveDraft("Cancel me");
    });
    await waitFor(() => expect(result.current.isUploading).toBe(true));
    act(() => result.current.discardDraft());
    await act(async () => finishUpload?.({ attachment_id: "late-screenshot" }));

    await expect(saveResult).resolves.toBe(false);
    expect(attachments.deleteAttachment).toHaveBeenCalledWith("late-screenshot");
    expect(feedback.create).not.toHaveBeenCalled();
  });
});
