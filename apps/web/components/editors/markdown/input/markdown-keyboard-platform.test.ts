import { expect, it, vi } from "vitest";
import { getMarkdownKeyboardPlatform } from "./markdown-keyboard-platform";

it.each([
  ["Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", "macos"],
  ["Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)", "macos"],
  ["Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "windows"],
  ["Mozilla/5.0 (X11; Linux x86_64)", "linux"],
  ["", "linux"],
])("matches the engine keyboard platform for %j", (userAgent, expected) => {
  expect(getMarkdownKeyboardPlatform(userAgent)).toBe(expected);
});

it("uses the engine user-agent contract when native platform and browser UA differ", () => {
  const platform = vi.spyOn(navigator, "platform", "get").mockReturnValue("Linux x86_64");
  const userAgent = vi
    .spyOn(navigator, "userAgent", "get")
    .mockReturnValue("Mozilla/5.0 (Macintosh; Intel Mac OS X) AppleWebKit Safari/605.1.15");
  try {
    expect(getMarkdownKeyboardPlatform()).toBe("macos");
  } finally {
    platform.mockRestore();
    userAgent.mockRestore();
  }
});
