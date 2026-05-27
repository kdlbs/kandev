import { describe, it, expect } from "vitest";
import { stripAnsi } from "./ansi";

describe("stripAnsi", () => {
  it("returns input unchanged when there are no escape sequences", () => {
    expect(stripAnsi("hello world")).toBe("hello world");
  });

  it("strips SGR colour codes", () => {
    const input = "\x1b[36mInstalling backend dependencies...\x1b[0m";
    expect(stripAnsi(input)).toBe("Installing backend dependencies...");
  });

  it("strips bold + colour wrapped output across multiple lines", () => {
    const input =
      "\x1b[36mInstalling deps...\x1b[0m\nDownloading...\n\x1b[32m\x1b[1m✓ All dependencies installed!\x1b[0m";
    expect(stripAnsi(input)).toBe(
      "Installing deps...\nDownloading...\n✓ All dependencies installed!",
    );
  });

  it("handles empty string", () => {
    expect(stripAnsi("")).toBe("");
  });
});
