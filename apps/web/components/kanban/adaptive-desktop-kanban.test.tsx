import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { AdaptiveDesktopKanban } from "./adaptive-desktop-kanban";

function renderBoard() {
  render(
    <AdaptiveDesktopKanban
      steps={[{ id: "backlog", title: "Backlog", color: "bg-blue-500" }]}
      renderColumn={() => (
        <div>
          <div data-testid="blank-column-space" />
          <button type="button" data-testid="card-action">
            <svg data-testid="card-action-icon" />
          </button>
          <div data-kanban-card="" data-testid="task-card-shell">
            <span data-testid="task-card-shell-child" />
          </div>
          <div data-testid="draggable-card" draggable />
        </div>
      )}
    />,
  );

  return screen.getByTestId("desktop-kanban-scroll-window");
}

function startPan(window: HTMLElement, clientX = 200) {
  fireEvent.mouseDown(screen.getByTestId("blank-column-space"), { button: 0, clientX });
  fireEvent.mouseMove(window, { buttons: 1, clientX: clientX - 20 });
}

afterEach(cleanup);
describe("AdaptiveDesktopKanban mouse panning", () => {
  it("@covers AC-1 AC-2 pans from the original pointer and scroll baseline", () => {
    const window = renderBoard();
    window.scrollLeft = 400;

    startPan(window);
    expect(window.scrollLeft).toBe(420);

    fireEvent.mouseMove(window, { buttons: 1, clientX: 120 });
    expect(window.scrollLeft).toBe(480);
  });

  it("@covers AC-1 AC-6 requires a primary drag beyond the threshold", () => {
    const window = renderBoard();
    window.scrollLeft = 400;

    fireEvent.mouseDown(screen.getByTestId("blank-column-space"), { button: 2, clientX: 200 });
    fireEvent.mouseMove(window, { buttons: 2, clientX: 100 });
    expect(window.scrollLeft).toBe(400);

    fireEvent.mouseDown(screen.getByTestId("blank-column-space"), { button: 0, clientX: 200 });
    fireEvent.mouseMove(window, { buttons: 1, clientX: 196 });
    expect(window.scrollLeft).toBe(400);
  });

  it("@covers AC-5 ignores interactive and draggable descendants", () => {
    const window = renderBoard();
    window.scrollLeft = 400;

    fireEvent.mouseDown(screen.getByTestId("card-action-icon"), { button: 0, clientX: 200 });
    fireEvent.mouseMove(window, { buttons: 1, clientX: 100 });
    expect(window.scrollLeft).toBe(400);

    fireEvent.mouseDown(screen.getByTestId("task-card-shell-child"), { button: 0, clientX: 200 });
    fireEvent.mouseMove(window, { buttons: 1, clientX: 100 });
    expect(window.scrollLeft).toBe(400);

    fireEvent.mouseDown(screen.getByTestId("draggable-card"), { button: 0, clientX: 200 });
    fireEvent.mouseMove(window, { buttons: 1, clientX: 100 });
    expect(window.scrollLeft).toBe(400);
  });

  it("@covers AC-3 AC-4 cancels on mouse-up and does not resume after leaving", () => {
    const window = renderBoard();
    window.scrollLeft = 400;

    startPan(window);
    expect(window.style.scrollSnapType).toBe("none");
    fireEvent.mouseUp(window);
    expect(window.style.scrollSnapType).toBe("");
    const releasedPosition = window.scrollLeft;
    fireEvent.mouseMove(window, { buttons: 0, clientX: 100 });
    expect(window.scrollLeft).toBe(releasedPosition);

    startPan(window);
    fireEvent.mouseLeave(window);
    const leftPosition = window.scrollLeft;
    fireEvent.mouseMove(window, { buttons: 1, clientX: 100 });
    expect(window.scrollLeft).toBe(leftPosition);
  });
});
