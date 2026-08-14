import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TodoIndicatorContent } from "./todo-indicator";

afterEach(cleanup);

/**
 * The chat status-bar popover/hovercard and the pinned Todos panel share
 * `TodoIndicatorContent`. `fillHeight` (pinned panel) is the opt-in full-
 * height variant; the default must keep the popover's `max-h-48` cap and
 * none of the panel-fill flex classes, or the chat surfaces silently change
 * layout (review finding, round 5).
 */
describe("TodoIndicatorContent default (popover) path", () => {
  it("keeps the popover max-height cap and no panel-fill classes when fillHeight is not set", () => {
    render(
      <TodoIndicatorContent
        todos={[{ text: "Review code", status: "in_progress" }]}
        completed={0}
        progress={0}
      />,
    );

    const root = screen.getByTestId("todo-indicator-popover");
    expect(root.classList.contains("h-full")).toBe(false);

    const list = root.querySelector<HTMLElement>(".overflow-y-auto");
    expect(list).not.toBeNull();
    expect(list!.classList.contains("max-h-48")).toBe(true);
    expect(list!.classList.contains("flex-1")).toBe(false);
    expect(list!.classList.contains("min-h-0")).toBe(false);
  });

  it("applies the panel-fill classes only when fillHeight is set", () => {
    render(
      <TodoIndicatorContent
        todos={[{ text: "Review code", status: "in_progress" }]}
        completed={0}
        progress={0}
        fillHeight
      />,
    );

    const root = screen.getByTestId("todo-indicator-popover");
    expect(root.classList.contains("h-full")).toBe(true);

    const list = root.querySelector<HTMLElement>(".overflow-y-auto");
    expect(list).not.toBeNull();
    expect(list!.classList.contains("max-h-48")).toBe(false);
    expect(list!.classList.contains("flex-1")).toBe(true);
    expect(list!.classList.contains("min-h-0")).toBe(true);
  });
});
