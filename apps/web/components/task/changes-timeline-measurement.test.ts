import { Virtualizer } from "@tanstack/react-virtual";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  captureChangesTimelineAnchor,
  resolveChangesTimelineAnchor,
  refreshChangesTimelineMeasurements,
  scrollTopForChangesTimelineAnchor,
} from "./changes-timeline-measurement";

const measureElementMock = vi.hoisted(() => vi.fn(() => 0));
vi.mock("@tanstack/react-virtual", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-virtual")>();
  return { ...actual, measureElement: measureElementMock };
});

import { measureFileTreeElement } from "./file-tree-measurement";

const measurementViewports: HTMLDivElement[] = [];

afterEach(() => measurementViewports.splice(0).forEach((viewport) => viewport.remove()));

function measurementFixture(heights: number[]) {
  const viewport = document.createElement("div");
  document.body.append(viewport);
  measurementViewports.push(viewport);
  const elements = heights.map((height, index) => {
    const element = document.createElement("div");
    element.dataset.changesTimelineRow = "";
    element.dataset.changesRowKey = `row-${index}`;
    element.dataset.index = String(index);
    Object.defineProperty(element, "offsetHeight", { get: () => height });
    viewport.append(element);
    return element;
  });
  const virtualizer = new Virtualizer<HTMLDivElement, HTMLDivElement>({
    count: 50_000,
    getScrollElement: () => null,
    estimateSize: () => 34,
    getItemKey: (index) => `row-${index}`,
    scrollToFn: () => {},
    observeElementRect: () => {},
    observeElementOffset: () => {},
    initialRect: { width: 800, height: 600 },
  });
  return { viewport, virtualizer, elements };
}

describe("Changes timeline measurement refresh", () => {
  // @covers AC-UI-BOUNDED-CHANGES-001.6
  it("restores mixed mounted heights while leaving unmounted rows estimated", () => {
    const { viewport, virtualizer } = measurementFixture([24, 52, 36]);
    refreshChangesTimelineMeasurements(viewport, virtualizer);
    expect(
      virtualizer
        .getVirtualItems()
        .slice(0, 4)
        .map(({ start, size }) => [start, size]),
    ).toEqual([
      [0, 24],
      [24, 52],
      [76, 36],
      [112, 34],
    ]);
  });

  it("retains hidden mounted rows' positive keyed sizes across invalidation", () => {
    const { viewport, virtualizer } = measurementFixture([0, 0, 0]);
    virtualizer.itemSizeCache.set("row-0", 24);
    virtualizer.itemSizeCache.set("row-1", 52);
    virtualizer.itemSizeCache.set("row-2", 0);
    refreshChangesTimelineMeasurements(viewport, virtualizer);
    expect(
      virtualizer
        .getVirtualItems()
        .slice(0, 3)
        .map(({ size }) => size),
    ).toEqual([24, 52, 34]);
  });

  // @covers AC-UI-BOUNDED-CHANGES-001.7
  it("ignores replaced keys, invalid indices, and disconnected wrappers", () => {
    const { viewport, virtualizer, elements } = measurementFixture([52, 52, 52, 52]);
    elements[0].dataset.changesRowKey = "previous-context";
    elements[1].dataset.index = "-1";
    elements[2].dataset.index = "50000";
    elements[3].remove();
    refreshChangesTimelineMeasurements(viewport, virtualizer);
    expect(
      virtualizer
        .getVirtualItems()
        .slice(0, 4)
        .map(({ size }) => size),
    ).toEqual([34, 34, 34, 34]);
  });
});

describe("Changes timeline measurement", () => {
  it("restores the same surviving row at its previous viewport offset", () => {
    const rows = [{ key: "header" }, { key: "anchor" }, { key: "next" }];
    const anchor = captureChangesTimelineAnchor(
      rows,
      [
        { index: 0, start: 0, end: 36 },
        { index: 1, start: 36, end: 64 },
      ],
      42,
    );
    expect(anchor).toEqual({ key: "anchor", index: 1, viewportOffset: -6 });

    const updated = [{ key: "inserted" }, ...rows];
    const target = resolveChangesTimelineAnchor(updated, anchor!);
    expect(target).toEqual({ key: "anchor", index: 2 });
    expect(scrollTopForChangesTimelineAnchor(72, anchor!.viewportOffset)).toBe(78);
  });

  it("uses the nearest surviving index when an anchor row is removed", () => {
    expect(
      resolveChangesTimelineAnchor([{ key: "a" }, { key: "b" }], {
        key: "removed",
        index: 2,
        viewportOffset: 8,
      }),
    ).toEqual({ key: "b", index: 1 });
    expect(
      resolveChangesTimelineAnchor([], { key: "removed", index: 0, viewportOffset: 0 }),
    ).toBeNull();
  });

  it("uses the timeline index map instead of scanning fifty thousand rows", () => {
    const rows = Array.from({ length: 50_000 }, (_, index) => ({ key: `row-${index}` }));
    const anchor: { key: string; index: number; viewportOffset: number } = {
      key: "row-49999",
      index: 0,
      viewportOffset: 0,
    };
    const indexByKey = new Map([[anchor.key, 49_999]]);
    const findIndex = vi.spyOn(rows, "findIndex");

    const resolveWithIndex = resolveChangesTimelineAnchor as unknown as (
      rows: { key: string }[],
      anchor: { key: string; index: number; viewportOffset: number },
      indexByKey: ReadonlyMap<string, number>,
    ) => { key: string; index: number } | null;
    expect(resolveWithIndex(rows, anchor, indexByKey)).toEqual({ key: anchor.key, index: 49_999 });
    expect(findIndex).not.toHaveBeenCalled();
  });

  it("retains positive geometry when a hidden row measures zero", () => {
    const instance = {
      indexFromElement: () => 3,
      options: { getItemKey: (index: number) => `row-${index}`, estimateSize: () => 28 },
      itemSizeCache: new Map([["row-3", 46]]),
    };
    measureElementMock.mockReturnValueOnce(0);

    expect(
      measureFileTreeElement({} as HTMLDivElement, {} as ResizeObserverEntry, instance as never),
    ).toBe(46);
  });

  it("uses a positive estimate when neither the measurement nor cache is positive", () => {
    const instance = {
      indexFromElement: () => 3,
      options: { getItemKey: (index: number) => `row-${index}`, estimateSize: () => 32 },
      itemSizeCache: new Map(),
    };
    measureElementMock.mockReturnValueOnce(0);

    expect(
      measureFileTreeElement({} as HTMLDivElement, {} as ResizeObserverEntry, instance as never),
    ).toBe(32);
  });

  it("prefers a current positive observer measurement", () => {
    const instance = {
      indexFromElement: () => 3,
      options: { getItemKey: (index: number) => `row-${index}`, estimateSize: () => 32 },
      itemSizeCache: new Map([["row-3", 46]]),
    };
    measureElementMock.mockReturnValueOnce(51);

    expect(
      measureFileTreeElement({} as HTMLDivElement, {} as ResizeObserverEntry, instance as never),
    ).toBe(51);
  });
});
