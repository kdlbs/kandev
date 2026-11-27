import { describe, expect, it, vi } from "vitest";
import {
  INSPECTOR_PROTOCOL_VERSION,
  INSPECTOR_SOURCE,
  isInspectorMessage,
  sendCaptureSelection,
  sendProjectPreviewMarkers,
  sendSetAnnotationActive,
  sendSetPreviewCaptureMode,
} from "./preview-inspect-bridge";

const RUNTIME_SELECTOR = "#runtime-total";
const RUNTIME_TOTAL = "$42.00";
const PREVIEW_ORIGIN = "http://preview.example";

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

  it("accepts capture-mode readiness only for supported modes", () => {
    for (const mode of ["text", "element", "screenshot", null]) {
      expect(
        isInspectorMessage({
          source: INSPECTOR_SOURCE,
          version: INSPECTOR_PROTOCOL_VERSION,
          type: "capture-mode-changed",
          payload: { mode },
        }),
      ).toBe(true);
    }
    expect(
      isInspectorMessage({
        source: INSPECTOR_SOURCE,
        version: INSPECTOR_PROTOCOL_VERSION,
        type: "capture-mode-changed",
        payload: { mode: "unknown" },
      }),
    ).toBe(false);
  });
});

describe("preview inspector commands", () => {
  function iframeWithPostMessage() {
    const postMessage = vi.fn();
    const iframe = {
      src: "http://preview.example/path",
      contentWindow: { postMessage },
    } as unknown as HTMLIFrameElement;
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
      PREVIEW_ORIGIN,
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
        payload: {
          markers: [expect.objectContaining({ id: "feedback-1" })],
          regions: [],
          highlighted_id: null,
          reveal_id: null,
        },
      }),
      PREVIEW_ORIGIN,
    );
  });

  it("pins marker evidence to the configured preview origin", () => {
    const postMessage = vi.fn();
    const iframe = {
      src: "https://preview.example/path",
      contentWindow: { postMessage },
    } as unknown as HTMLIFrameElement;

    sendProjectPreviewMarkers(iframe, [
      { id: "feedback-1", kind: "text", page_route: "/checkout", selected_text: "total" },
    ]);

    expect(postMessage).toHaveBeenCalledWith(expect.any(Object), "https://preview.example");
  });

  it("pins inspector state commands to the configured preview origin", () => {
    const { iframe, postMessage } = iframeWithPostMessage();
    sendSetAnnotationActive(iframe, true);
    sendCaptureSelection(iframe);
    expect(postMessage.mock.calls.map(([, targetOrigin]) => targetOrigin)).toEqual([
      PREVIEW_ORIGIN,
      PREVIEW_ORIGIN,
    ]);
  });

  it("sends annotation state, selection capture, and region anchors", () => {
    const postMessage = vi.fn();
    const iframe = {
      src: "http://preview.example/path",
      contentWindow: { postMessage },
    } as unknown as HTMLIFrameElement;
    const region = { x: 10, y: 20, width: 100, height: 40 };

    sendSetAnnotationActive(iframe, true);
    sendCaptureSelection(iframe);
    sendProjectPreviewMarkers(iframe, [], [{ id: "r1", page_route: "/", capture_rect: region }]);

    expect(postMessage.mock.calls.map(([sent]) => [sent.type, sent.payload])).toEqual([
      ["set-annotation-active", { active: true, surface_open: false }],
      ["capture-selection", {}],
      [
        "project-markers",
        {
          markers: [],
          regions: [{ id: "r1", page_route: "/", capture_rect: region }],
          highlighted_id: null,
          reveal_id: null,
        },
      ],
    ]);
  });
});

function message(type: string, payload: unknown, version: number = INSPECTOR_PROTOCOL_VERSION) {
  return { source: INSPECTOR_SOURCE, version, type, payload };
}

const REGION_LAYOUT = "region-layout";
const MARKER_ACTIVATED = "marker-activated";

