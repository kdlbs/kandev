import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

const SCRIPT_PATH = path.resolve(
  __dirname,
  "../../backend/internal/agentctl/server/api/scripts/inspector.js",
);
const SOURCE = "kandev-inspector";
const VERSION = 3;

const SET_CAPTURE_MODE = "set-capture-mode";
const CAPTURE_MODE_CHANGED = "capture-mode-changed";
const REGION_LAYOUT = "region-layout";
const SET_ANNOTATION_ACTIVE = "set-annotation-active";
const PROJECT_MARKERS = "project-markers";

type Sent = { source: string; version: number; type: string; payload: Record<string, unknown> };

const postMessage = vi.fn<(message: Sent) => void>();
let frames: FrameRequestCallback[] = [];

function flushFrames() {
  const pending = frames;
  frames = [];
  pending.forEach((callback) => callback(0));
}

function command(type: string, payload: Record<string, unknown> = {}) {
  window.dispatchEvent(
    new MessageEvent("message", { data: { source: SOURCE, version: VERSION, type, payload } }),
  );
}

function sentOfType(type: string) {
  return postMessage.mock.calls.map(([message]) => message).filter((m) => m.type === type);
}

function rect(left: number, top: number, width: number, height: number): DOMRect {
  return {
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => ({}),
  } as DOMRect;
}

function selectText(node: Node, start: number, end: number) {
  const range = document.createRange();
  range.setStart(node, start);
  range.setEnd(node, end);
  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
}

function pointer(type: string, x: number, y: number) {
  document.body.dispatchEvent(
    new PointerEvent(type, { clientX: x, clientY: y, pointerId: 1, bubbles: true }),
  );
}

beforeAll(() => {
  Object.defineProperty(window, "parent", { value: { postMessage }, configurable: true });
  window.requestAnimationFrame = (callback) => {
    frames.push(callback);
    return frames.length;
  };
  vi.spyOn(Range.prototype, "getClientRects").mockReturnValue([
    rect(20, 30, 80, 16),
  ] as unknown as DOMRectList);
  new Function(readFileSync(SCRIPT_PATH, "utf8"))();
});

beforeEach(() => {
  document.body.innerHTML =
    '<p id="copy">Lorem ipsum dolor</p><button id="buy">Buy</button>' +
    '<div id="list" style="overflow: auto"><span id="row">Row</span></div>';
  postMessage.mockClear();
  frames = [];
});

afterEach(() => {
  command(SET_CAPTURE_MODE, { mode: null });
  command(SET_ANNOTATION_ACTIVE, { active: false });
  command(PROJECT_MARKERS, { markers: [] });
  window.getSelection()?.removeAllRanges();
  flushFrames();
});

describe("inspector protocol 3", () => {
  it("declares protocol version 3", () => {
    command(SET_CAPTURE_MODE, { mode: "element" });
    expect(sentOfType(CAPTURE_MODE_CHANGED).at(-1)?.version).toBe(3);
  });

  it("keeps the element tool after a capture", () => {
    command(SET_CAPTURE_MODE, { mode: "element" });
    postMessage.mockClear();

    document.getElementById("buy")?.click();
    document.getElementById("copy")?.click();

    expect(sentOfType("capture-completed")).toHaveLength(2);
    expect(sentOfType(CAPTURE_MODE_CHANGED)).toHaveLength(0);
  });

  it("keeps the mode across a same-document route change", () => {
    command(SET_CAPTURE_MODE, { mode: "screenshot" });
    postMessage.mockClear();

    history.pushState({}, "", "/next");

    expect(sentOfType("route-changed")).toHaveLength(1);
    expect(sentOfType(CAPTURE_MODE_CHANGED)).toHaveLength(0);
  });

  it("routes Escape to Kandev only while annotation is active", () => {
    const outside = new KeyboardEvent("keydown", { key: "Escape", cancelable: true });
    document.dispatchEvent(outside);
    expect(sentOfType("escape")).toHaveLength(0);
    expect(outside.defaultPrevented).toBe(false);

    command(SET_ANNOTATION_ACTIVE, { active: true });
    const inside = new KeyboardEvent("keydown", { key: "Escape", cancelable: true });
    document.dispatchEvent(inside);
    expect(sentOfType("escape")).toHaveLength(1);
    expect(inside.defaultPrevented).toBe(true);

    command(SET_CAPTURE_MODE, { mode: "text" });
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", cancelable: true }));
    expect(sentOfType("escape")).toHaveLength(2);
  });
});

