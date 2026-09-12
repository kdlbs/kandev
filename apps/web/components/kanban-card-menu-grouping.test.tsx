import { afterEach, describe, expect, it, vi } from "vitest";
import { buildKanbanCardMenuEntries, type KanbanCardMenuEntry } from "./kanban-card-menu-items";
import { pluginRegistry } from "@/lib/plugins/registry";

const PLUGIN_ID = "kandev-plugin-tags";
const ACTION_ID = "quick-tag";
const ACTION_LABEL = "Quick tag";
const WORKFLOW_ONE = "wf-1";
const WORKFLOW_TWO = "wf-2";

const PluginBitbucketIcon = () => null;

afterEach(() => {
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

function entryKeys(entries: KanbanCardMenuEntry[]) {
  return entries.map((entry) => entry.key);
}

function registerPrimaryAction(visible?: () => boolean) {
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: ACTION_ID,
    label: ACTION_LABEL,
    icon: PluginBitbucketIcon,
    group: "primary",
    visible,
    run: vi.fn(),
  });
}

function movementArgs() {
  return {
    currentWorkflowId: WORKFLOW_ONE,
    workflows: [
      { id: WORKFLOW_ONE, name: "Workflow 1" },
      { id: WORKFLOW_TWO, name: "Workflow 2" },
    ],
    stepsByWorkflowId: {
      [WORKFLOW_ONE]: [
        { id: "s1", title: "Step 1" },
        { id: "s2", title: "Step 2" },
      ],
      [WORKFLOW_TWO]: [{ id: "s3", title: "Step 3" }],
    },
  };
}

describe("buildKanbanCardMenuEntries — 'primary' group plugin actions", () => {
  it("renders a 'primary' group action after movement and before removal", () => {
    registerPrimaryAction();

    const entries = buildKanbanCardMenuEntries({
      ...movementArgs(),
      onSendToWorkflow: vi.fn(),
      onLinkPullRequest: vi.fn(),
    });

    const keys = entryKeys(entries);
    const sendToIndex = keys.indexOf("send-to-workflow");
    const primaryIndex = keys.indexOf(`plugin-primary-${PLUGIN_ID}-${ACTION_ID}`);
    const linkIndex = keys.indexOf("link");

    expect(sendToIndex).toBeGreaterThanOrEqual(0);
    expect(primaryIndex).toBeGreaterThanOrEqual(0);
    expect(linkIndex).toBeGreaterThanOrEqual(0);
    expect(linkIndex).toBeLessThan(sendToIndex);
    expect(sendToIndex).toBeLessThan(primaryIndex);

    const archiveIndex = keys.indexOf("archive");
    expect(primaryIndex).toBeLessThan(archiveIndex);

    const primaryEntry = entries[primaryIndex];
    expect(primaryEntry.kind).toBe("item");
    if (primaryEntry.kind === "item") {
      expect(primaryEntry.label).toBe(ACTION_LABEL);
      expect((primaryEntry.icon as { type?: unknown })?.type).toBe(PluginBitbucketIcon);
    }
  });

  it("keeps exactly one separator between each nonempty card group", () => {
    registerPrimaryAction();

    const entries = buildKanbanCardMenuEntries({
      ...movementArgs(),
      currentStepId: "s1",
      currentPriority: "high",
      onSelectPriority: vi.fn(),
      onEdit: vi.fn(),
      onLinkPullRequest: vi.fn(),
      parentTaskId: "parent-1",
      onDetach: vi.fn(),
      onMoveToStep: vi.fn(),
      onSendToWorkflow: vi.fn(),
      onArchive: vi.fn(),
      onDelete: vi.fn(),
    });

    expect(entryKeys(entries)).toEqual([
      "priority",
      "edit-separator",
      "edit",
      "relationships-separator",
      "link",
      "detach",
      "move-separator",
      "move-to",
      "send-to-workflow",
      "plugins-separator",
      `plugin-primary-${PLUGIN_ID}-${ACTION_ID}`,
      "remove-separator",
      "archive",
      "delete",
    ]);
  });

  it("does not add a 'primary' entry when visible(context) returns false", () => {
    registerPrimaryAction(() => false);

    const entries = buildKanbanCardMenuEntries({ workflows: [], stepsByWorkflowId: {} });

    expect(entryKeys(entries)).not.toContain(`plugin-primary-${PLUGIN_ID}-${ACTION_ID}`);
  });

  it("leaves the 'edit' group submenu unaffected by 'primary' group registrations", () => {
    registerPrimaryAction();

    const entries = buildKanbanCardMenuEntries({ workflows: [], stepsByWorkflowId: {} });
    const editMenu = entries.find((entry) => entry.key === "edit");

    expect(editMenu?.kind).toBe("item");
  });
});
