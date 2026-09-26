/**
 * AC-UI-PIPELINE-ROW-002.6 and AC-UI-PIPELINE-ROW-004.4: this contract's
 * shared-source extraction (see pipeline-kanban-shared-source.test.tsx) must
 * not change what the Kanban card itself renders. For each fixture in the
 * design's Observability matrix, the task menu's entry identity/order/
 * enablement/destructive styling and the inline indicator set/order must
 * match this contract's base commit. The one sanctioned difference is the
 * widened title-preview disclosure (AC-004.4), asserted separately below as
 * an explicit negative/positive pair.
 */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import { pluginRegistry } from "@/lib/plugins/registry";
import { KanbanCard, type KanbanCardProps, type Task } from "@/components/kanban-card";
import { resolveTaskRepositoryChips } from "@/components/kanban-card-repositories";
import type { WorkflowStep } from "@/components/kanban-column";
import type { Repository } from "@/lib/types/http";

afterEach(() => {
  cleanup();
});

const STEPS: WorkflowStep[] = [
  { id: "step-1", title: "Triage", color: "#888" },
  { id: "step-2", title: "In Progress", color: "#888" },
  { id: "step-3", title: "Done", color: "#888" },
];

const EXTERNAL_LINK_AVAILABILITY = { jira: false, linear: false, sentry: false };

function baseTask(overrides: Partial<Task> = {}): Task {
  return {
    id: "task-1",
    title: "A task",
    workflowStepId: "step-2",
    ...overrides,
  };
}

function renderCard(
  task: Task,
  repositories: Repository[] = [],
  props: Partial<KanbanCardProps> = {},
) {
  return render(
    <ToastProvider>
      <StateProvider>
        <TooltipProvider delayDuration={0}>
          <KanbanCard
            task={task}
            workspaceId="workspace-1"
            externalLinkAvailability={EXTERNAL_LINK_AVAILABILITY}
            repositoryChips={resolveTaskRepositoryChips(task, repositories)}
            steps={STEPS}
            onDelete={() => undefined}
            {...props}
          />
        </TooltipProvider>
      </StateProvider>
    </ToastProvider>,
  );
}

async function openMenuAndSnapshot() {
  const trigger = screen.getByRole("button", { name: t("kanban:moreOptions") });
  fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
  fireEvent.click(trigger);
  const items = await waitFor(() => {
    const found = screen.getAllByRole("menuitem");
    expect(found.length).toBeGreaterThan(0);
    return found;
  });
  return items.map((item) => ({
    text: item.textContent,
    disabled: item.getAttribute("data-disabled") !== null,
    // Token match, not substring: every DropdownMenuItem's base class
    // literally contains "text-destructive" as part of the always-present
    // `data-[variant=destructive]:text-destructive` selector, so a substring
    // check matches every item regardless of its actual destructive state.
    destructive: item.className.split(/\s+/).includes("text-destructive"),
  }));
}

/**
 * None of this contract's fixtures seed a workflow move target or wire
 * onEdit/onArchive, so the top-level menu is identical across every fixture:
 * Edit and Archive stay disabled (no handler supplied), Link stays enabled
 * (KanbanCard wires its own link handlers internally), and Delete is the
 * only destructive, enabled entry. Asserting this exact array per fixture is
 * what proves the menu's identity/order/enablement/destructive styling is
 * unaffected by badge-driving task state (blocked, queued, review, repos,
 * plugins) — not just that a menu renders. Group order (priority, edit,
 * relationships, move, plugins, remove) comes from `buildGroupedMenuEntries`.
 */
function expectedMenuEntries() {
  return [
    { text: t("kanban:priority"), disabled: false, destructive: false },
    { text: t("common:edit"), disabled: true, destructive: false },
    { text: t("kanban:link"), disabled: false, destructive: false },
    { text: t("kanban:archive"), disabled: true, destructive: false },
    { text: t("kanban:delete"), disabled: false, destructive: true },
  ];
}

