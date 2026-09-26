import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeToggle } from "./theme-toggle";

const mocks = vi.hoisted(() => ({
  setTheme: vi.fn(),
  theme: "system" as "light" | "dark" | "system",
  resolvedTheme: "dark" as "light" | "dark",
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({
    theme: mocks.theme,
    resolvedTheme: mocks.resolvedTheme,
    setTheme: mocks.setTheme,
  }),
}));

afterEach(cleanup);

describe("ThemeToggle", () => {
  beforeEach(() => {
    mocks.setTheme.mockClear();
    mocks.theme = "system";
    mocks.resolvedTheme = "dark";
  });

  it("switches from the resolved dark theme to light when the saved theme is system", () => {
    render(<ThemeToggle />);

    fireEvent.click(screen.getByRole("button"));

    expect(mocks.setTheme).toHaveBeenCalledWith("light");
  });
});
