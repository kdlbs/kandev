import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WebAppFrame } from "./web-app-frame";
import { WEB_APP_STARTUP_RESULT_TYPE, WEB_APP_STARTUP_VERSION } from "./web-app-startup";

const responsive = { isMobile: false };
const theme = { resolvedTheme: "light" as "light" | "dark" };

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => theme,
}));

function acknowledge(frame: HTMLElement, result: "ready" | "failed" = "ready") {
  const probe = (frame as HTMLIFrameElement).contentWindow;
  const message = (
    probe as unknown as { postMessage: ReturnType<typeof vi.fn> }
  ).postMessage.mock.calls.find(([value]) => value?.type === "kandev.web_app.startup_probe")?.[0];
  if (!message) throw new Error("startup probe was not sent");
  window.dispatchEvent(
    new MessageEvent("message", {
      data: {
        type: WEB_APP_STARTUP_RESULT_TYPE,
        version: WEB_APP_STARTUP_VERSION,
        nonce: message.nonce,
        result,
        ...(result === "failed" ? { code: "document_error" } : {}),
      },
      source: probe as unknown as Window,
    }),
  );
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  responsive.isMobile = false;
  theme.resolvedTheme = "light";
  document.documentElement.className = "";
  document.documentElement.style.cssText = "";
});

describe("WebAppFrame startup", () => {
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

  it("waits for the current frame startup acknowledgement before revealing it", async () => {
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
    expect(onLoad).not.toHaveBeenCalled();
    expect(postMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "kandev.web_app.appearance",
        version: 1,
        mode: "light",
      }),
      "*",
    );
    expect(screen.queryByRole("status")).not.toBeNull();
    acknowledge(frame);
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
    expect(onLoad).toHaveBeenCalledOnce();
  });
});

describe("WebAppFrame updates and failure handling", () => {
  it("sends live resolved-theme changes without replacing the iframe", async () => {
    const { rerender } = render(<WebAppFrame runtimeUrl="/runtime/one/" title="Canvas" />);
    const frame = screen.getByTitle("Canvas");
    const postMessage = vi.fn();
    Object.defineProperty(frame, "contentWindow", {
      configurable: true,
      value: { postMessage },
    });
    fireEvent.load(frame);
    acknowledge(frame);
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
    const initialCallCount = postMessage.mock.calls.length;

    theme.resolvedTheme = "dark";
    document.documentElement.classList.add("dark");
    rerender(<WebAppFrame runtimeUrl="/runtime/one/" title="Canvas" />);

    await waitFor(() => expect(postMessage.mock.calls.length).toBeGreaterThan(initialCallCount));
    expect(postMessage.mock.calls.at(-1)?.[0]).toMatchObject({ mode: "dark" });
    expect(screen.getByTitle("Canvas")).toBe(frame);
  });

  it("ignores wrong-frame results and becomes unavailable at the deadline", async () => {
    vi.useFakeTimers();
    const onError = vi.fn();
    render(<WebAppFrame runtimeUrl="/runtime/one/" title="Canvas" onError={onError} />);
    const frame = screen.getByTitle("Canvas");
    const postMessage = vi.fn();
    const siblingWindow = { postMessage: vi.fn() };
    Object.defineProperty(frame, "contentWindow", {
      configurable: true,
      value: { postMessage },
    });
    fireEvent.load(frame);
    const probe = postMessage.mock.calls.find(
      ([value]) => value?.type === "kandev.web_app.startup_probe",
    )?.[0];
    expect(probe).toBeDefined();
    window.dispatchEvent(
      new MessageEvent("message", {
        data: {
          type: WEB_APP_STARTUP_RESULT_TYPE,
          version: WEB_APP_STARTUP_VERSION,
          nonce: probe.nonce,
          result: "ready",
        },
        source: siblingWindow as unknown as Window,
      }),
    );
    expect(screen.getByTestId("web-app-frame").getAttribute("data-frame-state")).toBe("loading");
    act(() => vi.advanceTimersByTime(15_000));
    expect(onError).toHaveBeenCalledOnce();
    expect(screen.getByTestId("web-app-frame").getAttribute("data-frame-state")).toBe(
      "unavailable",
    );
    expect(screen.queryByTitle("Canvas")).toBeNull();
    vi.useRealTimers();
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
