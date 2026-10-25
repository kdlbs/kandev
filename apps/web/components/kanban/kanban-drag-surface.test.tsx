import { createRef } from "react";
import { act, cleanup, fireEvent, render } from "@testing-library/react";
import type { DragCancelEvent, DragStartEvent } from "@dnd-kit/core";
import { afterEach, describe, expect, it, vi } from "vitest";

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

function renderCard(label = "Open task") {
  const open = vi.fn();
  const started = vi.fn();
  const cancelled = vi.fn();
  const view = render(
    <KanbanDragSurface
      layoutContent={<button onClick={open}>{label}</button>}
      activeTask={null}
      boardRef={createRef<HTMLDivElement>()}
      onDragStart={started}
      onDragCancel={cancelled}
    />,
  );
  const callbacks = dndContextProps.mock.calls.at(-1)![0] as {
    onDragStart: (event: DragStartEvent) => void;
    onDragCancel: (event: DragCancelEvent) => void;
  };
  const event = { active: { id: "task" } } as DragStartEvent;
  const cancel = () => {
    act(() => callbacks.onDragStart(event));
    act(() => callbacks.onDragCancel(event as DragCancelEvent));
    expect(started).toHaveBeenCalledWith(event);
    expect(cancelled).toHaveBeenCalledWith(event);
  };
  return { card: view.getByRole("button", { name: label }), open, cancel, unmount: view.unmount };
}

describe("KanbanDragSurface", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

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

  it.each(["mouse", "touch"])("ignores a delayed %s click from a cancelled drag", (pointerType) => {
    vi.useFakeTimers();
    const { card, open, cancel } = renderCard();
    fireEvent.pointerDown(card, { pointerType, isPrimary: true });
    cancel();
    vi.advanceTimersByTime(1000);
    fireEvent.pointerUp(card, { pointerType, isPrimary: true });
    fireEvent.click(card, { detail: 1 });
    expect(open).not.toHaveBeenCalled();
  });

  it.each(["mouse", "touch"])(
    "allows the next ordinary %s gesture after drag cancellation",
    (pointerType) => {
      const { card, open, cancel } = renderCard();
      fireEvent.pointerDown(card, { pointerType, isPrimary: true });
      cancel();
      fireEvent.pointerUp(card, { pointerType, isPrimary: true });
      fireEvent.click(card, { detail: 1 });
      expect(open).not.toHaveBeenCalled();
      fireEvent.pointerDown(card, { pointerType, isPrimary: true });
      fireEvent.pointerUp(card, { pointerType, isPrimary: true });
      fireEvent.click(card, { detail: 1 });
      expect(open).toHaveBeenCalledTimes(1);
    },
  );

  it("allows keyboard activation after drag cancellation", () => {
    const { card, open, cancel } = renderCard();
    fireEvent.pointerDown(card, { pointerType: "mouse", isPrimary: true });
    cancel();
    fireEvent.click(card, { detail: 0 });
    expect(open).toHaveBeenCalledTimes(1);
  });

  it("does not open another swimlane's card on the cancelled gesture's release", () => {
    vi.useFakeTimers();
    const source = renderCard("Dragged task");
    const target = renderCard("Other task");
    fireEvent.pointerDown(source.card, { pointerType: "mouse", isPrimary: true });
    source.cancel();
    vi.advanceTimersByTime(1000);
    fireEvent.pointerUp(target.card, { pointerType: "mouse", isPrimary: true });
    fireEvent.click(target.card, { detail: 1 });
    expect(source.open).not.toHaveBeenCalled();
    expect(target.open).not.toHaveBeenCalled();
    fireEvent.pointerDown(target.card, { pointerType: "mouse", isPrimary: true });
    fireEvent.pointerUp(target.card, { pointerType: "mouse", isPrimary: true });
    fireEvent.click(target.card, { detail: 1 });
    expect(target.open).toHaveBeenCalledTimes(1);
  });

  it("allows a new pointer gesture when the cancelled drag emitted no click", () => {
    const { card, open, cancel } = renderCard();
    fireEvent.pointerDown(card, { pointerType: "touch", isPrimary: true });
    cancel();
    fireEvent.pointerUp(card, { pointerType: "touch", isPrimary: true });
    fireEvent.pointerDown(card, { pointerType: "touch", isPrimary: true });
    fireEvent.pointerUp(card, { pointerType: "touch", isPrimary: true });
    fireEvent.click(card, { detail: 1 });
    expect(open).toHaveBeenCalledTimes(1);
  });

  it("removes the pending gesture guard when its surface unmounts", () => {
    const source = renderCard("Dragged task");
    const target = renderCard("Other task");
    fireEvent.pointerDown(source.card, { pointerType: "mouse", isPrimary: true });
    source.cancel();
    source.unmount();
    fireEvent.click(target.card, { detail: 1 });
    expect(target.open).toHaveBeenCalledTimes(1);
  });

  it("keeps the original gesture guarded when a secondary touch starts", () => {
    const { card, open, cancel } = renderCard();
    fireEvent.pointerDown(card, { pointerType: "touch", isPrimary: true });
    cancel();
    fireEvent.pointerDown(card, { pointerType: "touch", isPrimary: false });
    fireEvent.pointerUp(card, { pointerType: "touch", isPrimary: true });
    fireEvent.click(card, { detail: 1 });
    expect(open).not.toHaveBeenCalled();
  });
});
