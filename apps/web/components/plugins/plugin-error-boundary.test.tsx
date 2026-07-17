import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PluginErrorBoundary } from "./plugin-error-boundary";

function Boom(): never {
  throw new Error("boom");
}

beforeEach(() => {
  // React logs caught errors to console.error — silence so test output stays clean.
  vi.spyOn(console, "error").mockImplementation(() => undefined);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("PluginErrorBoundary", () => {
  it("renders children when nothing throws", () => {
    render(
      <PluginErrorBoundary context='slot "task-sidebar"'>
        <div data-testid="child">child content</div>
      </PluginErrorBoundary>,
    );

    expect(screen.getByTestId("child")).not.toBeNull();
  });

  it("renders nothing when a child throws and no fallback is given", () => {
    const { container } = render(
      <PluginErrorBoundary context='slot "task-sidebar"'>
        <Boom />
      </PluginErrorBoundary>,
    );

    expect(container.innerHTML).toBe("");
  });

  it("renders the given fallback when a child throws", () => {
    render(
      <PluginErrorBoundary
        context='route "/plugins/hello"'
        fallback={<div data-testid="fallback">This plugin page failed to load</div>}
      >
        <Boom />
      </PluginErrorBoundary>,
    );

    expect(screen.getByTestId("fallback")).not.toBeNull();
  });

  it("logs the failure with the provided context so a specific plugin/route/slot is identifiable", () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);

    render(
      <PluginErrorBoundary context='route "/plugins/hello"'>
        <Boom />
      </PluginErrorBoundary>,
    );

    expect(errorSpy).toHaveBeenCalledWith(
      expect.stringContaining('route "/plugins/hello"'),
      expect.any(Error),
      expect.anything(),
    );
  });
});
