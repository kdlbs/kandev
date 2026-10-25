import { describe, expect, it } from "vitest";
import {
  ManagedAutomationImportError,
  parseManagedAutomationImport,
} from "./managed-automation-import";

const managedDocument = `version: 1
type: kandev_automations
automations:
  - name: Morning brief
    enabled: true
    task_mode: managed_conversation
    managed_destination:
      plugin_id: kandev-plugin-coordinator
      instance_key: morning
    prompt: Summarize blocked work
    triggers:
      - type: scheduled
        enabled: true
        config:
          cron_expression: "0 9 * * 1-5"
`;

describe("parseManagedAutomationImport", () => {
  it("reads portable intent and leaves destination binding to the receiver", () => {
    const result = parseManagedAutomationImport(managedDocument);

    expect(result.entries).toEqual([
      {
        sourceIndex: 0,
        name: "Morning brief",
        description: "",
        enabled: true,
        sourcePluginId: "kandev-plugin-coordinator",
        sourceInstanceKey: "morning",
        prompt: "Summarize blocked work",
        trigger: {
          type: "scheduled",
          enabled: true,
          config: { cron_expression: "0 9 * * 1-5" },
        },
      },
    ]);
    expect(result.skippedCount).toBe(0);
  });

  it("skips other automation modes while selecting managed schedules", () => {
    const result = parseManagedAutomationImport(
      managedDocument.replace(
        "automations:\n",
        `automations:\n  - name: Existing task mode\n    task_mode: normal_task\n`,
      ),
    );

    expect(result.entries).toHaveLength(1);
    expect(result.skippedCount).toBe(1);
  });

  it("rejects managed destinations without a valid scheduled trigger", () => {
    expect(() =>
      parseManagedAutomationImport(managedDocument.replace("type: scheduled", "type: webhook")),
    ).toThrowError(ManagedAutomationImportError);
  });
});
