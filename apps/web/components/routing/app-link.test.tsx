import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import Link from "./app-link";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("AppLink", () => {
  it("navigates same-origin links through browser history", () => {
    window.history.replaceState({}, "", "/");
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    render(<Link href="/tasks">Tasks</Link>);

    fireEvent.click(screen.getByText("Tasks"));

    expect(window.location.pathname).toBe("/tasks");
    expect(scrollTo).toHaveBeenCalledWith({ top: 0, left: 0 });
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

  it("does not intercept hash-only links", () => {
    const pushState = vi.spyOn(window.history, "pushState");
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    render(<Link href="#details">Details</Link>);

    fireEvent.click(screen.getByText("Details"));

    expect(pushState).not.toHaveBeenCalled();
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it("forwards refs to the anchor element", () => {
    const ref = { current: null as HTMLAnchorElement | null };
    render(
      <Link ref={ref} href="/tasks">
        Tasks
      </Link>,
    );

    expect(ref.current).toBeInstanceOf(HTMLAnchorElement);
    expect(ref.current?.getAttribute("href")).toBe("/tasks");
  });
});
