import { afterEach, describe, expect, it } from "vitest";
import {
  getFixedDarkTerminalTheme,
  getTerminalTheme,
  TERMINAL_MINIMUM_CONTRAST_RATIO,
} from "./terminal-theme";

const ANSI_COLOR_KEYS = [
  "black",
  "red",
  "green",
  "yellow",
  "blue",
  "magenta",
  "cyan",
  "white",
  "brightBlack",
  "brightRed",
  "brightGreen",
  "brightYellow",
  "brightBlue",
  "brightMagenta",
  "brightCyan",
  "brightWhite",
] as const;

function createThemeContainer(): HTMLDivElement {
  const container = document.createElement("div");
  container.style.setProperty("--background", "#ffffff");
  container.style.setProperty("--foreground", "#111111");
  container.style.setProperty("--muted", "#e5e7eb");
  document.body.append(container);
  return container;
}

function channel(value: number): number {
  const normalized = value / 255;
  return normalized <= 0.03928 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
}

function luminance(color: string): number {
  const hex = color.replace("#", "");
  const red = Number.parseInt(hex.slice(0, 2), 16);
  const green = Number.parseInt(hex.slice(2, 4), 16);
  const blue = Number.parseInt(hex.slice(4, 6), 16);
  return 0.2126 * channel(red) + 0.7152 * channel(green) + 0.0722 * channel(blue);
}

function contrastRatio(first: string, second: string): number {
  const firstLuminance = luminance(first);
  const secondLuminance = luminance(second);
  const lighter = Math.max(firstLuminance, secondLuminance);
  const darker = Math.min(firstLuminance, secondLuminance);
  return (lighter + 0.05) / (darker + 0.05);
}

afterEach(() => {
  document.body.replaceChildren();
});

describe("getTerminalTheme", () => {
  it("uses a readable ANSI palette in light mode", () => {
    const theme = getTerminalTheme(createThemeContainer(), "light");
    const background = theme.background ?? "";
    const unreadableColors = ANSI_COLOR_KEYS.filter((key) => {
      const color = theme[key];
      return typeof color === "string" && contrastRatio(color, background) < 4.5;
    });

    expect(unreadableColors).toEqual([]);
  });

  it("selects the resolved theme palette", () => {
    const container = createThemeContainer();
    const lightTheme = getTerminalTheme(container, "light");
    const darkTheme = getTerminalTheme(container, "dark");

    expect(lightTheme.red).toBe("#cd3131");
    expect(darkTheme.red).toBe("#f44747");
  });

  it("keeps light bright ANSI variants visually distinct", () => {
    const theme = getTerminalTheme(createThemeContainer(), "light");

    expect(theme.brightBlue).not.toBe(theme.blue);
    expect(theme.brightCyan).not.toBe(theme.cyan);
    expect(theme.brightYellow).not.toBe(theme.yellow);
  });

  it("keeps fixed-dark terminals independent of the application theme", () => {
    expect(getFixedDarkTerminalTheme()).toMatchObject({
      background: "#0b0b0c",
      foreground: "#d4d4d4",
      red: "#f44747",
    });
  });

  it("uses the WCAG AA xterm contrast contract", () => {
    expect(TERMINAL_MINIMUM_CONTRAST_RATIO).toBe(4.5);
  });
});
