import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  CAPTURE_MODE_CHANGED,
  PROJECT_MARKERS,
  REGION_LAYOUT,
  SET_ANNOTATION_ACTIVE,
  SET_CAPTURE_MODE,
  command,
  flushFrames,
  pointer,
  postMessage,
  rect,
  sentOfType,
  useInspectorPage,
} from "./preview-inspector-script.test-helpers";

useInspectorPage();

function selectText(node: Node, start: number, end: number) {
  const range = document.createRange();
  range.setStart(node, start);
  range.setEnd(node, end);
  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
}

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

  it("leaves Kandev preview parameters out of the page route", () => {
    postMessage.mockClear();

    history.pushState({}, "", "/site/index.html?kandev_preview_version=7&tab=pro");

    expect(sentOfType("route-changed").at(-1)?.payload.page_route).toBe("/site/index.html?tab=pro");
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
    const listBox = vi.spyOn(list, "getBoundingClientRect").mockReturnValue(rect(0, 0, 300, 100));
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
      { id: "r1", rect: { x: 10, y: 10, width: 100, height: 50 }, visible: true, drifted: false },
    ]);

    list.scrollTop = 30;
    list.dispatchEvent(new Event("scroll"));
    flushFrames();
    expect(sentOfType(REGION_LAYOUT).at(-1)?.payload.regions).toEqual([
      {
        id: "r1",
        rect: { x: 10, y: -20, width: 100, height: 50 },
        visible: true,
        drifted: false,
        clip: { x: 0, y: 0, width: 300, height: 100 },
      },
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
    listBox.mockRestore();
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

  it.each([false, true])(
    "reports marker clicks without passing them to the page (annotation mode %s)",
    (active) => {
      const pageClick = vi.fn();
      document.addEventListener("click", pageClick);
      command(SET_ANNOTATION_ACTIVE, { active });
      command(PROJECT_MARKERS, { markers: [marker()] });

      expect(markerNode()?.style.pointerEvents).toBe("auto");
      markerNode()?.dispatchEvent(new MouseEvent("click", { bubbles: true }));

      expect(sentOfType("marker-activated").map((m) => m.payload.id)).toEqual(["m1"]);
      expect(pageClick).not.toHaveBeenCalled();
      document.removeEventListener("click", pageClick);
    },
  );

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

    command(SET_ANNOTATION_ACTIVE, { active: false, surface_open: true });
    pointer("pointerdown", 50, 50);
    expect(sentOfType("page-pointerdown")).toHaveLength(2);
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

const ANCHOR = "section#card";
let restoreRects: Array<() => void> = [];

function placeElement(id: string, box: DOMRect) {
  const element = document.getElementById(id) as HTMLElement;
  const spy = vi.spyOn(element, "getBoundingClientRect").mockReturnValue(box);
  restoreRects.push(() => spy.mockRestore());
  return element;
}

function lastLayout() {
  return sentOfType(REGION_LAYOUT).at(-1)?.payload as {
    regions: Array<Record<string, unknown>>;
    highlighted: unknown;
  };
}

function drawRegion() {
  command(SET_CAPTURE_MODE, { mode: "screenshot" });
  pointer("pointerdown", 110, 120);
  pointer("pointermove", 210, 170);
  pointer("pointerup", 210, 170);
  return sentOfType("screenshot-region-selected").at(-1)?.payload as {
    page_route: string;
    capture_rect: Record<string, unknown>;
  };
}

function project(captureRect: Record<string, unknown>) {
  command(PROJECT_MARKERS, {
    markers: [],
    regions: [{ id: "r1", page_route: location.pathname, capture_rect: captureRect }],
  });
  flushFrames();
}

function setupAnchorFixture() {
  beforeEach(() => {
    document.body.innerHTML =
      '<main id="page"><section id="card"><span id="label">Plan</span></section></main>' +
      '<div id="list" style="overflow: auto"><span id="row">Row</span></div>';
    document.elementFromPoint = () => document.getElementById("label");
    placeElement("label", rect(120, 130, 40, 16));
    placeElement("card", rect(100, 100, 300, 200));
    placeElement("page", rect(0, 0, 1024, 900));
  });

  afterEach(() => {
    restoreRects.forEach((restore) => restore());
    restoreRects = [];
  });
}

describe("inspector region anchors", () => {
  setupAnchorFixture();

  it("anchors a region to the smallest element that contains it", () => {
    const selected = drawRegion();
    expect(selected.capture_rect).toMatchObject({
      anchor_selector: ANCHOR,
      anchor_offset_x: 10,
      anchor_offset_y: 20,
      anchor_width: 300,
      anchor_height: 200,
    });
  });

  it("never anchors to the body", () => {
    placeElement("card", rect(100, 100, 50, 50));
    placeElement("page", rect(100, 100, 50, 50));
    const selected = drawRegion();
    expect(selected.capture_rect.anchor_selector).toBeUndefined();
  });

  it("follows its anchor when the layout moves it", () => {
    const { capture_rect: captureRect } = drawRegion();
    project(captureRect);
    expect(lastLayout().regions[0]).toEqual({
      id: "r1",
      rect: { x: 110, y: 120, width: 100, height: 50 },
      visible: true,
      drifted: false,
    });

    placeElement("card", rect(40, 100, 300, 200));
    window.dispatchEvent(new Event("resize"));
    flushFrames();
    expect(lastLayout().regions[0]).toMatchObject({
      rect: { x: 50, y: 120, width: 100, height: 50 },
      drifted: false,
    });
  });

  it("lays out again after a DOM change", async () => {
    const { capture_rect: captureRect } = drawRegion();
    project(captureRect);
    placeElement("card", rect(100, 160, 300, 200));
    document.getElementById("page")?.prepend(document.createElement("p"));
    await Promise.resolve();
    flushFrames();
    expect(lastLayout().regions[0]).toMatchObject({ rect: { x: 110, y: 180 } });
  });
});

describe("inspector region drift", () => {
  setupAnchorFixture();

  it("flags drift when the anchor changes size and keeps following its corner", () => {
    const { capture_rect: captureRect } = drawRegion();
    placeElement("card", rect(100, 100, 200, 260));
    project(captureRect);
    expect(lastLayout().regions[0]).toMatchObject({
      rect: { x: 110, y: 120 },
      drifted: true,
    });
  });

  it("tolerates small anchor size changes", () => {
    const { capture_rect: captureRect } = drawRegion();
    placeElement("card", rect(100, 100, 306, 204));
    project(captureRect);
    expect(lastLayout().regions[0]).toMatchObject({ drifted: false });
  });

  it("flags drift and falls back to document coordinates when the anchor is gone", () => {
    const { capture_rect: captureRect } = drawRegion();
    document.getElementById("card")?.remove();
    project(captureRect);
    expect(lastLayout().regions[0]).toMatchObject({
      rect: { x: 110, y: 120, width: 100, height: 50 },
      visible: true,
      drifted: true,
    });
  });

  it("flags drift for an unanchored region after a width change", () => {
    project({
      x: 10,
      y: 10,
      width: 50,
      height: 50,
      document_x: 10,
      document_y: 10,
      viewport_width: window.innerWidth + 200,
    });
    expect(lastLayout().regions[0]).toMatchObject({ drifted: true });
  });

  it("subtracts container scroll when the anchor is the scroll container", () => {
    const list = placeElement("list", rect(0, 400, 300, 100));
    project({
      x: 10,
      y: 410,
      width: 50,
      height: 20,
      scroll_container_selector: "div#list",
      container_scroll_x: 0,
      container_scroll_y: 0,
      anchor_selector: "div#list",
      anchor_offset_x: 10,
      anchor_offset_y: 10,
      anchor_width: 300,
      anchor_height: 100,
    });
    list.scrollTop = 30;
    list.dispatchEvent(new Event("scroll"));
    flushFrames();
    expect(lastLayout().regions[0]).toMatchObject({ rect: { x: 10, y: 380 } });
  });
});

describe("inspector anchor requests", () => {
  setupAnchorFixture();

  it("answers anchor requests for a moved region", () => {
    command("anchor-region", {
      request_id: "req-1",
      rect: { x: 110, y: 120, width: 100, height: 50 },
    });
    expect(sentOfType("region-anchored").at(-1)?.payload).toEqual({
      request_id: "req-1",
      anchor: {
        anchor_selector: ANCHOR,
        anchor_offset_x: 10,
        anchor_offset_y: 20,
        anchor_width: 300,
        anchor_height: 200,
      },
      container: null,
    });

    document.elementFromPoint = () => document.body;
    command("anchor-region", { request_id: "req-2", rect: { x: 1, y: 1, width: 5, height: 5 } });
    expect(sentOfType("region-anchored").at(-1)?.payload).toEqual({
      request_id: "req-2",
      anchor: null,
      container: null,
    });
  });

  it("reports the scroll container a moved region now sits in", () => {
    const list = document.getElementById("list") as HTMLElement;
    Object.defineProperty(list, "scrollHeight", { value: 400, configurable: true });
    Object.defineProperty(list, "clientHeight", { value: 100, configurable: true });
    list.scrollTop = 15;
    document.elementFromPoint = () => document.getElementById("row");
    command("anchor-region", { request_id: "req-3", rect: { x: 1, y: 1, width: 5, height: 5 } });
    expect(sentOfType("region-anchored").at(-1)?.payload.container).toEqual({
      scroll_container_selector: "div#list",
      container_scroll_x: 0,
      container_scroll_y: 15,
    });
  });

  it("reports where the highlighted marker is", () => {
    command(PROJECT_MARKERS, {
      markers: [
        {
          id: "m1",
          kind: "text",
          page_route: location.pathname,
          capture_rect: { x: 5, y: 15, width: 10, height: 10, document_x: 5, document_y: 15 },
        },
      ],
      highlighted_id: "m1",
    });
    flushFrames();
    expect(lastLayout().highlighted).toEqual({
      id: "m1",
      rect: { x: 5, y: 15, width: 0, height: 0 },
    });
  });
});

describe("inspector region scroll containers", () => {
  setupAnchorFixture();

  it("scrolls a revealed region into view inside its container and the page", () => {
    const list = placeElement("list", rect(0, 400, 300, 100));
    const listScroll = vi.fn();
    list.scrollBy = listScroll as unknown as typeof list.scrollBy;
    const pageScroll = vi.spyOn(window, "scrollBy").mockImplementation(() => undefined);
    const inList = {
      x: 10,
      y: 700,
      width: 50,
      height: 20,
      document_x: 10,
      document_y: 700,
      scroll_container_selector: "div#list",
      container_scroll_x: 0,
      container_scroll_y: 0,
    };
    const regions = [{ id: "r1", page_route: location.pathname, capture_rect: inList }];
    command(PROJECT_MARKERS, { markers: [], regions, reveal_id: "r1" });

    expect(listScroll).toHaveBeenCalledWith(expect.objectContaining({ top: 260 }));
    expect(pageScroll).not.toHaveBeenCalled();

    const far = { ...inList, y: 5000, document_y: 5000, scroll_container_selector: undefined };
    command(PROJECT_MARKERS, {
      markers: [],
      regions: [{ ...regions[0], id: "r2", capture_rect: far }],
      reveal_id: "r2",
    });
    expect(pageScroll).toHaveBeenCalledTimes(1);
    expect(pageScroll.mock.calls[0][0]).toMatchObject({ top: expect.any(Number) });
    pageScroll.mockRestore();
  });

  it("clips a region to its scroll container and hides it once scrolled out", () => {
    const list = placeElement("list", rect(0, 400, 300, 100));
    project({
      x: 10,
      y: 410,
      width: 50,
      height: 20,
      scroll_container_selector: "div#list",
      container_scroll_x: 0,
      container_scroll_y: 0,
      anchor_selector: "div#list",
      anchor_offset_x: 10,
      anchor_offset_y: 10,
      anchor_width: 300,
      anchor_height: 100,
    });
    expect(lastLayout().regions[0]).toMatchObject({ visible: true });
    expect(lastLayout().regions[0].clip).toBeUndefined();

    list.scrollTop = 20;
    list.dispatchEvent(new Event("scroll"));
    flushFrames();
    expect(lastLayout().regions[0]).toMatchObject({
      visible: true,
      clip: { x: 0, y: 400, width: 300, height: 100 },
    });

    list.scrollTop = 40;
    list.dispatchEvent(new Event("scroll"));
    flushFrames();
    expect(lastLayout().regions[0]).toMatchObject({ visible: false });
  });
});
