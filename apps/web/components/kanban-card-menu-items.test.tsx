import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  buildKanbanCardMenuEntries,
  KanbanCardDropdownMenuItems,
  type KanbanCardMenuEntry,
} from "./kanban-card-menu-items";

// Regression: React synthetic events bubble through the fiber tree from a Radix portal; without stopPropagation the parent Card's onClick fires instead of the confirm dialog.
describe("KanbanCardDropdownMenuItems — click propagation", () => {
  function renderWithParent(entries: KanbanCardMenuEntry[], parentOnClick: () => void) {
    return render(
      <div data-testid="parent-card" onClick={parentOnClick}>
        <DropdownMenu defaultOpen>
          <DropdownMenuTrigger>open</DropdownMenuTrigger>
          <DropdownMenuContent>
            <KanbanCardDropdownMenuItems entries={entries} />
          </DropdownMenuContent>
        </DropdownMenu>
      </div>,
    );
  }

  it("clicking a menu item does not call the parent card's onClick", () => {
    const onDelete = vi.fn();
    const parentOnClick = vi.fn();
    const entries: KanbanCardMenuEntry[] = [
      {
        kind: "item",
        key: "delete",
        label: "Delete",
        onSelect: onDelete,
      },
    ];

    renderWithParent(entries, parentOnClick);

    const deleteItem = screen.getByRole("menuitem", { name: /delete/i });
    fireEvent.click(deleteItem);

    expect(onDelete).toHaveBeenCalledTimes(1);
    expect(parentOnClick).not.toHaveBeenCalled();
  });

  it("clicking an archive menu item does not call the parent card's onClick", () => {
    const onArchive = vi.fn();
    const parentOnClick = vi.fn();
    const entries: KanbanCardMenuEntry[] = [
      {
        kind: "item",
        key: "archive",
        label: "Archive",
        onSelect: onArchive,
      },
    ];

    renderWithParent(entries, parentOnClick);

    fireEvent.click(screen.getByRole("menuitem", { name: /archive/i }));

    expect(onArchive).toHaveBeenCalledTimes(1);
    expect(parentOnClick).not.toHaveBeenCalled();
  });

  it("pointer-down on a menu item does not reach the parent (dnd-kit guard)", () => {
    const parentOnPointerDown = vi.fn();
    const entries: KanbanCardMenuEntry[] = [
      { kind: "item", key: "delete", label: "Delete", onSelect: vi.fn() },
    ];

    render(
      <div data-testid="parent-card" onPointerDown={parentOnPointerDown}>
        <DropdownMenu defaultOpen>
          <DropdownMenuTrigger>open</DropdownMenuTrigger>
          <DropdownMenuContent>
            <KanbanCardDropdownMenuItems entries={entries} />
          </DropdownMenuContent>
        </DropdownMenu>
      </div>,
    );

    fireEvent.pointerDown(screen.getByRole("menuitem", { name: /delete/i }));

    expect(parentOnPointerDown).not.toHaveBeenCalled();
  });
});

describe("buildKanbanCardMenuEntries — external issue links", () => {
  function itemLabels(entry: KanbanCardMenuEntry | undefined) {
    if (entry?.kind !== "submenu") return [];
    return entry.children.filter((child) => child.kind === "item").map((child) => child.label);
  }

  it("adds configured external issue providers to the Link submenu", () => {
    const entries = buildKanbanCardMenuEntries({
      workflows: [],
      stepsByWorkflowId: {},
      onLinkPullRequest: vi.fn(),
      onLinkIssue: vi.fn(),
      onLinkMergeRequest: vi.fn(),
      onLinkJiraTicket: vi.fn(),
      onLinkLinearIssue: vi.fn(),
      onLinkSentryIssue: vi.fn(),
    });

    const linkMenu = entries.find((entry) => entry.kind === "submenu" && entry.key === "link");
    expect(linkMenu?.kind).toBe("submenu");

    expect(itemLabels(linkMenu)).toEqual([
      "GitHub Pull Request",
      "GitHub Issue",
      "GitLab Merge Request",
      "Jira Ticket",
      "Linear Issue",
      "Sentry Issue",
    ]);
  });

  it("omits external issue providers that are not configured", () => {
    const entries = buildKanbanCardMenuEntries({
      workflows: [],
      stepsByWorkflowId: {},
      onLinkPullRequest: vi.fn(),
      onLinkIssue: vi.fn(),
      onLinkJiraTicket: vi.fn(),
    });

    const linkMenu = entries.find((entry) => entry.kind === "submenu" && entry.key === "link");
    expect(linkMenu?.kind).toBe("submenu");

    expect(itemLabels(linkMenu)).toEqual(["GitHub Pull Request", "GitHub Issue", "Jira Ticket"]);
  });
});

