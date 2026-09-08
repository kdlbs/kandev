import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WebAppFrame } from "./web-app-frame";

const responsive = { isMobile: false };
const theme = { resolvedTheme: "light" as "light" | "dark" };

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => theme,
}));

describe("WebAppFrame", () => {
  afterEach(() => {
    cleanup();
    responsive.isMobile = false;
    theme.resolvedTheme = "light";
    document.documentElement.className = "";
    document.documentElement.style.cssText = "";
  });

  it("uses an opaque sandbox and does not send host capabilities to the iframe", () => {
    render(
      <WebAppFrame runtimeUrl="/api/v1/plugins/web-apps/runtime/capability/" title="Task board" />,
    );

    const frame = screen.getByTitle("Task board");
    expect(frame.getAttribute("sandbox")).toBe("allow-scripts allow-forms");
    expect(frame.getAttribute("allow-same-origin")).toBeNull();
    expect(frame.getAttribute("allow")).toBeNull();
    expect(frame.getAttribute("referrerpolicy")).toBe("no-referrer");
    expect(frame.getAttribute("src")).toContain("/api/v1/plugins/web-apps/runtime/");
  });

  it("sends appearance before revealing the loaded frame and reports load callbacks", async () => {
    const onLoad = vi.fn();
    render(<WebAppFrame runtimeUrl="/runtime/one/" title="Canvas" onLoad={onLoad} />);

    expect(screen.getByRole("status")).not.toBeNull();
    const frame = screen.getByTitle("Canvas");
    const postMessage = vi.fn();
    Object.defineProperty(frame, "contentWindow", {
      configurable: true,
      value: { postMessage },
    });
    fireEvent.load(frame);
    expect(onLoad).toHaveBeenCalledOnce();
    expect(postMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "kandev.web_app.appearance",
        version: 1,
        mode: "light",
      }),
      "*",
    );
    expect(screen.queryByRole("status")).not.toBeNull();
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  });

  it("sends live resolved-theme changes without replacing the iframe", async () => {
    const { rerender } = render(<WebAppFrame runtimeUrl="/runtime/one/" title="Canvas" />);
    const frame = screen.getByTitle("Canvas");
    const postMessage = vi.fn();
    Object.defineProperty(frame, "contentWindow", {
      configurable: true,
      value: { postMessage },
    });
    fireEvent.load(frame);
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
    const initialCallCount = postMessage.mock.calls.length;

    theme.resolvedTheme = "dark";
    document.documentElement.classList.add("dark");
    rerender(<WebAppFrame runtimeUrl="/runtime/one/" title="Canvas" />);

    await waitFor(() => expect(postMessage.mock.calls.length).toBeGreaterThan(initialCallCount));
    expect(postMessage.mock.calls.at(-1)?.[0]).toMatchObject({ mode: "dark" });
    expect(screen.getByTitle("Canvas")).toBe(frame);
  });

  it("uses the phone safe-area inset and renders no iframe without a capability", () => {
    responsive.isMobile = true;
    const { rerender } = render(<WebAppFrame title="Canvas" />);
    expect(screen.queryByTitle("Canvas")).toBeNull();
    expect(screen.getByTestId("web-app-frame").dataset.mobile).toBe("true");
    expect(screen.getByRole("alert")).not.toBeNull();

    rerender(<WebAppFrame runtimeUrl="/runtime/two/" title="Canvas" />);
    expect(screen.getByTitle("Canvas")).not.toBeNull();
    responsive.isMobile = false;
  });
});
