import { useRef } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useThreadSelectionRecovery } from "./use-thread-selection-recovery";

const tasks = ["a", "b", "c", "d", "e", "f", "g", "h"];

function Fixture({
  layout,
  ids = tasks,
  geometry,
  isMobile = false,
}: {
  layout: "columns" | "grid";
  ids?: string[];
  geometry?: { width: number; layout: "columns" | "grid" };
  isMobile?: boolean;
}) {
  const boardRef = useRef<HTMLDivElement>(null);
  const remember = useThreadSelectionRecovery(ids, boardRef, isMobile, layout);
  return (
    <div
      ref={(board) => {
        boardRef.current = board;
        if (board)
          board.getBoundingClientRect = () =>
            ({ left: 0, right: geometry?.width ?? 600, width: geometry?.width ?? 600 }) as DOMRect;
      }}
      data-testid="board"
      onPointerDownCapture={remember}
    >
      {ids.map((id, index) => (
        <section
          key={id}
          data-thread-column-id={id}
          data-testid={id}
          ref={(element) => {
            if (!element) return;
            element.getBoundingClientRect = () => {
              const left =
                ((geometry?.layout ?? layout) === "grid" ? Math.floor(index / 2) : index) * 372 -
                (boardRef.current?.scrollLeft ?? 0);
              return { left, right: left + 360 } as DOMRect;
            };
          }}
        />
      ))}
    </div>
  );
}

afterEach(cleanup);

describe("reader identity across grid reflow", () => {
  it("does not cancel an in-flight deep-link scroll with a zero-offset assignment", () => {
    const view = render(<Fixture layout="columns" ids={["a"]} />);
    const board = screen.getByTestId("board");
    const assignScroll = vi.fn();
    Object.defineProperty(board, "scrollLeft", { get: () => 0, set: assignScroll });

    view.rerender(<Fixture layout="grid" ids={["a"]} />);

    expect(assignScroll).not.toHaveBeenCalled();
  });
  // @covers AC-UI-THREADS-DECK-004.6
  it("ignores resize-generated scroll and live snapshots before responsive reflow commits", () => {
    const geometry = { width: 600, layout: "columns" as "columns" | "grid" };
    const view = render(<Fixture layout="columns" geometry={geometry} isMobile />);
    const board = screen.getByTestId("board");
    board.scrollLeft = 1860;
    fireEvent.pointerDown(screen.getByTestId("f"));
    geometry.width = 400;
    geometry.layout = "grid";
    board.scrollLeft = 1100;
    fireEvent.scroll(board);
    view.rerender(<Fixture layout="columns" geometry={geometry} ids={[...tasks]} isMobile />);
    view.rerender(<Fixture layout="grid" geometry={geometry} />);
    expect(board.scrollLeft).toBe(744);
  });
  // @covers AC-UI-THREADS-DECK-004.6
  it("keeps an interacted lower-row task at its prior horizontal offset", () => {
    const view = render(<Fixture layout="columns" />);
    const board = screen.getByTestId("board");
    board.scrollLeft = 1660;
    fireEvent.pointerDown(screen.getByTestId("f"));
    const offset = screen.getByTestId("f").getBoundingClientRect().left;
    view.rerender(<Fixture layout="grid" />);
    expect(screen.getByTestId("f").getBoundingClientRect().left).toBe(offset);
    expect(board.scrollLeft).toBe(544);
  });
});
