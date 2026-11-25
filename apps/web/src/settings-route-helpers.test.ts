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

  it("drops a conflicting current hash when the destination selects a tab", () => {
    window.history.replaceState({}, "", "/settings/system/storage?tab=host#setting-system-storage");

    expect(resolveSettingsRedirect("/settings/system/storage?tab=office-retention")).toBe(
      "/settings/system/storage?tab=office-retention",
    );
  });
});

it("preserves SSH bookmark context and fragment on relocation", () => {
  const query =
    "?tab=identities&id=key%2Fa&new=1&adopt_executor=e&adopt_digest=d&adopt_connection=c&return_connection=b#key";
  window.history.replaceState({}, "", `/settings/ssh-connections${query}`);
  expect(resolveSettingsRedirect("/settings/executors/ssh-connections")).toBe(
    `/settings/executors/ssh-connections${query}`,
  );
});
