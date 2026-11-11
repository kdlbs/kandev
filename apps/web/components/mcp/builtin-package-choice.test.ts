import { expect, it } from "vitest";
import type { BuiltinMCPManagementView } from "@/lib/types/builtin-mcp";
import { builtinPackageChoice } from "./builtin-package-choice";

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