describe("inspector selection reporting", () => {
  it("reports selection changes once per frame without a capture tool", () => {
    const text = document.getElementById("copy")?.firstChild as Node;
    selectText(text, 6, 11);
    document.dispatchEvent(new Event("selectionchange"));
    document.dispatchEvent(new Event("selectionchange"));
    flushFrames();

    const changes = sentOfType("selection-changed");
    expect(changes).toHaveLength(1);
    expect(changes[0].payload.rect).toMatchObject({ x: 20, y: 30, width: 80, height: 16 });

    window.getSelection()?.removeAllRanges();
    document.dispatchEvent(new Event("selectionchange"));
    flushFrames();
    expect(sentOfType("selection-changed").at(-1)?.payload.rect).toBeNull();
  });

  it("ignores selection changes while a capture tool is active", () => {
    command(SET_CAPTURE_MODE, { mode: "element" });
    selectText(document.getElementById("copy")?.firstChild as Node, 0, 5);
    document.dispatchEvent(new Event("selectionchange"));
    flushFrames();

    expect(sentOfType("selection-changed")).toHaveLength(0);
  });

  it("captures the current selection on request", () => {
    selectText(document.getElementById("copy")?.firstChild as Node, 6, 11);

    command("capture-selection");

    const [capture] = sentOfType("capture-completed");
    expect(capture?.payload).toMatchObject({ kind: "text", selected_text: "ipsum" });
  });
});

describe("inspector regions and markers", () => {
  it("anchors a region to its scroll container and reports its moving layout", () => {
    const list = document.getElementById("list") as HTMLElement;
    Object.defineProperty(list, "scrollHeight", { value: 400, configurable: true });
    Object.defineProperty(list, "clientHeight", { value: 100, configurable: true });
    document.elementFromPoint = () => document.getElementById("row");
    command(SET_CAPTURE_MODE, { mode: "screenshot" });

    pointer("pointerdown", 10, 10);
    pointer("pointermove", 110, 60);
    pointer("pointerup", 110, 60);

    const [selected] = sentOfType("screenshot-region-selected");
    const captureRect = selected?.payload.capture_rect as Record<string, unknown>;
    expect(captureRect).toMatchObject({
      x: 10,
      y: 10,
      width: 100,
      height: 50,
      scroll_container_selector: "div#list",
      container_scroll_x: 0,
      container_scroll_y: 0,
    });
    expect(sentOfType(CAPTURE_MODE_CHANGED)).toHaveLength(1);

    command(PROJECT_MARKERS, {
      markers: [],
      regions: [{ id: "r1", page_route: selected?.payload.page_route, capture_rect: captureRect }],
    });
    flushFrames();
    expect(sentOfType(REGION_LAYOUT).at(-1)?.payload.regions).toEqual([
      { id: "r1", rect: { x: 10, y: 10, width: 100, height: 50 }, visible: true },
    ]);

    list.scrollTop = 30;
    list.dispatchEvent(new Event("scroll"));
    flushFrames();
    expect(sentOfType(REGION_LAYOUT).at(-1)?.payload.regions).toEqual([
      { id: "r1", rect: { x: 10, y: -20, width: 100, height: 50 }, visible: true },
    ]);

    const layouts = sentOfType(REGION_LAYOUT).length;
    list.dispatchEvent(new Event("scroll"));
    flushFrames();
    expect(sentOfType(REGION_LAYOUT)).toHaveLength(layouts);

    list.remove();
    window.dispatchEvent(new Event("resize"));
    flushFrames();
    expect(sentOfType(REGION_LAYOUT).at(-1)?.payload.regions).toEqual([
      expect.objectContaining({ id: "r1", visible: false }),
    ]);
  });

  it("draws pending markers dashed", () => {
    command(PROJECT_MARKERS, {
      markers: [
        {
          id: "m1",
          kind: "text",
          page_route: location.pathname,
          pending: true,
          capture_rect: { x: 5, y: 5, width: 10, height: 10 },
        },
      ],
    });

    const marker = document.querySelector<HTMLElement>('[data-kandev-inspector-marker="m1"]');
    expect(marker?.style.borderStyle).toBe("dashed");
  });
});

