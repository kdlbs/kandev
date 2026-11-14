import { expect, it } from "vitest";
import type { BuiltinMCPManagementView } from "@/lib/types/builtin-mcp";
import { builtinPackageChoice, builtinPackageRestriction } from "./builtin-package-choice";

function view(): BuiltinMCPManagementView {
  return {
    policy: {
      workspace_id: "ws",
      version: 1,
      revision: 0,
      packages: { configuration: "available", diagnostics: "default" },
    },
    preferences: { workspace_id: "ws", task_id: "task", version: 1, revision: 0, packages: {} },
    catalog: ["configuration", "diagnostics"].map((id) => ({
      id: id as "configuration" | "diagnostics",
      runtime_name: `kandev-${id}`,
      label_key: id,
      description_key: id,
      default_policy: "available",
      surfaces: ["kanban-task"],
      required_surfaces: null,
      tools: [],
    })),
    can_edit: true,
    surface: "kanban-task",
  };
}
it("shows effective inherited choices without marking implicit inheritance dirty", () => {
  expect(
    builtinPackageChoice(view(), { configuration: "inherit" }, "tasks", "configuration"),
  ).toMatchObject({
    enabled: false,
    dirty: false,
    inherited: true,
    labelKey: "settings:builtinMcp.choices.off",
  });
  expect(builtinPackageChoice(view(), {}, "tasks", "diagnostics")).toMatchObject({
    enabled: true,
    dirty: false,
  });
});
it("marks only the changed task choice and keeps workspace blocks effective", () => {
  const value = view();
  const draft = { configuration: "on" as const };
  expect(builtinPackageChoice(value, draft, "tasks", "configuration")).toMatchObject({
    enabled: true,
    dirty: true,
  });
  expect(builtinPackageChoice(value, draft, "tasks", "diagnostics").dirty).toBe(false);
  value.policy.packages.configuration = "blocked";
  expect(builtinPackageChoice(value, draft, "tasks", "configuration").enabled).toBe(false);
});
it("compares workspace drafts with policy rather than task preferences", () => {
  const value = view();
  expect(
    builtinPackageChoice(value, value.policy.packages, "workspaces", "diagnostics"),
  ).toMatchObject({ dirty: false, labelKey: "settings:builtinMcp.choices.default" });
  expect(
    builtinPackageChoice(value, { diagnostics: "blocked" }, "workspaces", "diagnostics"),
  ).toMatchObject({ dirty: true, enabled: false });
});
it("supports initial task choices without persisted preferences", () => {
  const value = view();
  delete value.preferences;
  expect(
    builtinPackageChoice(value, { configuration: "on" }, "tasks", "configuration"),
  ).toMatchObject({ dirty: true, enabled: true });
});
it("keeps required Core enabled and rejects an ineligible optional context", () => {
  const value = view();
  value.surface = "office-task";
  expect(builtinPackageChoice(value, { core: "off" }, "tasks", "core")).toMatchObject({
    enabled: true,
    dirty: false,
    labelKey: "settings:builtinMcp.required",
  });
  expect(
    builtinPackageChoice(value, { configuration: "on" }, "tasks", "configuration").enabled,
  ).toBe(false);
});

// @covers AC-AGENTS-BUILTIN-MCP-005.2
it("restores inheritance without confusing an equal explicit value with inheritance", () => {
  const value = view();
  value.preferences!.packages.diagnostics = "on";
  expect(
    builtinPackageChoice(value, { diagnostics: "inherit" }, "tasks", "diagnostics"),
  ).toMatchObject({ enabled: true, inherited: true, dirty: true, inheritedEnabled: true });
  value.policy.packages.diagnostics = "available";
  expect(
    builtinPackageChoice(value, { diagnostics: "inherit" }, "tasks", "diagnostics"),
  ).toMatchObject({ enabled: false, inheritedEnabled: false });
  expect(builtinPackageChoice(value, { diagnostics: "on" }, "tasks", "diagnostics")).toMatchObject({
    enabled: true,
    inherited: false,
  });
});

// @covers AC-AGENTS-BUILTIN-MCP-005.3
it("uses the workspace draft when unblocking and changing default inclusion", () => {
  const value = view();
  value.policy.packages.diagnostics = "blocked";
  expect(
    builtinPackageChoice(value, { diagnostics: "default" }, "workspaces", "diagnostics"),
  ).toMatchObject({ enabled: true, dirty: true });
});

// @covers AC-AGENTS-BUILTIN-MCP-005.4
it("keeps a blocked task override identifiable and removable", () => {
  const value = view();
  value.policy.packages.diagnostics = "blocked";
  value.preferences!.packages.diagnostics = "on";
  expect(builtinPackageChoice(value, { diagnostics: "on" }, "tasks", "diagnostics")).toMatchObject({
    choice: "on",
    enabled: false,
    inherited: false,
    dirty: false,
    inheritedEnabled: false,
  });
  expect(
    builtinPackageChoice(value, { diagnostics: "inherit" }, "tasks", "diagnostics"),
  ).toMatchObject({ enabled: false, inherited: true, dirty: true });
});

it("reports required, unavailable, and unrestricted package surfaces", () => {
  const value = view();
  const configuration = value.catalog[0];
  expect(builtinPackageRestriction(value, { ...configuration, id: "core" })).toBe(
    "settings:builtinMcp.coreRequired",
  );
  expect(builtinPackageRestriction(null, configuration)).toBeNull();
  expect(builtinPackageRestriction(value, configuration)).toBeNull();
  value.surface = "configuration";
  configuration.surfaces.push("configuration");
  configuration.required_surfaces = ["configuration"];
  expect(builtinPackageRestriction(value, configuration)).toBe(
    "settings:builtinMcp.configurationFixed",
  );
  expect(builtinPackageChoice(value, {}, "tasks", "configuration").inheritedEnabled).toBe(true);
  value.surface = "office-task";
  expect(builtinPackageRestriction(value, configuration)).toBe("settings:builtinMcp.ineligible");
  expect(builtinPackageChoice(value, {}, "tasks", "configuration").inheritedEnabled).toBe(false);
  expect(builtinPackageChoice(value, {}, "tasks", "core").inheritedEnabled).toBe(true);
});
