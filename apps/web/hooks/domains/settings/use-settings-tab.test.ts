import { describe, expect, it } from "vitest";

import { resolveSettingsTab } from "./use-settings-tab";

describe("resolveSettingsTab", () => {
  const tabs = ["database", "logs"] as const;

  it("uses the default when the query is missing or invalid", () => {
    expect(resolveSettingsTab(null, tabs, "database")).toBe("database");
    expect(resolveSettingsTab("unknown", tabs, "database")).toBe("database");
  });

  it("keeps a valid query selection", () => {
    expect(resolveSettingsTab("logs", tabs, "database")).toBe("logs");
  });
});
