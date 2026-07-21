import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";

import { DiffErrorBoundary } from "./diff-error-boundary";

function Boom({ message = "boom" }: { message?: string }): never {
  throw new Error(message);
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

beforeEach(() => {
  // React logs caught errors to console.error — silence so test output stays clean.
  vi.spyOn(console, "error").mockImplementation(() => undefined);
});

describe("DiffErrorBoundary", () => {
  it("renders children when no error is thrown", () => {
    render(
      <DiffErrorBoundary filePath="src/foo.ts">
        <div data-testid="child">child content</div>
      </DiffErrorBoundary>,
    );
    expect(screen.getByTestId("child")).toBeTruthy();
  });

  it("renders fallback when a child throws during render", () => {
    render(
      <DiffErrorBoundary filePath="src/foo.ts">
        <Boom />
      </DiffErrorBoundary>,
    );
    expect(screen.getByText(/Unable to render diff/i)).toBeTruthy();
  });

  it("resets fallback when filePath changes", () => {
    const { rerender } = render(
      <DiffErrorBoundary filePath="src/foo.ts">
        <Boom />
      </DiffErrorBoundary>,
    );
    expect(screen.getByText(/Unable to render diff/i)).toBeTruthy();

    rerender(
      <DiffErrorBoundary filePath="src/bar.ts">
        <div data-testid="child">recovered</div>
      </DiffErrorBoundary>,
    );
    expect(screen.getByTestId("child")).toBeTruthy();
  });
});
