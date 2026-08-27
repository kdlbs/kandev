import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTheme } from "@/components/theme/app-theme";
import { ThemeProvider } from "./theme-provider";

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
    Object.defineProperty(navigator, "windowControlsOverlay", {
      configurable: true,
      value: {
        visible: true,
        getTitlebarAreaRect: () => new DOMRect(72, 0, 1448, 40),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      },
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
      expect(themeColors).toHaveLength(1);
      expect(themeColors[0].getAttribute("content")).toBe("#181818");
      expect(themeColors[0].hasAttribute("media")).toBe(false);
    });

    fireEvent.click(screen.getByRole("button", { name: "Switch theme" }));

    await waitFor(() => {
      expect(document.head.querySelector('meta[name="theme-color"]')?.getAttribute("content")).toBe(
        "#ffffff",
      );
    });
  });
});
