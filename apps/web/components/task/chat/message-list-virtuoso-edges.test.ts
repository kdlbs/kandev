import { describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import {
  findMessageItemIndex,
  resolveVirtuosoEdgeState,
  type RangePosition,
} from "./message-list-virtuoso-edges";

function elementWithRect(top: number, bottom: number): HTMLElement {
  const el = document.createElement("div");
  el.getBoundingClientRect = () => ({ top, bottom }) as DOMRect;
  return el;
}

function message(id: string): Message {
  return { id } as Message;
}

describe("findMessageItemIndex", () => {
  it("returns -1 for a null or undefined id", () => {
    const items: RenderItem[] = [{ type: "message", message: message("m1") }];

    expect(findMessageItemIndex(items, null)).toBe(-1);
    expect(findMessageItemIndex(items, undefined)).toBe(-1);
  });

  it("finds a plain message item by id", () => {
    const items: RenderItem[] = [
      { type: "message", message: message("m1") },
      { type: "message", message: message("m2") },
    ];

    expect(findMessageItemIndex(items, "m2")).toBe(1);
  });

  it("finds a message nested inside a turn_group item", () => {
    const items: RenderItem[] = [
      { type: "message", message: message("m1") },
      {
        type: "turn_group",
        id: "group-1",
        turnId: "turn-1",
        messages: [message("m2"), message("m3")],
      },
    ];

    expect(findMessageItemIndex(items, "m3")).toBe(1);
  });

  it("returns -1 when the id doesn't match any item", () => {
    const items: RenderItem[] = [
      { type: "message", message: message("m1") },
      { type: "turn_group", id: "group-1", turnId: "turn-1", messages: [message("m2")] },
    ];

    expect(findMessageItemIndex(items, "unknown")).toBe(-1);
  });
});

describe("resolveVirtuosoEdgeState", () => {
  // Codex review #3668343109: the "scroll to start" control stayed hidden
  // while the first prompt was still mounted but only partially clipped,
  // because the old logic only compared item indices — which don't move
  // until the row fully leaves Virtuoso's rendered range. The mounted branch
  // must read live geometry instead of trusting the index comparison.
  it("uses live geometry when the row is mounted, ignoring the index comparison", () => {
    const container = elementWithRect(0, 500);
    const partiallyClippedRow = elementWithRect(-10, 40);
    const geometryCheck = vi.fn(() => true);
    const fromRangePosition = vi.fn(() => false);

    // Index comparison alone would say "not past" (itemIndex === renderedStart),
    // but the mounted row's geometry says it's already clipped.
    const result = resolveVirtuosoEdgeState(
      partiallyClippedRow,
      container,
      5,
      { start: 5, end: 5 },
      { geometryCheck, fromRangePosition },
    );

    expect(result).toBe(true);
    expect(geometryCheck).toHaveBeenCalledWith(container, partiallyClippedRow);
    expect(fromRangePosition).not.toHaveBeenCalled();
  });

  it("falls back to the index/range comparison once the row is unmounted", () => {
    const container = elementWithRect(0, 500);
    const geometryCheck = vi.fn(() => false);
    const fromRangePosition = (position: RangePosition) => position !== "within";
    const resolvers = { geometryCheck, fromRangePosition };

    expect(resolveVirtuosoEdgeState(null, container, 3, { start: 5, end: 5 }, resolvers)).toBe(
      true,
    );
    expect(resolveVirtuosoEdgeState(null, container, 5, { start: 5, end: 5 }, resolvers)).toBe(
      false,
    );
    expect(resolveVirtuosoEdgeState(null, container, -1, { start: 5, end: 5 }, resolvers)).toBe(
      false,
    );
    expect(geometryCheck).not.toHaveBeenCalled();
  });

  it("treats a tracked row after the rendered range as out of view", () => {
    const container = elementWithRect(0, 500);
    const geometryCheck = vi.fn(() => false);
    const fromRangePosition = (position: RangePosition) => position !== "within";
    expect(
      resolveVirtuosoEdgeState(
        null,
        container,
        8,
        { start: 5, end: 7 },
        {
          geometryCheck,
          fromRangePosition,
        },
      ),
    ).toBe(true);
    expect(geometryCheck).not.toHaveBeenCalled();
  });
});

describe("resolveVirtuosoEdgeState tri-state mapping", () => {
  function edgeFromPosition(position: RangePosition): "above" | "below" | "visible" {
    if (position === "before") return "above";
    if (position === "after") return "below";
    return "visible";
  }

  it("maps unmounted range positions to caller-provided values, distinguishing before from after", () => {
    const container = elementWithRect(0, 500);
    const geometryCheck = vi.fn(() => "visible" as const);
    const resolvers = { geometryCheck, fromRangePosition: edgeFromPosition };

    // Before the rendered range: scrolled past it going down -> "above".
    expect(resolveVirtuosoEdgeState(null, container, 2, { start: 5, end: 7 }, resolvers)).toBe(
      "above",
    );
    // After the rendered range: not yet reached -> "below".
    expect(resolveVirtuosoEdgeState(null, container, 9, { start: 5, end: 7 }, resolvers)).toBe(
      "below",
    );
    // Within the rendered range but unmounted (e.g. unknown id): "visible".
    expect(resolveVirtuosoEdgeState(null, container, -1, { start: 5, end: 7 }, resolvers)).toBe(
      "visible",
    );
  });
});
