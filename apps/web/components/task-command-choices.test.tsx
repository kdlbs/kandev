import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginCommandChoices, useTaskMoveChoices } from "./task-command-choices";

afterEach(cleanup);

it("offers one task-scoped workflow-change command for every other workflow", () => {
  const openMoveOptions = vi.fn();
  const moveImmediately = vi.fn();
  const onChangeWorkflow = vi.fn();
  const { result } = renderHook(() =>
    useTaskMoveChoices({
      task: { id: "task", title: "Task", workflowId: "current" },
      workflows: [
        { id: "current", name: "Current" },
        { id: "empty", name: "Empty" },
        { id: "missing", name: "Missing" },
        { id: "target", name: "Target" },
      ],
      stepsByWorkflowId: { empty: [], target: [{ id: "review", title: "Review" }] },
      openMoveOptions,
      moveImmediately,
      onChangeWorkflow,
    }),
  );
  expect(result.current.workflows).toHaveLength(1);
  expect(result.current.workflows[0]).toMatchObject({
    id: "task-change-workflow",
    label: "Change workflow...",
  });
  result.current.workflows[0].action?.();
  expect(onChangeWorkflow).toHaveBeenCalledOnce();
  expect(openMoveOptions).not.toHaveBeenCalled();
  expect(moveImmediately).not.toHaveBeenCalled();
});

/**
 * A plugin submenu carries no action of its own (its label is a trigger), so
 * the command palette has to reach its children -- dropping the entry would
 * silently remove a plugin's actions from the palette and the sidebar's task
 * commands (see task-commands.test.tsx's plugin parity assertions).
 */
describe("pluginCommandChoices", () => {
  const item = (label: string, key = label) => ({
    kind: "item" as const,
    key,
    label,
    onSelect: vi.fn(),
  });

  it("turns a flat plugin entry into one command", () => {
    const entry = { ...item("Add tag"), disabled: true, icon: "icon" };
    expect(pluginCommandChoices(entry, "Tasks")).toEqual([
      {
        id: "Add tag",
        label: "Add tag",
        group: "Tasks",
        action: entry.onSelect,
        disabled: true,
        icon: "icon",
      },
    ]);
  });

  it("flattens a plugin submenu into its item children, keeping their disabled flag", () => {
    const first = item("Blocked", "plugin-primary-tags-add-tag-more");
    const second = { ...item("Urgent"), disabled: true };
    const entry = {
      kind: "submenu" as const,
      key: "plugin-primary-tags-add-tag",
      label: "Add tag...",
      children: [first, second, { kind: "separator" as const, key: "sep" }],
    };

    expect(pluginCommandChoices(entry, "Tasks").map((command) => command.id)).toEqual([
      "plugin-primary-tags-add-tag-more",
      "Urgent",
    ]);
    const [more, urgent] = pluginCommandChoices(entry, "Tasks");
    more.action?.();
    expect(first.onSelect).toHaveBeenCalledTimes(1);
    expect(urgent.disabled).toBe(true);
    expect(more.context).toBe("Add tag...");
  });

  it("reuses the entry key as the command id, which the builder keeps unique", () => {
    // Uniqueness is the builder's job: a child's key encodes its id, so the
    // palette can use it directly as both the React key and cmdk's value.
    const child = item("Pick", "plugin-primary-p-quick#pick-x");
    const submenu = {
      kind: "submenu" as const,
      key: "plugin-primary-p-quick",
      label: "Quick",
      children: [child],
    };
    const flat = item("Quick pick", "plugin-primary-p-quick-pick");

    const ids = [
      ...pluginCommandChoices(submenu, "Tasks"),
      ...pluginCommandChoices(flat, "Tasks"),
    ].map((command) => command.id);

    expect(ids).toEqual(["plugin-primary-p-quick#pick-x", "plugin-primary-p-quick-pick"]);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("ignores a label the palette cannot render", () => {
    const entry = { ...item("x"), label: {} as never };
    expect(pluginCommandChoices(entry, "Tasks")).toEqual([]);
  });
});
