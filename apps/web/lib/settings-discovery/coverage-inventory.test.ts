import { describe, expect, it } from "vitest";
import {
  SETTINGS_COVERAGE_INVENTORY,
  validateSettingsCoverageInventory,
  type SettingsCoverageEvidence,
} from "./coverage-inventory";

const REQUIRED_DOMAINS = [
  "agent_profile",
  "agent_profile_mcp",
  "user_settings",
  "workflow",
  "workflow_step",
  "workspace",
  "repository",
  "repository_set",
  "repository_script",
  "executor",
  "executor_profile",
  "environment",
  "task",
  "prompt",
  "utility_agent",
  "editor",
  "notification_provider",
  "issue_integrations",
  "code_host_integrations",
  "automation",
  "runtime_flag",
  "storage_maintenance",
] as const;

describe("settings coverage inventory", () => {
  it("records every required domain independently of the runtime catalog", () => {
    const evidence = SETTINGS_COVERAGE_INVENTORY.map((entry) => entry.domain);

    expect(evidence).toEqual(expect.arrayContaining([...REQUIRED_DOMAINS]));
    expect(new Set(evidence).size).toBeGreaterThanOrEqual(REQUIRED_DOMAINS.length);
    expect(SETTINGS_COVERAGE_INVENTORY.every((entry) => entry.sourcePaths.length > 0)).toBe(true);
  });

  it("rejects eligible fields that have no owner or remain pending", () => {
    const invalid: SettingsCoverageEvidence[] = [
      {
        id: "missing-owner",
        domain: "workspace",
        fieldPath: "name",
        sourcePaths: ["apps/web/components/settings/workspaces"],
        status: "supported",
        owner: "",
      },
      {
        id: "pending-field",
        domain: "workspace",
        fieldPath: "default_executor_id",
        sourcePaths: ["apps/backend/internal/task/service"],
        status: "pending",
        owner: "task-07-workspace-settings",
      },
    ];

    expect(validateSettingsCoverageInventory(invalid)).toEqual([
      "missing-owner: owner is required",
      "pending-field: eligible fields cannot remain pending",
    ]);
  });

  it("keeps explicit exceptions reasoned and recoverable", () => {
    const exceptions = SETTINGS_COVERAGE_INVENTORY.filter((entry) => entry.status === "exception");

    expect(exceptions.length).toBeGreaterThan(0);
    expect(
      exceptions.every((entry) => entry.exceptionCategory && entry.reason && entry.recovery),
    ).toBe(true);
  });
});
