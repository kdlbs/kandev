import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTheme } from "@/components/theme/app-theme";
import { ThemeProvider } from "./theme-provider";

class FakeWindowControlsOverlay extends EventTarget implements KandevWindowControlsOverlay {
  visible = true;

  getTitlebarAreaRect(): DOMRectReadOnly {
    return new DOMRect(72, 0, 1448, 40);
  }

  publish(visible: boolean): void {
    this.visible = visible;
    this.dispatchEvent(new Event("geometrychange"));
  }
}

let overlay: FakeWindowControlsOverlay;
const OVERLAY_THEME_COLOR_SELECTOR = 'meta[data-kandev-window-controls-theme-color="true"]';

function ThemeSwitch() {
  const { setTheme } = useTheme();
  return (
    <button type="button" onClick={() => setTheme("light")}>
      Switch theme
    </button>
  );
}

describe("ThemeProvider desktop PWA title bar", () => {
  beforeEach(() => {
    document.head.innerHTML = `
      <meta name="theme-color" content="#f8fafc" media="(prefers-color-scheme: light)" />
      <meta name="theme-color" content="#09090b" media="(prefers-color-scheme: dark)" />
    `;
    window.localStorage.setItem("theme", "dark");
    vi.stubGlobal("matchMedia", () => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }));
    overlay = new FakeWindowControlsOverlay();
    Object.defineProperty(navigator, "windowControlsOverlay", {
      configurable: true,
      value: overlay,
    });
  });

  afterEach(() => {
    cleanup();
    document.head.innerHTML = "";
    window.localStorage.clear();
    Reflect.deleteProperty(navigator, "windowControlsOverlay");
    vi.unstubAllGlobals();
  });

  it("uses the resolved app theme for the desktop PWA window-control surface", async () => {
    render(
      <ThemeProvider>
        <ThemeSwitch />
      </ThemeProvider>,
    );

    await waitFor(() => {
      const themeColors = document.head.querySelectorAll('meta[name="theme-color"]');
      expect(themeColors).toHaveLength(3);
      const activeThemeColor = document.head.querySelector(OVERLAY_THEME_COLOR_SELECTOR);
      expect(activeThemeColor).not.toBeNull();
      expect(activeThemeColor?.getAttribute("content")).toBe("#181818");
      expect(activeThemeColor?.hasAttribute("media")).toBe(false);
      expect(themeColors[1].getAttribute("media")).toBe("(prefers-color-scheme: light)");
      expect(themeColors[2].getAttribute("media")).toBe("(prefers-color-scheme: dark)");
    });

    fireEvent.click(screen.getByRole("button", { name: "Switch theme" }));

    await waitFor(() => {
      expect(
        document.head.querySelector(OVERLAY_THEME_COLOR_SELECTOR)?.getAttribute("content"),
      ).toBe("#ffffff");
    });
  });

  it("keeps media fallbacks when the overlay is hidden and restores them after it hides", async () => {
    overlay.visible = false;
    render(
      <ThemeProvider>
        <ThemeSwitch />
      </ThemeProvider>,
    );

    await waitFor(() => {
      expect(document.head.querySelectorAll('meta[name="theme-color"]')).toHaveLength(2);
      expect(document.head.querySelector(OVERLAY_THEME_COLOR_SELECTOR)).toBeNull();
    });

    act(() => overlay.publish(true));
    await waitFor(() => {
      expect(
        document.head.querySelector(OVERLAY_THEME_COLOR_SELECTOR)?.getAttribute("content"),
      ).toBe("#181818");
    });

    act(() => overlay.publish(false));
    await waitFor(() => {
      expect(document.head.querySelectorAll('meta[name="theme-color"]')).toHaveLength(2);
      expect(document.head.querySelector(OVERLAY_THEME_COLOR_SELECTOR)).toBeNull();
    });
  });
});
