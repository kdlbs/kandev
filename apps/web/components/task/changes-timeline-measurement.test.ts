import { describe, expect, it, vi } from "vitest";
import {
  captureChangesTimelineAnchor,
  resolveChangesTimelineAnchor,
  scrollTopForChangesTimelineAnchor,
} from "./changes-timeline-measurement";

const measureElementMock = vi.hoisted(() => vi.fn(() => 0));
vi.mock("@tanstack/react-virtual", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-virtual")>();
  return { ...actual, measureElement: measureElementMock };
});

import { measureFileTreeElement } from "./file-tree-measurement";

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
