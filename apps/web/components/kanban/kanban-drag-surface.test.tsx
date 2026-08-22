import { createRef } from "react";
import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const dndContextProps = vi.hoisted(() => vi.fn());
const pointerWithinMock = vi.hoisted(() => vi.fn());

vi.mock("@dnd-kit/core", () => ({
  DndContext: ({ children, ...props }: { children: React.ReactNode }) => {
    dndContextProps(props);
    return <>{children}</>;
  },
  DragOverlay: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  pointerWithin: pointerWithinMock,
}));

import { KANBAN_AUTO_SCROLL_OPTIONS, KanbanDragSurface } from "./kanban-drag-surface";

describe("KanbanDragSurface", () => {
  it("leaves horizontal layout compensation to the board anchor", () => {
    render(
      <KanbanDragSurface
        layoutContent={<div />}
        activeTask={null}
        boardRef={createRef<HTMLDivElement>()}
      />,
    );

    expect(KANBAN_AUTO_SCROLL_OPTIONS).toEqual({
      layoutShiftCompensation: { x: false, y: true },
    });
    expect(dndContextProps).toHaveBeenCalledWith(
      expect.objectContaining({
        autoScroll: KANBAN_AUTO_SCROLL_OPTIONS,
        collisionDetection: pointerWithinMock,
      }),
    );
  });
});
