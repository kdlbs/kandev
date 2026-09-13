import { describe, expect, it } from "vitest";
import { resolveThreadLayout } from "./thread-layout";

describe("Threads layout composition", () => {
  // @covers AC-UI-THREADS-DECK-004.1, AC-UI-THREADS-DECK-004.2
  it.each([
    [2, 1],
    [3, 2],
    [5, 3],
    [30, 15],
  ])("places %i chats into %i two-row tracks", (taskCount, columns) => {
    expect(
      resolveThreadLayout({ layout: "grid", isMobile: false, taskCount, contentHeight: 612 }),
    ).toEqual({ layout: "grid", rows: 2, columns, heightFallback: false });
  });

  // @covers AC-UI-THREADS-DECK-004.7
  it.each([null, 0, 400, 611.9])(
    "falls back below the measured two-row floor: %s",
    (contentHeight) => {
      expect(
        resolveThreadLayout({ layout: "grid", isMobile: false, taskCount: 5, contentHeight }),
      ).toEqual({ layout: "columns", rows: 1, columns: 5, heightFallback: contentHeight !== null });
    },
  );

  // @covers AC-UI-THREADS-DECK-004.3, AC-UI-THREADS-DECK-004.4
  it("keeps a single chat full height and phones single-row regardless of saved Grid", () => {
    expect(
      resolveThreadLayout({ layout: "grid", isMobile: false, taskCount: 1, contentHeight: 350 }),
    ).toEqual({ layout: "grid", rows: 1, columns: 1, heightFallback: false });
    expect(
      resolveThreadLayout({ layout: "grid", isMobile: true, taskCount: 5, contentHeight: 900 }),
    ).toEqual({ layout: "columns", rows: 1, columns: 5, heightFallback: false });
    expect(
      resolveThreadLayout({
        layout: "columns",
        isMobile: false,
        taskCount: 0,
        contentHeight: null,
      }),
    ).toEqual({ layout: "columns", rows: 1, columns: 0, heightFallback: false });
  });
});
