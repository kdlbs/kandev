import { describe, expect, it } from "vitest";
import { getHiddenTopbarActionIds } from "./topbar-action-overflow";

const QUICK_CHAT = "quick-chat";
const ATTENTION = "attention";
const TOOLS = "tools";

const items = [
  { id: QUICK_CHAT, priority: 20 },
  { id: ATTENTION, priority: 80 },
  { id: TOOLS, priority: 10 },
];

const widths = new Map([
  [QUICK_CHAT, 88],
  [ATTENTION, 160],
  [TOOLS, 120],
]);

describe("getHiddenTopbarActionIds", () => {
  it("keeps every action visible when there is enough width", () => {
    expect(
      getHiddenTopbarActionIds({
        items,
        availableWidth: 600,
        itemWidths: widths,
        gap: 8,
        overflowTriggerWidth: 40,
      }),
    ).toEqual([]);
  });

  it("hides low-priority tools before quick chat or contextual actions", () => {
    expect(
      getHiddenTopbarActionIds({
        items,
        availableWidth: 320,
        itemWidths: widths,
        gap: 8,
        overflowTriggerWidth: 40,
      }),
    ).toEqual([TOOLS]);
  });

  it("hides quick chat before contextual attention actions", () => {
    expect(
      getHiddenTopbarActionIds({
        items,
        availableWidth: 230,
        itemWidths: widths,
        gap: 8,
        overflowTriggerWidth: 40,
      }),
    ).toEqual([QUICK_CHAT, TOOLS]);
  });
});
