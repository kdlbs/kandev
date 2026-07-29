import { describe, expect, it } from "vitest";
import { resolveLspStatusPlacement } from "./lsp-status-placement";

describe("resolveLspStatusPlacement", () => {
  it("uses the toolbar by default", () => {
    expect(
      resolveLspStatusPlacement({
        preferredLocation: undefined,
        appStatusBarEnabled: true,
        hasFinePointer: true,
        isPhone: false,
      }),
    ).toBe("toolbar");
  });

  it("uses the status bar when preferred and supported", () => {
    expect(
      resolveLspStatusPlacement({
        preferredLocation: "status_bar",
        appStatusBarEnabled: true,
        hasFinePointer: true,
        isPhone: false,
      }),
    ).toBe("status_bar");
  });

  it.each([
    ["disabled application status bar", false, true],
    ["coarse pointer", true, false],
  ])("falls back to the toolbar for %s", (_name, appStatusBarEnabled, hasFinePointer) => {
    expect(
      resolveLspStatusPlacement({
        preferredLocation: "status_bar",
        appStatusBarEnabled,
        hasFinePointer,
        isPhone: false,
      }),
    ).toBe("toolbar");
  });

  it("excludes the phone viewer", () => {
    expect(
      resolveLspStatusPlacement({
        preferredLocation: "status_bar",
        appStatusBarEnabled: true,
        hasFinePointer: true,
        isPhone: true,
      }),
    ).toBe("hidden");
  });
});