describe("KanbanCard regression — AC-UI-PIPELINE-ROW-002.6 fixture matrix", () => {
  it("bare task: renders the base menu and no status badges or repo chips", async () => {
    const { container } = renderCard(baseTask());

    const entries = await openMenuAndSnapshot();
    expect(entries).toEqual(expectedMenuEntries());

    expect(container.querySelector('[data-testid="kanban-card-blocked-badge"]')).toBeNull();
    expect(container.querySelector('[data-testid="task-repo-chip"]')).toBeNull();
  });

  it("two repositories with an overflow chip: shows 2 visible chips, a +1 overflow indicator, and the base menu", async () => {
    const repositories: Repository[] = [
      { id: "repo-1", workspace_id: "ws-1", name: "alpha" } as Repository,
      { id: "repo-2", workspace_id: "ws-1", name: "beta" } as Repository,
      { id: "repo-3", workspace_id: "ws-1", name: "gamma" } as Repository,
    ];
    const task = baseTask({
      repositoryId: "repo-1",
      repositories: [
        { id: "link-1", repository_id: "repo-1", position: 0 },
        { id: "link-2", repository_id: "repo-2", position: 1 },
        { id: "link-3", repository_id: "repo-3", position: 2 },
      ],
    });

    renderCard(task, repositories);

    expect(screen.getAllByTestId("task-repo-chip")).toHaveLength(2);
    expect(screen.getByText("+1")).not.toBeNull();
    expect(await openMenuAndSnapshot()).toEqual(expectedMenuEntries());
  });

  it("blocked by a failed predecessor: shows the failed-styled blocked badge and the base menu", async () => {
    const task = baseTask({
      blocked: true,
      blockedReason: "failed",
      dependsOn: [{ id: "dep-1", title: "Predecessor task" }],
    });

    renderCard(task);

    const badge = screen.getByTestId("kanban-card-blocked-badge");
    expect(badge.textContent).toBe(t("kanban:blockedFailed"));
    expect(badge.className).toContain("border-red-500/40");
    expect(await openMenuAndSnapshot()).toEqual(expectedMenuEntries());
  });

  it("queued for a step: shows the queued-for-step badge with the step title and the base menu", async () => {
    const task = baseTask({ queuedForStepId: "step-3", queuedForStepTitle: "Done" });

    renderCard(task);

    expect(screen.getByText(t("kanban:queuedForStep", { step: "Done" }))).not.toBeNull();
    expect(await openMenuAndSnapshot()).toEqual(expectedMenuEntries());
  });

  it("review state and more than one session: shows both the changes-requested badge and the session count, and the base menu", async () => {
    const task = baseTask({ reviewStatus: "changes_requested", sessionCount: 2 });

    renderCard(task);

    expect(screen.getByText(t("kanban:changesRequested"))).not.toBeNull();
    expect(screen.getByText(t("kanban:sessionCount", { count: 2 }))).not.toBeNull();
    expect(await openMenuAndSnapshot()).toEqual(expectedMenuEntries());
  });

  it("both plugin slots contributing: renders the registered indicator and tags components, and the base menu", async () => {
    const PLUGIN_ID = "kandev-plugin-regression-fixture";
    function Indicator() {
      return <span data-testid="regression-indicator">indicator</span>;
    }
    function Tags() {
      return <span data-testid="regression-tags">tags</span>;
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerComponent("task-card-indicators", Indicator);
    pluginRegistry.forPlugin(PLUGIN_ID).registerComponent("task-card-tags", Tags);

    try {
      renderCard(baseTask());

      expect(screen.getByTestId("regression-indicator")).not.toBeNull();
      expect(screen.getByTestId("regression-tags")).not.toBeNull();
      expect(await openMenuAndSnapshot()).toEqual(expectedMenuEntries());
    } finally {
      pluginRegistry.unregisterPlugin(PLUGIN_ID);
    }
  });
});

describe("KanbanCard — title-preview disclosure widening (AC-UI-PIPELINE-ROW-004.4)", () => {
  it("negative case: a short-titled task with no description, parent, or subtasks still opens no popover", () => {
    renderCard(baseTask({ title: "Short" }));
    expect(screen.queryByTestId("task-title-preview-trigger")).toBeNull();
  });

  it("positive case: a task with a description mounts the preview trigger on the Kanban card", () => {
    renderCard(baseTask({ description: "Some description content" }));
    expect(screen.getByTestId("task-title-preview-trigger")).not.toBeNull();
  });
});

describe("KanbanCard — title preview in multi-select mode", () => {
  // @covers AC-TASKS-RICH-TASK-TITLE-PREVIEWS-001.2
  it.each([
    ["unselected", false],
    ["selected", true],
  ] as const)("does not mount a title preview for a %s card", (_label, isSelected) => {
    renderCard(baseTask({ description: "Some description content" }), [], {
      isMultiSelectMode: true,
      isSelected,
    });

    expect(screen.queryByTestId("task-title-preview-trigger")).toBeNull();
  });
});

// @covers AC-TASKS-MOBILE-KANBAN-SCROLL-001.1, AC-TASKS-MOBILE-KANBAN-SCROLL-001.4
it.each(["Enter", " "])(
  "phone cards open with %j without exposing or invoking drag pickup",
  (key) => {
    const onCardKeyDown = vi.fn();
    const onClick = vi.fn();
    renderCard(baseTask(), [], { presentation: "mobile", onCardKeyDown, onClick });
    const card = screen.getByTestId("task-card-task-1");
    expect(card.getAttribute("aria-roledescription")).toBeNull();
    expect(card.getAttribute("aria-describedby")).toBeNull();
    expect(card.tabIndex).toBe(0);
    fireEvent.keyDown(card, { key });
    fireEvent.keyDown(card, { key, repeat: true });
    fireEvent.keyDown(screen.getByRole("button", { name: t("kanban:moreOptions") }), { key });
    expect(onCardKeyDown).not.toHaveBeenCalled();
    expect(onClick).toHaveBeenCalledTimes(1);
  },
);

it("desktop cards retain keyboard reorder and drag instructions", () => {
  const onCardKeyDown = vi.fn();
  renderCard(baseTask(), [], { presentation: "desktop", onCardKeyDown });
  const card = screen.getByTestId("task-card-task-1");
  expect(card.getAttribute("aria-roledescription")).toBe("draggable");
  fireEvent.keyDown(card, { key: "Enter" });
  expect(onCardKeyDown).toHaveBeenCalledTimes(1);
});
