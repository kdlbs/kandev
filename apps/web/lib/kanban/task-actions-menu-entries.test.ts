import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { buildTaskActionsMenuEntries } from "./task-actions-menu-entries";
import type { KanbanCardMenuEntry } from "@/components/kanban-card-menu-items";

const PLUGIN_ID = "test-task-actions-menu-plugin";
const DELETE_SEPARATOR = "delete-separator";

function itemKeys(entries: KanbanCardMenuEntry[]): string[] {
  return entries.map((entry) => entry.key);
}

const baseArgs = {
  currentWorkflowId: "workflow-1",
  currentStepId: "step-1",
  workflows: [
    { id: "workflow-1", name: "Workflow 1" },
    { id: "workflow-2", name: "Workflow 2" },
  ],
  stepsByWorkflowId: {
    "workflow-1": [
      { id: "step-1", title: "Step 1" },
      { id: "step-2", title: "Step 2" },
    ],
    "workflow-2": [{ id: "step-3", title: "Step 3" }],
  },
  onEdit: vi.fn(),
  onArchive: vi.fn(),
  onDelete: vi.fn(),
  onDetach: vi.fn(),
  parentTaskId: "parent-1",
  onMoveToStep: vi.fn(),
  onSendToWorkflow: vi.fn(),
};

describe("buildTaskActionsMenuEntries — normal tier", () => {
  afterEach(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));

  it("always uses the flat Edit item, even when a plugin edit-group action is registered", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "edit-action",
      group: "edit",
      label: "Plugin edit action",
      run: vi.fn(),
    });

    const entries = buildTaskActionsMenuEntries("normal", baseArgs);
    const edit = entries.find((entry) => entry.key === "edit");

    expect(edit?.kind).toBe("item");
  });

  it("presents the full card order for a resolved, unarchived subject", () => {
    const entries = buildTaskActionsMenuEntries("normal", baseArgs);
    expect(itemKeys(entries)).toEqual([
      "edit",
      "move-to",
      "send-to-workflow",
      "archive",
      "detach",
      DELETE_SEPARATOR,
      "delete",
    ]);
  });
});

describe("buildTaskActionsMenuEntries — archived tier", () => {
  afterEach(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));

  it("presents Delete alone with no leading separator when no plugin action is admitted", () => {
    const entries = buildTaskActionsMenuEntries("archived", baseArgs);
    expect(itemKeys(entries)).toEqual(["delete"]);
  });

  it("presents admitted plugin primary actions, a separator, then Delete", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "primary-action",
      group: "primary",
      label: "Plugin primary action",
      run: vi.fn(),
    });

    const entries = buildTaskActionsMenuEntries("archived", baseArgs);
    expect(itemKeys(entries)).toEqual([
      `plugin-primary-${PLUGIN_ID}-primary-action`,
      DELETE_SEPARATOR,
      "delete",
    ]);
  });

  it("omits Edit, Move to, Send to workflow, Link, Detach from parent, and Archive", () => {
    const entries = buildTaskActionsMenuEntries("archived", baseArgs);
    const keys = itemKeys(entries);
    for (const omitted of ["edit", "move-to", "send-to-workflow", "link", "archive", "detach"]) {
      expect(keys).not.toContain(omitted);
    }
  });
});

describe("buildTaskActionsMenuEntries — unresolved board row tier", () => {
  afterEach(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));

  it("presents only Archive and Delete when no plugin action is admitted", () => {
    const entries = buildTaskActionsMenuEntries("unresolved-row", baseArgs);
    expect(itemKeys(entries)).toEqual(["archive", DELETE_SEPARATOR, "delete"]);
  });

  it("orders admitted plugin primary actions before Archive", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "primary-action",
      group: "primary",
      label: "Plugin primary action",
      run: vi.fn(),
    });

    const entries = buildTaskActionsMenuEntries("unresolved-row", baseArgs);
    expect(itemKeys(entries)).toEqual([
      `plugin-primary-${PLUGIN_ID}-primary-action`,
      "archive",
      DELETE_SEPARATOR,
      "delete",
    ]);
  });

  it("omits Edit, Move to, Send to workflow, Link, and Detach from parent even with a parentTaskId", () => {
    const entries = buildTaskActionsMenuEntries("unresolved-row", baseArgs);
    const keys = itemKeys(entries);
    for (const omitted of ["edit", "move-to", "send-to-workflow", "link", "detach"]) {
      expect(keys).not.toContain(omitted);
    }
  });
});
