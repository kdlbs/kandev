import { afterEach, describe, expect, it } from "vitest";
import { isMacTauriWebview, macTauriDragRegionProps } from "./window-chrome";

describe("macOS Tauri window chrome detection", () => {
  afterEach(() => {
    delete (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__;
  });

  it("recognizes a macOS Tauri WebView", () => {
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {};
    expect(isMacTauriWebview(window, "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0)")).toBe(true);
  });

  it("keeps the browser PWA overlay contract separate", () => {
    expect(isMacTauriWebview(window, "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0)")).toBe(false);
  });

  it("does not apply the macOS layout to another Tauri platform", () => {
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {};
    expect(isMacTauriWebview(window, "Mozilla/5.0 (X11; Linux x86_64)")).toBe(false);
  });

  it("adds a native drag region only to a macOS Tauri window", () => {
    const macUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0)";
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {};
    expect(macTauriDragRegionProps(window, macUserAgent)).toEqual({
      "data-tauri-drag-region": "deep",
    });
    expect(macTauriDragRegionProps(window, "Mozilla/5.0 (X11; Linux x86_64)")).toEqual({});
  });
});
