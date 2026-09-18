import type { TFunction } from "i18next";
import { describe, expect, it, vi } from "vitest";
import { t } from "@/lib/i18n";
import { buildSidebarTaskCommands, type TaskCommandContext } from "./task-command-items";
const PLUGIN_ACTION = "plugin-primary";
const child = { id: "child", label: "Choice", group: "Tasks", action: vi.fn() };
function context(overrides: Partial<TaskCommandContext> = {}): TaskCommandContext {
  return {
    task: {
      id: "task-a",
      title: "Task A",
      workflowId: "workflow-a",
      workflowStepId: "work",
      parentTaskId: "parent",
    },
    t: t as TFunction,
    isPinned: false,
    colors: [child],
    priorities: [child],
    links: [child],
    nesting: [child],
    steps: [child],
    workflows: [child],
    plugins: [{ ...child, id: PLUGIN_ACTION }],
    onPin: vi.fn(),
    onEdit: vi.fn(),
    onRename: vi.fn(),
    onDetach: vi.fn(),
    onDelete: vi.fn(),
    ...overrides,
  };
}
describe("sidebar task command parity", () => {
  it("offers every missing single-task sidebar action", () => {
    expect(buildSidebarTaskCommands(context()).map((command) => command.id)).toEqual([
      "task-pin",
      "task-color",
      "task-priority",
      "task-edit",
      "task-rename",
      "task-duplicate",
      "task-nest",
      "task-link",
      "task-detach",
      "task-move",
      "task-send-workflow",
      PLUGIN_ACTION,
      "task-delete",
    ]);
  });
  it("keeps Duplicate disabled and identifies the task", () => {
    const commands = buildSidebarTaskCommands(context());
    expect(commands.find((c) => c.id === "task-duplicate")?.disabled).toBe(true);
    expect(commands.every((c) => c.context === "Task A")).toBe(true);
  });
  it("preserves archived-task eligibility and conditional detach", () => {
    const ctx = context();
    ctx.task = { ...ctx.task, isArchived: true, parentTaskId: undefined };
    expect(buildSidebarTaskCommands(ctx).map((c) => c.id)).toEqual([
      "task-pin",
      "task-rename",
      "task-link",
      PLUGIN_ACTION,
      "task-delete",
    ]);
  });
  it("invokes existing task handlers and switches pin labels", () => {
    const ctx = context({ isPinned: true });
    const commands = buildSidebarTaskCommands(ctx);
    const pin = commands.find((c) => c.id === "task-pin");
    expect(pin?.label).toBe(t("task:unpin"));
    pin?.action?.();
    commands.find((c) => c.id === "task-delete")?.action?.();
    expect(ctx.onPin).toHaveBeenCalledTimes(1);
    expect(ctx.onDelete).toHaveBeenCalledTimes(1);
  });
  it("removes vanished plugin actions and unavailable choices on rebuild", () => {
    const ctx = context({ plugins: [], links: [], steps: [], workflows: [] });
    expect(buildSidebarTaskCommands(ctx).map((c) => c.id)).not.toContain(PLUGIN_ACTION);
    expect(buildSidebarTaskCommands(ctx).map((c) => c.id)).not.toContain("task-move");
  });
});
