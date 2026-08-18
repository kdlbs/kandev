import { describe, expect, it, vi, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { RefObject } from "react";
import { usePreviewConsoleForwarder } from "./use-preview-console-forwarder";

const INSPECTOR_SOURCE = "kandev-inspector";

// The console forwarder must only accept messages from the preview iframe
// origin and contentWindow; wrong-origin or wrong-source messages are ignored.
describe("usePreviewConsoleForwarder", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  const PREVIEW_ORIGIN = "http://api.example";

  function makeIframeRef(
    contentWindow: unknown,
    src = `${window.location.origin}/port-proxy/session/3000/`,
  ) {
    return {
      current: {
        contentWindow,
        src,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      },
    } as unknown as RefObject<HTMLIFrameElement | null>;
  }

  function dispatchMessage(data: unknown, origin: string, source: unknown) {
    window.dispatchEvent(
      new MessageEvent("message", { data, origin, source: source as MessageEventSource }),
    );
  }

  function consoleMessage(level: string, args: unknown[]) {
    return { source: INSPECTOR_SOURCE, type: "console", payload: { level, args } };
  }

  it("forwards same-origin console messages from the preview iframe", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = { postMessage: vi.fn() };
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow)));

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), window.location.origin, contentWindow);
    });

    expect(warn).toHaveBeenCalledWith("[preview]", "boom");
  });

  it("binds the console bridge and forwards messages from a separate preview origin", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = { postMessage: vi.fn() };
    const iframeRef = makeIframeRef(contentWindow, `${PREVIEW_ORIGIN}/port-proxy/session/3000/`);
    renderHook(() => usePreviewConsoleForwarder(iframeRef));

    act(() => {
      dispatchMessage(
        { source: INSPECTOR_SOURCE, type: "console-ready", payload: {} },
        PREVIEW_ORIGIN,
        contentWindow,
      );
    });

    expect(contentWindow.postMessage).toHaveBeenCalledWith(
      { source: INSPECTOR_SOURCE, type: "console-bind", payload: {} },
      PREVIEW_ORIGIN,
    );

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), PREVIEW_ORIGIN, contentWindow);
    });

    expect(warn).toHaveBeenCalledWith("[preview]", "boom");
  });

  it("ignores messages from a different origin", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = { postMessage: vi.fn() };
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow)));

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), "https://evil.example", contentWindow);
    });

    expect(warn).not.toHaveBeenCalled();
  });

  it("ignores messages from a different source window", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = { postMessage: vi.fn() };
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow)));

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), window.location.origin, { other: true });
    });

    expect(warn).not.toHaveBeenCalled();
  });

  it("ignores non-console inspector messages", () => {
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    const contentWindow = { postMessage: vi.fn() };
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow)));

    act(() => {
      dispatchMessage(
        { source: INSPECTOR_SOURCE, type: "other" },
        window.location.origin,
        contentWindow,
      );
    });

    expect(log).not.toHaveBeenCalled();
  });

  it("ignores messages when the iframe source is not an HTTP origin", () => {
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    const contentWindow = { postMessage: vi.fn() };
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow, "about:blank")));

    act(() => {
      dispatchMessage(consoleMessage("log", ["x"]), PREVIEW_ORIGIN, contentWindow);
    });

    expect(log).not.toHaveBeenCalled();
  });
});