describe("preview inspector protocol 3", () => {
  const region = { x: 10, y: 20, width: 100, height: 40 };

  it("uses version 3", () => {
    expect(INSPECTOR_PROTOCOL_VERSION).toBe(3);
    expect(isInspectorMessage(message("escape", {}, 2))).toBe(false);
  });

  it("accepts selection changes with a rectangle or null", () => {
    expect(isInspectorMessage(message("selection-changed", { rect: region }))).toBe(true);
    expect(isInspectorMessage(message("selection-changed", { rect: null }))).toBe(true);
    expect(isInspectorMessage(message("selection-changed", { rect: { x: "1" } }))).toBe(false);
  });

  it("accepts escape and rejects the retired capture-cancelled event", () => {
    expect(isInspectorMessage(message("escape", {}))).toBe(true);
    expect(isInspectorMessage(message("capture-cancelled", {}))).toBe(false);
    expect(isInspectorMessage(message("toString", {}))).toBe(false);
  });

  it("accepts bounded region layouts", () => {
    const entry = { id: "r1", rect: region, visible: true };
    expect(isInspectorMessage(message(REGION_LAYOUT, { regions: [entry] }))).toBe(true);
    expect(isInspectorMessage(message(REGION_LAYOUT, { regions: Array(33).fill(entry) }))).toBe(
      false,
    );
    expect(
      isInspectorMessage(message(REGION_LAYOUT, { regions: [{ ...entry, visible: "yes" }] })),
    ).toBe(false);
    expect(
      isInspectorMessage(message(REGION_LAYOUT, { regions: [{ ...entry, id: "x".repeat(200) }] })),
    ).toBe(false);
  });

  it("accepts scroll-container anchors on a selected region", () => {
    const selected = (capture_rect: unknown) =>
      message("screenshot-region-selected", { page_route: "/", page_title: "", capture_rect });
    expect(
      isInspectorMessage(
        selected({
          ...region,
          scroll_container_selector: "div#list",
          container_scroll_x: 0,
          container_scroll_y: 120,
        }),
      ),
    ).toBe(true);
    expect(
      isInspectorMessage(selected({ ...region, scroll_container_selector: "x".repeat(5000) })),
    ).toBe(false);
    expect(isInspectorMessage(selected({ ...region, container_scroll_y: Number.NaN }))).toBe(false);
  });

  it("accepts marker activation and page presses", () => {
    expect(isInspectorMessage(message(MARKER_ACTIVATED, { id: "item-1" }))).toBe(true);
    expect(isInspectorMessage(message(MARKER_ACTIVATED, { id: "" }))).toBe(false);
    expect(isInspectorMessage(message(MARKER_ACTIVATED, { id: "x".repeat(200) }))).toBe(false);
    expect(isInspectorMessage(message(MARKER_ACTIVATED, {}))).toBe(false);
    const markerRect = { x: 5, y: 6, width: 0, height: 0 };
    expect(isInspectorMessage(message(MARKER_ACTIVATED, { id: "a", rect: markerRect }))).toBe(true);
    expect(
      isInspectorMessage(message(MARKER_ACTIVATED, { id: "a", rect: { ...markerRect, x: "5" } })),
    ).toBe(false);
    expect(isInspectorMessage(message("page-pointerdown", {}))).toBe(true);
  });

  it("sends an open comment and a highlighted marker", () => {
    const postMessage = vi.fn();
    const iframe = {
      src: "http://preview.example/path",
      contentWindow: { postMessage },
    } as unknown as HTMLIFrameElement;

    sendSetAnnotationActive(iframe, true, true);
    sendProjectPreviewMarkers(
      iframe,
      [{ id: "m1", kind: "text", page_route: "/", number: 2 }],
      [],
      "m1",
    );

    expect(postMessage.mock.calls.map(([sent]) => sent.payload)).toEqual([
      { active: true, surface_open: true },
      {
        markers: [{ id: "m1", kind: "text", page_route: "/", number: 2 }],
        regions: [],
        highlighted_id: "m1",
        reveal_id: null,
      },
    ]);
  });
});

describe("preview region anchors", () => {
  const region = { x: 10, y: 20, width: 100, height: 40 };
  const anchor = {
    anchor_selector: "section#card",
    anchor_offset_x: 10,
    anchor_offset_y: 20,
    anchor_width: 300,
    anchor_height: 200,
  };

  it("accepts drift, a highlighted marker, and anchored regions", () => {
    const entry = { id: "r1", rect: region, visible: true, drifted: true };
    expect(
      isInspectorMessage(
        message(REGION_LAYOUT, { regions: [entry], highlighted: { id: "m1", rect: region } }),
      ),
    ).toBe(true);
    expect(
      isInspectorMessage(message(REGION_LAYOUT, { regions: [entry], highlighted: null })),
    ).toBe(true);
    expect(
      isInspectorMessage(message(REGION_LAYOUT, { regions: [{ ...entry, drifted: "yes" }] })),
    ).toBe(false);
    expect(
      isInspectorMessage(message(REGION_LAYOUT, { regions: [], highlighted: { id: "m1" } })),
    ).toBe(false);

    const selected = (capture_rect: unknown) =>
      message("screenshot-region-selected", { page_route: "/", page_title: "", capture_rect });
    expect(isInspectorMessage(selected({ ...region, ...anchor }))).toBe(true);
    expect(
      isInspectorMessage(selected({ ...region, ...anchor, anchor_selector: "x".repeat(5000) })),
    ).toBe(false);
    expect(isInspectorMessage(selected({ ...region, ...anchor, anchor_width: "300" }))).toBe(false);
  });

  it("accepts anchor replies with an anchor or none", () => {
    expect(isInspectorMessage(message("region-anchored", { request_id: "a", anchor }))).toBe(true);
    expect(isInspectorMessage(message("region-anchored", { request_id: "a", anchor: null }))).toBe(
      true,
    );
    expect(
      isInspectorMessage(message("region-anchored", { request_id: "a", anchor: { x: 1 } })),
    ).toBe(false);
  });
});

describe("preview region layout clips", () => {
  const region = { x: 10, y: 20, width: 100, height: 40 };
  const entry = { id: "r1", rect: region, visible: true };

  it("accepts a valid clip box and rejects a malformed one", () => {
    const layout = (clip: unknown) => message(REGION_LAYOUT, { regions: [{ ...entry, clip }] });
    expect(isInspectorMessage(layout(region))).toBe(true);
    expect(isInspectorMessage(layout({ ...region, width: -1 }))).toBe(false);
  });
});
