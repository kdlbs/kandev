import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
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
