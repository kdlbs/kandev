import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CanvasPage } from "./canvas-page";

const responsive = { isMobile: false };

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

describe("CanvasPage", () => {
  afterEach(() => {
    cleanup();
    responsive.isMobile = false;
  });

  it("renders a full-height host and forwards runtime and load lifecycle props", async () => {
    const onLoad = vi.fn();
    render(
      <CanvasPage
        runtimeUrl="/api/v1/plugins/web-apps/runtime/capability/"
        title="Task board"
        onLoad={onLoad}
      />,
    );

    const page = screen.getByTestId("canvas-page");
    expect(page.className).toContain("h-full");
    expect(page.className).toContain("min-h-0");
    expect(page.className).toContain("min-w-0");
    expect(page.className).toContain("overflow-hidden");

    const frame = screen.getByTitle("Task board");
    expect(frame.getAttribute("src")).toContain("/api/v1/plugins/web-apps/runtime/");
    expect(screen.getByRole("status").closest("iframe")).toBeNull();

    fireEvent.load(frame);
    expect(onLoad).toHaveBeenCalledOnce();
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  });

  it("keeps the unavailable host state outside the iframe", () => {
    render(<CanvasPage title="Canvas" />);

    expect(screen.queryByTitle("Canvas")).toBeNull();
    expect(screen.getByRole("alert").closest("iframe")).toBeNull();
  });

  it("delegates phone safe-area behavior to WebAppFrame", () => {
    responsive.isMobile = true;
    render(<CanvasPage runtimeUrl="/runtime/phone/" title="Canvas" />);

    const frame = screen.getByTestId("web-app-frame");
    expect(frame.dataset.mobile).toBe("true");
    expect(frame.className).toContain("pb-[env(safe-area-inset-bottom)]");
  });
});
