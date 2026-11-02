import { describe, expect, it } from "vitest";
import { isSettingsRoute, settingsReturnHref } from "./settings-return-navigation";

describe("Settings return destination", () => {
  // @covers AC-UI-SETTINGS-RETURN-001.1, AC-UI-SETTINGS-RETURN-001.2
  it("retains the complete origin across settings redirects and nested pages", () => {
    const origin = "/t/task-a?sessionId=session-b&layout=review&tag=a&tag=b#changes";
    const captured = settingsReturnHref(null, origin, "/settings");
    expect(captured).toBe(origin);
    expect(settingsReturnHref(captured, "/settings", "/settings/agents?tab=profiles")).toBe(origin);
  });

  // @covers AC-UI-SETTINGS-RETURN-001.4, AC-UI-SETTINGS-RETURN-001.5
  it("clears a departed visit and captures the next page independently", () => {
    expect(settingsReturnHref("/t/task-a", "/settings/agents", "/stats")).toBeNull();
    expect(settingsReturnHref(null, "/stats?range=week", "/settings/")).toBe("/stats?range=week");
    expect(settingsReturnHref(null, "/settings", "/settings/agents")).toBeNull();
  });

  it.each(["https://example.com/task", "//example.com/task", "task-relative"])(
    "does not accept an external or relative origin: %s",
    (origin) => {
      expect(settingsReturnHref(null, origin, "/settings")).toBeNull();
    },
  );

  it.each([
    ["/settings", true],
    ["/settings/agents", true],
    ["/settings-other", false],
    ["/t/task-a", false],
    [null, false],
  ])("recognizes Settings route boundaries: %s", (pathname, expected) => {
    expect(isSettingsRoute(pathname)).toBe(expected);
  });
});