describe("buildKanbanCardMenuEntries — !onEdit does not disable plugin edit actions", () => {
  const PLUGIN_ID = "kandev-plugin-notes";

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
  });

  it("disables Edit task but leaves a visible plugin action enabled when onEdit is absent", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "enhance",
      label: "Enhance notes",
      group: "edit",
      run: vi.fn(),
    });

    const entries = buildKanbanCardMenuEntries({
      workflows: [],
      stepsByWorkflowId: {},
      // onEdit intentionally omitted — a card with no edit handler wired up.
    });

    const editMenu = entries.find((entry) => entry.key === "edit");
    expect(editMenu?.kind).toBe("submenu");
    if (editMenu?.kind !== "submenu") return;

    const editTask = editMenu.children.find(
      (child) => child.kind === "item" && child.key === "edit-task",
    );
    const pluginAction = editMenu.children.find(
      (child) => child.kind === "item" && child.key === `plugin-edit-${PLUGIN_ID}-enhance`,
    );

    expect(editTask?.kind === "item" && editTask.disabled).toBe(true);
    expect(pluginAction?.kind === "item" && pluginAction.disabled).toBeFalsy();
  });
});

describe("buildKanbanCardMenuEntries — 'primary' group plugin actions", () => {
  const PLUGIN_ID = "kandev-plugin-tags";

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
  });

  function entryKeys(entries: KanbanCardMenuEntry[]) {
    return entries.map((entry) => entry.key);
  }

  it("renders a 'primary' group action as a flat item between Send to workflow and Link", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "quick-tag",
      label: "Quick tag",
      group: "primary",
      run: vi.fn(),
    });

    const entries = buildKanbanCardMenuEntries({
      currentWorkflowId: "wf-1",
      workflows: [
        { id: "wf-1", name: "Workflow 1" },
        { id: "wf-2", name: "Workflow 2" },
      ],
      stepsByWorkflowId: {
        "wf-1": [
          { id: "s1", title: "Step 1" },
          { id: "s2", title: "Step 2" },
        ],
        "wf-2": [{ id: "s3", title: "Step 3" }],
      },
      onSendToWorkflow: vi.fn(),
      onLinkPullRequest: vi.fn(),
    });

    const keys = entryKeys(entries);
    const sendToIndex = keys.indexOf("send-to-workflow");
    const primaryIndex = keys.indexOf(`plugin-primary-${PLUGIN_ID}-quick-tag`);
    const linkIndex = keys.indexOf("link");

    expect(sendToIndex).toBeGreaterThanOrEqual(0);
    expect(primaryIndex).toBeGreaterThanOrEqual(0);
    expect(linkIndex).toBeGreaterThanOrEqual(0);
    expect(sendToIndex).toBeLessThan(primaryIndex);
    expect(primaryIndex).toBeLessThan(linkIndex);

    const primaryEntry = entries[primaryIndex];
    expect(primaryEntry.kind).toBe("item");
    if (primaryEntry.kind === "item") expect(primaryEntry.label).toBe("Quick tag");
  });

  it("does not add a 'primary' entry when visible(context) returns false", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "quick-tag",
      label: "Quick tag",
      group: "primary",
      visible: () => false,
      run: vi.fn(),
    });

    const entries = buildKanbanCardMenuEntries({ workflows: [], stepsByWorkflowId: {} });

    expect(entryKeys(entries)).not.toContain(`plugin-primary-${PLUGIN_ID}-quick-tag`);
  });

  it("leaves the 'edit' group submenu unaffected by 'primary' group registrations", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "quick-tag",
      label: "Quick tag",
      group: "primary",
      run: vi.fn(),
    });

    const entries = buildKanbanCardMenuEntries({ workflows: [], stepsByWorkflowId: {} });
    const editMenu = entries.find((entry) => entry.key === "edit");

    expect(editMenu?.kind).toBe("item");
  });
});

describe("buildKanbanCardMenuEntries — detach", () => {
  const baseArgs = {
    workflows: [],
    stepsByWorkflowId: {},
  };

  it("offers detach for a child task and invokes the action", () => {
    const onDetach = vi.fn();
    const entries = buildKanbanCardMenuEntries({
      ...baseArgs,
      parentTaskId: "parent-1",
      onDetach,
    });
    const detach = entries.find((entry) => entry.kind === "item" && entry.key === "detach");

    expect(detach?.kind).toBe("item");
    if (detach?.kind === "item") detach.onSelect?.();
    expect(onDetach).toHaveBeenCalledOnce();
  });

  it("omits detach for a root task", () => {
    const entries = buildKanbanCardMenuEntries({
      ...baseArgs,
      onDetach: vi.fn(),
    });

    expect(entries.some((entry) => entry.key === "detach")).toBe(false);
  });
});
