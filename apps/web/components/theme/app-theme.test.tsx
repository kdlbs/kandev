import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppThemeProvider, useTheme } from "./app-theme";

function ThemeProbe() {
  const { theme, savedTheme, previewTheme, commitTheme, restoreTheme } = useTheme();
  return (
    <div>
      <span data-testid="current-theme">{theme}</span>
      <span data-testid="saved-theme">{savedTheme}</span>
      <button type="button" onClick={() => previewTheme("light")}>
        Preview light
      </button>
      <button type="button" onClick={() => commitTheme("light")}>
        Save light
      </button>
      <button type="button" onClick={restoreTheme}>
        Discard preview
      </button>
    </div>
  );
}

describe("AppThemeProvider settings preview", () => {
  beforeEach(() => {
    window.localStorage.setItem("theme", "dark");
    vi.stubGlobal("matchMedia", () => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }));
  });

  afterEach(() => {
    cleanup();
    window.localStorage.clear();
    vi.unstubAllGlobals();
  });

  it("previews without writing storage and restores the saved theme", () => {
    render(
      <AppThemeProvider>
        <ThemeProbe />
      </AppThemeProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Preview light" }));

    expect(screen.getByTestId("current-theme").textContent).toBe("light");
    expect(screen.getByTestId("saved-theme").textContent).toBe("dark");
    expect(window.localStorage.getItem("theme")).toBe("dark");

    fireEvent.click(screen.getByRole("button", { name: "Discard preview" }));
    expect(screen.getByTestId("current-theme").textContent).toBe("dark");
  });

  it("writes the durable theme only when committed", () => {
    render(
      <AppThemeProvider>
        <ThemeProbe />
      </AppThemeProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Save light" }));

    expect(screen.getByTestId("saved-theme").textContent).toBe("light");
    expect(window.localStorage.getItem("theme")).toBe("light");
  });
});
