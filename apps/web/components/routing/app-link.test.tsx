import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  clearNavigationBlockerForTests,
  setNavigationBlocker,
} from "@/lib/routing/navigation-guard";
import { LOCATION_CHANGE_EVENT } from "@/lib/routing/navigation-event";
import Link from "./app-link";

afterEach(() => {
  cleanup();
  clearNavigationBlockerForTests();
  vi.restoreAllMocks();
});

// eslint-disable-next-line max-lines-per-function -- the suite keeps one AppLink contract together.
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

  it("calls onNavigated after the location event for a committed navigation", () => {
    const events: string[] = [];
    const onNavigated = vi.fn(() => events.push("callback"));
    window.addEventListener(LOCATION_CHANGE_EVENT, () => events.push("location"), { once: true });
    render(
      <Link href="/tasks" onNavigated={onNavigated}>
        Tasks
      </Link>,
    );

    fireEvent.click(screen.getByText("Tasks"));

    expect(events).toEqual(["location", "callback"]);
    expect(onNavigated).toHaveBeenCalledOnce();
  });

  it("does not intercept modified clicks", () => {
    const pushState = vi.spyOn(window.history, "pushState");
    const onNavigated = vi.fn();
    render(
      <Link href="/tasks" onNavigated={onNavigated}>
        Tasks
      </Link>,
    );

    fireEvent.click(screen.getByText("Tasks"), { metaKey: true });

    expect(pushState).not.toHaveBeenCalled();
    expect(onNavigated).not.toHaveBeenCalled();
  });

  it("does not call onNavigated when a caller prevents the click", () => {
    const onNavigated = vi.fn();
    render(
      <Link href="/tasks" onNavigated={onNavigated} onClick={(event) => event.preventDefault()}>
        Tasks
      </Link>,
    );

    fireEvent.click(screen.getByText("Tasks"));

    expect(onNavigated).not.toHaveBeenCalled();
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

  it("waits for a navigation blocker before changing routes", () => {
    window.history.replaceState({}, "", "/settings/general/appearance");
    let proceed: () => void = () => undefined;
    const onLocationChange = vi.fn();
    window.addEventListener("kandev:navigation", onLocationChange, { once: true });
    setNavigationBlocker((intent) => {
      proceed = intent.proceed;
    });
    render(<Link href="/tasks">Tasks</Link>);

    fireEvent.click(screen.getByText("Tasks"));

    expect(window.location.pathname).toBe("/settings/general/appearance");
    proceed();
    expect(window.location.pathname).toBe("/tasks");
    expect(onLocationChange).toHaveBeenCalledOnce();
  });

  it("runs onNavigated only when a navigation blocker proceeds", () => {
    window.history.replaceState({}, "", "/settings/general/appearance");
    let proceed: () => void = () => undefined;
    const onNavigated = vi.fn();
    setNavigationBlocker((intent) => {
      proceed = intent.proceed;
    });
    render(
      <Link href="/tasks" onNavigated={onNavigated}>
        Tasks
      </Link>,
    );

    fireEvent.click(screen.getByText("Tasks"));

    expect(onNavigated).not.toHaveBeenCalled();
    proceed();
    expect(onNavigated).toHaveBeenCalledOnce();
  });
});
