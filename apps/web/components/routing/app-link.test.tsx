import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import Link from "./app-link";

afterEach(() => cleanup());

describe("AppLink", () => {
  it("navigates same-origin links through browser history", () => {
    window.history.replaceState({}, "", "/");
    render(<Link href="/tasks">Tasks</Link>);

    fireEvent.click(screen.getByText("Tasks"));

    expect(window.location.pathname).toBe("/tasks");
  });

  it("preserves caller click handlers", () => {
    const onClick = vi.fn();
    render(
      <Link href="/tasks" onClick={onClick}>
        Tasks
      </Link>,
    );

    fireEvent.click(screen.getByText("Tasks"));

    expect(onClick).toHaveBeenCalledOnce();
  });

  it("does not intercept modified clicks", () => {
    const pushState = vi.spyOn(window.history, "pushState");
    render(<Link href="/tasks">Tasks</Link>);

    fireEvent.click(screen.getByText("Tasks"), { metaKey: true });

    expect(pushState).not.toHaveBeenCalled();
  });
});
