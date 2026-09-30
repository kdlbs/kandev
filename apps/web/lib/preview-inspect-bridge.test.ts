import { describe, expect, it, vi } from "vitest";
import {
  INSPECTOR_PROTOCOL_VERSION,
  INSPECTOR_SOURCE,
  isInspectorMessage,
  sendProjectPreviewMarkers,
  sendSetPreviewCaptureMode,
} from "./preview-inspect-bridge";

const RUNTIME_SELECTOR = "#runtime-total";
const RUNTIME_TOTAL = "$42.00";

const renderedElement = {
  tag: "span",
  id: "runtime-total",
  classes: ["price"],
  role: "status",
  accessible_label: "Cart total",
  visible_text: RUNTIME_TOTAL,
  selector: RUNTIME_SELECTOR,
  outer_html: '<span id="runtime-total" class="price">$42.00</span>',
};

function textCaptureMessage() {
  return {
    source: INSPECTOR_SOURCE,
    version: INSPECTOR_PROTOCOL_VERSION,
    type: "capture-completed",
    payload: {
      kind: "text",
      page_route: "/checkout?step=review",
      page_title: "Checkout",
      selected_text: RUNTIME_TOTAL,
      text_anchor: {
        start: { selector: RUNTIME_SELECTOR, node_path: [0], offset: 0 },
        end: { selector: RUNTIME_SELECTOR, node_path: [0], offset: 6 },
        rects: [{ x: 120, y: 340, width: 58, height: 20 }],
        union_rect: { x: 120, y: 340, width: 58, height: 20 },
        scroll_x: 0,
        scroll_y: 260,
        viewport_width: 1280,
        viewport_height: 720,
        device_pixel_ratio: 2,
        containing_element: renderedElement,
      },
    },
  };
}

describe("preview inspector protocol validation", () => {
  it("accepts script-generated text with its rendered DOM range and position", () => {
    const message = textCaptureMessage();

    expect(isInspectorMessage(message)).toBe(true);
    if (!isInspectorMessage(message) || message.type !== "capture-completed") return;
    expect(message.payload.text_anchor?.containing_element).toEqual(renderedElement);
    expect(message.payload.text_anchor?.start).toEqual({
      selector: RUNTIME_SELECTOR,
      node_path: [0],
      offset: 0,
    });
    expect(message.payload.text_anchor?.union_rect).toEqual({
      x: 120,
      y: 340,
      width: 58,
      height: 20,
    });
    expect(message.payload.text_anchor?.scroll_y).toBe(260);
  });

  it("rejects capture events with the wrong protocol version or incomplete anchors", () => {
    expect(isInspectorMessage({ ...textCaptureMessage(), version: 1 })).toBe(false);
    const missingEndpoint = textCaptureMessage();
    delete (missingEndpoint.payload.text_anchor as { end?: unknown }).end;
    expect(isInspectorMessage(missingEndpoint)).toBe(false);
  });

  it("rejects oversized rendered HTML and invalid rectangle numbers", () => {
    const oversized = textCaptureMessage();
    oversized.payload.text_anchor.containing_element.outer_html = "x".repeat(70_000);
    expect(isInspectorMessage(oversized)).toBe(false);

    const invalidRect = textCaptureMessage();
    invalidRect.payload.text_anchor.rects[0]!.width = Number.POSITIVE_INFINITY;
    expect(isInspectorMessage(invalidRect)).toBe(false);
  });

  it("accepts bounded candidate and route announcements", () => {
    expect(
      isInspectorMessage({
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: "candidate-changed",
        payload: { label: "button#save.primary" },
      }),
    ).toBe(true);
    expect(
      isInspectorMessage({
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: "route-changed",
        payload: { page_route: "/account", page_title: "Account" },
      }),
    ).toBe(true);
    expect(
      isInspectorMessage({
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: "screenshot-region-selected",
        payload: {
          page_route: "/account",
          page_title: "Account",
          capture_rect: {
            x: 20,
            y: 40,
            width: 300,
            height: 180,
            document_x: 20,
            document_y: 640,
            scroll_x: 0,
            scroll_y: 600,
            viewport_width: 390,
            viewport_height: 844,
            device_pixel_ratio: 3,
          },
        },
      }),
    ).toBe(true);
  });
});

describe("preview inspector commands", () => {
  function iframeWithPostMessage() {
    const postMessage = vi.fn();
    const iframe = { contentWindow: { postMessage } } as unknown as HTMLIFrameElement;
    return { iframe, postMessage };
  }

  it("starts one explicit selection mode", () => {
    const { iframe, postMessage } = iframeWithPostMessage();
    sendSetPreviewCaptureMode(iframe, "element");
    expect(postMessage).toHaveBeenCalledWith(
      {
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: "set-capture-mode",
        payload: { mode: "element" },
      },
      "*",
    );
  });

  it("projects only immutable marker evidence", () => {
    const { iframe, postMessage } = iframeWithPostMessage();
    sendProjectPreviewMarkers(iframe, [
      {
        id: "feedback-1",
        kind: "element",
        page_route: "/checkout",
        element_snapshot: renderedElement,
        capture_rect: { x: 10, y: 20, width: 100, height: 40 },
      },
    ]);
    expect(postMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        version: INSPECTOR_PROTOCOL_VERSION,
        type: "project-markers",
        payload: { markers: [expect.objectContaining({ id: "feedback-1" })] },
      }),
      "*",
    );
  });
});