describe("inspector marker interaction", () => {
  const marker = (extra: Record<string, unknown> = {}) => ({
    id: "m1",
    kind: "text",
    page_route: location.pathname,
    capture_rect: { x: 5, y: 5, width: 10, height: 10 },
    ...extra,
  });
  const markerNode = () =>
    document.querySelector<HTMLElement>('[data-kandev-inspector-marker="m1"]');

  it("renders the number the parent assigns", () => {
    command(PROJECT_MARKERS, { markers: [marker({ number: 4 })] });
    expect(markerNode()?.textContent).toBe("4");
  });

  it("keeps markers pointer-transparent outside annotation mode", () => {
    command(PROJECT_MARKERS, { markers: [marker()] });
    expect(markerNode()?.style.pointerEvents).toBe("none");
    markerNode()?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(sentOfType("marker-activated")).toHaveLength(0);
  });

  it("reports marker clicks in annotation mode without passing them to the page", () => {
    const pageClick = vi.fn();
    document.addEventListener("click", pageClick);
    command(SET_ANNOTATION_ACTIVE, { active: true });
    command(PROJECT_MARKERS, { markers: [marker()] });

    expect(markerNode()?.style.pointerEvents).toBe("auto");
    markerNode()?.dispatchEvent(new MouseEvent("click", { bubbles: true }));

    expect(sentOfType("marker-activated").map((m) => m.payload)).toEqual([{ id: "m1" }]);
    expect(pageClick).not.toHaveBeenCalled();
    document.removeEventListener("click", pageClick);
  });

  it("re-renders markers when annotation mode changes", () => {
    command(PROJECT_MARKERS, { markers: [marker()] });
    command(SET_ANNOTATION_ACTIVE, { active: true });
    expect(markerNode()?.style.pointerEvents).toBe("auto");
  });

  it("scrolls only a revealed marker into view", () => {
    const scrollBy = vi.spyOn(window, "scrollBy").mockImplementation(() => undefined);
    const layout = vi
      .spyOn(HTMLElement.prototype, "getBoundingClientRect")
      .mockReturnValue(rect(5, 5000, 22, 22));
    const far = marker({ capture_rect: { x: 5, y: 5000, width: 10, height: 10 } });
    command(PROJECT_MARKERS, { markers: [far], highlighted_id: "m1" });
    expect(scrollBy).not.toHaveBeenCalled();
    command(PROJECT_MARKERS, { markers: [far], highlighted_id: "m1", reveal_id: "m1" });
    expect(scrollBy).toHaveBeenCalledTimes(1);
    scrollBy.mockRestore();
    layout.mockRestore();
  });

  it("highlights the requested marker", () => {
    command(PROJECT_MARKERS, { markers: [marker()], highlighted_id: "m1" });
    expect(markerNode()?.getAttribute("data-highlighted")).toBe("true");
  });

  it("reports page presses only while a comment is open", () => {
    command(SET_ANNOTATION_ACTIVE, { active: true });
    pointer("pointerdown", 50, 50);
    expect(sentOfType("page-pointerdown")).toHaveLength(0);

    command(SET_ANNOTATION_ACTIVE, { active: true, surface_open: true });
    pointer("pointerdown", 50, 50);
    expect(sentOfType("page-pointerdown")).toHaveLength(1);
  });
});

describe("inspector region drags over markers", () => {
  it("starts a region drag on a marker in the Region tool", () => {
    command(SET_ANNOTATION_ACTIVE, { active: true });
    command(SET_CAPTURE_MODE, { mode: "screenshot" });
    command(PROJECT_MARKERS, {
      markers: [
        {
          id: "m1",
          kind: "text",
          page_route: location.pathname,
          capture_rect: { x: 30, y: 30, width: 10, height: 10 },
        },
      ],
    });
    const node = document.querySelector('[data-kandev-inspector-marker="m1"]') as HTMLElement;
    node.dispatchEvent(
      new PointerEvent("pointerdown", { clientX: 25, clientY: 25, pointerId: 1, bubbles: true }),
    );
    pointer("pointermove", 120, 140);
    pointer("pointerup", 120, 140);
    expect(sentOfType("screenshot-region-selected")).toHaveLength(1);
  });
});
