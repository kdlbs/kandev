import { afterEach, describe, expect, it } from "vitest";
import { resolveSettingsRedirect } from "./settings-route-helpers";

describe("resolveSettingsRedirect", () => {
  afterEach(() => {
    window.history.replaceState({}, "", "/settings/system/backups?filter=recent#old-target");
  });

  it("preserves unrelated query state and uses the destination target", () => {
    window.history.replaceState(
      {},
      "",
      "/settings/system/backups?filter=recent&tab=old#old-target",
    );

    expect(
      resolveSettingsRedirect("/settings/system/data-storage?tab=database#setting-system-backups"),
    ).toBe("/settings/system/data-storage?filter=recent&tab=database#setting-system-backups");
  });

  it("preserves the current hash when the destination has no fragment", () => {
    window.history.replaceState({}, "", "/settings/system/storage?tab=host#setting-system-storage");

    expect(resolveSettingsRedirect("/settings/system/storage?tab=office-retention")).toBe(
      "/settings/system/storage?tab=office-retention#setting-system-storage",
    );
  });
});
