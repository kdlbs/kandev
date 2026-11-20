import { afterEach, describe, expect, it, vi } from "vitest";
import {
  PROJECT_MARKERS,
  command,
  rect,
  useInspectorPage,
} from "./preview-inspector-script.test-helpers";

useInspectorPage();

const HIGHLIGHT_NAME = "kandev-comment-target";

const textMarker = (extra: Record<string, unknown> = {}) => ({
  id: "m1",
  kind: "text",
  page_route: location.pathname,
  selected_text: "ipsum",
  text_anchor: {
    start: { selector: "#copy", node_path: [0], offset: 6 },
    end: { selector: "#copy", node_path: [0], offset: 11 },
    rects: [{ x: 1, y: 2, width: 3, height: 4, document_x: 1, document_y: 2 }],
    union_rect: { x: 1, y: 2, width: 3, height: 4, document_x: 1, document_y: 2 },
  },
  ...extra,
});

const markerNode = () => document.querySelector<HTMLElement>('[data-kandev-inspector-marker="m1"]');

function hoverMarker() {
  markerNode()?.dispatchEvent(new MouseEvent("mouseover", { bubbles: true }));
}

function boxes() {
  return Array.from(
    document.querySelectorAll<HTMLElement>('[data-kandev-inspector-ui="target-highlight"]'),
  ).map((box) => ({
    left: box.style.left,
    top: box.style.top,
    width: box.style.width,
    height: box.style.height,
  }));
}

afterEach(() => vi.unstubAllGlobals());

describe("inspector marker click", () => {
  it("reports where the clicked marker's target is", () => {
    command(PROJECT_MARKERS, {
      markers: [
        {
          id: "m1",
          kind: "element",
          page_route: location.pathname,
          capture_rect: { x: 5, y: 5, width: 10, height: 10, document_x: 5, document_y: 5 },
        },
      ],
    });
    markerNode()?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(markerActivations()).toEqual([{ id: "m1", rect: { x: 5, y: 5, width: 0, height: 0 } }]);
  });
});

function markerActivations() {
  return (
    window.parent.postMessage as unknown as {
      mock: { calls: Array<[{ type: string; payload: unknown }]> };
    }
  ).mock.calls
    .map(([message]) => message)
    .filter((message) => message.type === "marker-activated")
    .map((message) => message.payload);
}

describe("inspector target highlight", () => {
  it("paints a hovered text comment's text with the page's highlight registry", () => {
    const registry = new Map<string, Set<Range>>();
    vi.stubGlobal("CSS", { highlights: registry });
    vi.stubGlobal("Highlight", class extends Set<Range> {});
    command(PROJECT_MARKERS, { markers: [textMarker()] });

    hoverMarker();

    const painted = Array.from(registry.get(HIGHLIGHT_NAME) ?? []);
    expect(painted.map((range) => range.toString())).toEqual(["ipsum"]);
    expect(boxes()).toEqual([]);

    document.getElementById("copy")?.dispatchEvent(new MouseEvent("mouseover", { bubbles: true }));
    expect(registry.has(HIGHLIGHT_NAME)).toBe(false);
  });

  it("outlines the text's current boxes without a highlight registry", () => {
    command(PROJECT_MARKERS, { markers: [textMarker()] });
    hoverMarker();
    expect(boxes()).toEqual([{ left: "20px", top: "30px", width: "80px", height: "16px" }]);
  });

  it("falls back to the saved boxes when the text has changed", () => {
    command(PROJECT_MARKERS, { markers: [textMarker({ selected_text: "changed" })] });
    hoverMarker();
    expect(boxes()).toEqual([{ left: "1px", top: "2px", width: "3px", height: "4px" }]);
  });

  it("highlights nothing when neither the text nor saved boxes are found", () => {
    const marker = textMarker({ selected_text: "changed" });
    command(PROJECT_MARKERS, {
      markers: [{ ...marker, text_anchor: { ...marker.text_anchor, rects: [] } }],
    });
    hoverMarker();
    expect(boxes()).toEqual([]);
  });

  it("outlines a hovered element comment's element", () => {
    const layout = vi
      .spyOn(HTMLButtonElement.prototype, "getBoundingClientRect")
      .mockReturnValue(rect(40, 50, 60, 20));
    command(PROJECT_MARKERS, {
      markers: [
        {
          id: "m1",
          kind: "element",
          page_route: location.pathname,
          element_snapshot: { selector: "#buy" },
        },
      ],
    });
    hoverMarker();
    expect(boxes()).toEqual([{ left: "40px", top: "50px", width: "60px", height: "20px" }]);
    layout.mockRestore();
  });

  it("clears the highlight when the pointer leaves the page", () => {
    command(PROJECT_MARKERS, { markers: [textMarker()] });
    hoverMarker();
    markerNode()?.dispatchEvent(new MouseEvent("mouseout", { bubbles: true, relatedTarget: null }));
    expect(boxes()).toEqual([]);
  });

  it("keeps the open comment's target highlighted until it closes", () => {
    command(PROJECT_MARKERS, { markers: [textMarker()], highlighted_id: "m1" });
    expect(boxes()).toHaveLength(1);
    command(PROJECT_MARKERS, { markers: [textMarker()] });
    expect(boxes()).toEqual([]);
  });

  it("does not highlight region captures", () => {
    command(PROJECT_MARKERS, {
      markers: [
        {
          id: "m1",
          kind: "screenshot",
          page_route: location.pathname,
          capture_rect: { x: 5, y: 5, width: 10, height: 10 },
        },
      ],
      highlighted_id: "m1",
    });
    hoverMarker();
    expect(boxes()).toEqual([]);
  });
});
