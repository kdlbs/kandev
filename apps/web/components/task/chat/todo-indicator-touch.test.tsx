import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TodoIndicator } from "./todo-indicator";

const TRIGGER = "todo-indicator";
const CONTENT = "todo-indicator-popover";

const todos = [
  { text: "Write tests", status: "completed" as const },
  { text: "Implement feature", status: "in_progress" as const },
];

function viewport(width: number, fine: boolean) {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: width });
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query === "(pointer: fine)" && fine,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

function tap(trigger: HTMLElement) {
  fireEvent.pointerEnter(trigger, { pointerType: "touch" });
  fireEvent.pointerDown(trigger, { pointerType: "touch", button: 0 });
  fireEvent.touchStart(trigger);
  fireEvent.pointerUp(trigger, { pointerType: "touch", button: 0 });
  fireEvent.touchEnd(trigger);
  fireEvent.click(trigger);
}

function expectChecklist() {
  const details = screen.getByTestId(CONTENT);
  expect(within(details).getByText("Write tests")).toBeTruthy();
  expect(within(details).getByText("Implement feature")).toBeTruthy();
  expect(details.textContent).toContain("1/2");
  expect(within(details).getByText("Write tests").className).toContain("line-through");
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("TodoIndicator disclosure", () => {
  it("opens the checklist from a wide coarse-pointer tap", async () => {
    viewport(1280, false);
    render(<TodoIndicator todos={todos} />);
    tap(screen.getByTestId(TRIGGER));
    await screen.findByTestId(CONTENT);
    expectChecklist();
  });

  it("opens the checklist from a phone tap", async () => {
    viewport(390, false);
    render(<TodoIndicator todos={todos} />);
    tap(screen.getByTestId(TRIGGER));
    await screen.findByTestId(CONTENT);
    expectChecklist();
  });

  it("opens the checklist on fine desktop keyboard focus", async () => {
    viewport(1280, true);
    render(<TodoIndicator todos={todos} />);
    fireEvent.focus(screen.getByTestId(TRIGGER));
    await screen.findByTestId(CONTENT);
    expectChecklist();
  });

  it("does not render a chip without todos", () => {
    viewport(1280, false);
    render(<TodoIndicator todos={[]} />);
    expect(screen.queryByTestId(TRIGGER)).toBeNull();
  });
});
