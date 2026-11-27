import { describe, expect, it } from "vitest";
import type { AutomationMCPOptions } from "@/lib/types/automation";
import { summarizeAgentTools } from "./agent-tools-summary";

const OPTIONS = {
  packages: [
    { id: "core", label_key: "pkg.core" },
    { id: "configuration", label_key: "pkg.configuration" },
    { id: "rich_output", label_key: "pkg.rich" },
  ],
  definitions: [
    { id: "def-github", name: "github", enabled: true },
    { id: "def-sentry", name: "sentry", enabled: true },
  ],
  inherited: [],
  task_defaults: [],
  task_blocked: [],
} as unknown as AutomationMCPOptions;

const LABELS: Record<string, string> = {
  "pkg.core": "Core",
  "pkg.configuration": "Configuration",
  "pkg.rich": "Rich Output",
};

function translate(key: string, options?: Record<string, unknown>): string {
  if (key === "automations:toolsSummaryPluginTools") return `${options?.count} plugin tools`;
  return LABELS[key] ?? key;
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.5
describe("summarizeAgentTools", () => {
  it("names enabled packages in catalog order, then selected servers", () => {
    const summary = summarizeAgentTools({
      selection: {
        builtin_packages: ["rich_output", "core"] as never,
        definition_ids: ["def-sentry"],
      },
      options: OPTIONS,
      pluginTools: [],
      translate,
      language: "en",
    });
    expect(summary).toBe("Core, Rich Output, sentry");
  });

  it("counts selected plugin tools", () => {
    const summary = summarizeAgentTools({
      selection: { builtin_packages: ["core"] as never, definition_ids: [] },
      options: OPTIONS,
      pluginTools: [
        { plugin_id: "p", installation_id: "i", policy_version: "1", tool_names: ["a", "b"] },
      ],
      translate,
      language: "en",
    });
    expect(summary).toBe("Core, 2 plugin tools");
  });

  it("skips servers that are no longer offered", () => {
    const summary = summarizeAgentTools({
      selection: { builtin_packages: ["core"] as never, definition_ids: ["def-gone"] },
      options: OPTIONS,
      pluginTools: [],
      translate,
      language: "en",
    });
    expect(summary).toBe("Core");
  });
});
