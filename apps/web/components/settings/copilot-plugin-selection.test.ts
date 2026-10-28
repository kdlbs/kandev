import { describe, expect, it } from "vitest";
import { buildCopilotPluginSelectionRows } from "./copilot-plugin-selection";

const installed = {
  id: "installed:copilot-plugins/spark",
  name: "spark",
  marketplace: "copilot-plugins",
  version: "1.0.0",
  native_enabled: false,
  availability: "available" as const,
};
const builtin = {
  id: "builtin:/browser-use",
  name: "browser-use",
  version: "0.0.4",
  native_enabled: false,
  availability: "unavailable" as const,
  reason_code: "builtin_not_mountable",
};

describe("buildCopilotPluginSelectionRows", () => {
  it("keeps native-disabled installed plugins selectable", () => {
    const [row] = buildCopilotPluginSelectionRows({ plugins: [installed], status: "ready" }, []);
    expect(row.nativeEnabled).toBe(false);
    expect(row.available).toBe(true);
    expect(row.selected).toBe(false);
  });

  it("shows missing saved identities after a successful inventory", () => {
    const rows = buildCopilotPluginSelectionRows({ plugins: [installed], status: "ready" }, [
      "installed:missing/plugin",
    ]);
    const missing = rows.find((row) => row.id === "installed:missing/plugin");
    expect(missing?.missing).toBe(true);
    expect(missing?.availabilityUnchecked).toBe(false);
    expect(missing?.selected).toBe(true);
  });

  it("shows saved identities as unchecked after a failed refresh", () => {
    const rows = buildCopilotPluginSelectionRows({ plugins: [], status: "unavailable" }, [
      "installed:copilot-plugins/spark",
    ]);
    expect(rows[0].availabilityUnchecked).toBe(true);
    expect(rows[0].missing).toBe(false);
    expect(rows[0].selected).toBe(true);
  });

  it("marks retained rows unchecked when the refresh failed", () => {
    const rows = buildCopilotPluginSelectionRows({ plugins: [installed], status: "unavailable" }, [
      "installed:copilot-plugins/spark",
    ]);
    expect(rows[0].available).toBe(false);
    expect(rows[0].availabilityUnchecked).toBe(true);
    expect(rows[0].reasonCode).toBeNull();
    expect(rows[0].selected).toBe(true);
  });

  it("keeps unavailable inventory rows visible with their reason", () => {
    const rows = buildCopilotPluginSelectionRows({ plugins: [builtin], status: "ready" }, []);
    expect(rows[0].available).toBe(false);
    expect(rows[0].reasonCode).toBe("builtin_not_mountable");
  });
});
