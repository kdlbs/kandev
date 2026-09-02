/**
 * AC-UI-PIPELINE-ROW-002.4 and AC-UI-PIPELINE-ROW-003.8: the Kanban card and
 * the pipeline row must render the same task menu and the same inline status
 * strip from the same shared source, not two implementations that happen to
 * agree. See "Shared-source tests" in
 * docs/specs/ui/system-design/pipeline-task-row-parity.md#observability —
 * without this pair, a duplicated-JSX fork would pass every other test.
 */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import { KanbanCard, type Task } from "@/components/kanban-card";
import { resolveTaskRepositoryChips } from "@/components/kanban-card-repositories";
import type { WorkflowStep } from "@/components/kanban-column";
import type { Repository } from "@/lib/types/http";
import type { TaskPR } from "@/lib/types/github";
import { Graph2TaskPipeline } from "./graph2-task-pipeline";

afterEach(() => {
  cleanup();
});

const WORKSPACE_ID = "workspace-1";

const STEPS: WorkflowStep[] = [
  { id: "step-1", title: "Triage", color: "#888" },
  { id: "step-2", title: "In Progress", color: "#888" },
  { id: "step-3", title: "Done", color: "#888" },
];

const REPOSITORIES: Repository[] = [
  {
    id: "repo-1",
    workspace_id: WORKSPACE_ID,
    name: "alpha",
    provider_owner: "acme",
    provider_name: "alpha",
  } as Repository,
  {
    id: "repo-2",
    workspace_id: WORKSPACE_ID,
    name: "beta",
    provider_owner: "acme",
    provider_name: "beta",
  } as Repository,
];

/**
 * Carries a blocked state, a queued step, a review state, two repositories,
 * more than one session, a subagent count and a linked PR, per the
 * Observability fixture.
 */
const TASK: Task = {
  id: "task-1",
  title: "Shared source fixture task",
  workflowStepId: "step-2",
  state: "IN_PROGRESS",
  repositoryId: "repo-1",
  repositories: [
    { id: "link-1", repository_id: "repo-1", position: 0 },
    { id: "link-2", repository_id: "repo-2", position: 1 },
  ],
  sessionCount: 2,
  activeSubagentCount: 3,
  reviewStatus: "changes_requested",
  queuedForStepId: "step-3",
  queuedForStepTitle: "Done",
  blocked: true,
  blockedReason: "pending",
  dependsOn: [{ id: "dep-1", title: "Predecessor task" }],
};

const TASK_PR: TaskPR = {
  id: "pr-1",
  workspace_id: WORKSPACE_ID,
  task_id: TASK.id,
  owner: "acme",
  repo: "alpha",
  pr_number: 7,
  pr_url: "",
  pr_title: "Fix",
  head_branch: "feat",
  base_branch: "main",
  author_login: "alice",
  state: "open",
  review_state: "",
  checks_state: "",
  mergeable_state: "unknown",
  review_count: 0,
  pending_review_count: 0,
  comment_count: 0,
  unresolved_review_threads: 0,
  checks_total: 0,
  checks_passing: 0,
  additions: 0,
  deletions: 0,
  created_at: "",
  merged_at: null,
  closed_at: null,
  last_synced_at: null,
  updated_at: "",
};

const EXTERNAL_LINK_AVAILABILITY = { jira: false, linear: false, sentry: false };

const SHARED_INITIAL_STATE = {
  workspaces: { activeId: WORKSPACE_ID, items: [] },
  taskPRs: {
    byTaskId: { [TASK.id]: [TASK_PR] },
    workspaceId: WORKSPACE_ID,
    workspaceContextGeneration: 0,
    deletedAssociationIdsByTaskId: {},
  },
};

function renderKanbanCard() {
  return render(
    <ToastProvider>
      <StateProvider initialState={SHARED_INITIAL_STATE}>
        <TooltipProvider delayDuration={0}>
          <KanbanCard
            task={TASK}
            workspaceId={WORKSPACE_ID}
            externalLinkAvailability={EXTERNAL_LINK_AVAILABILITY}
            repositoryChips={resolveTaskRepositoryChips(TASK, REPOSITORIES)}
            steps={STEPS}
            onDelete={() => undefined}
          />
        </TooltipProvider>
      </StateProvider>
    </ToastProvider>,
  );
}

function renderPipelineRow() {
  return render(
    <ToastProvider>
      <StateProvider initialState={SHARED_INITIAL_STATE}>
        <TooltipProvider delayDuration={0}>
          <Graph2TaskPipeline
            task={TASK}
            steps={STEPS}
            moveTargetSteps={STEPS}
            workspaceId={WORKSPACE_ID}
            externalLinkAvailability={EXTERNAL_LINK_AVAILABILITY}
            repositories={REPOSITORIES}
            onMoveTask={() => undefined}
            onPreviewTask={() => undefined}
            onOpenTask={() => undefined}
            onDeleteTask={() => undefined}
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

describe("Shared-source parity — task menu (AC-UI-PIPELINE-ROW-002.4)", () => {
  it("renders identical menu entry identities, order, and disabled/destructive state", async () => {
    renderKanbanCard();
    const cardEntries = await openMenuAndSnapshot();
    cleanup();

    renderPipelineRow();
    const rowEntries = await openMenuAndSnapshot();

    expect(rowEntries).toEqual(cardEntries);
    expect(rowEntries.length).toBeGreaterThan(0);
  });
});

/**
 * The indicator identities the AC-UI-PIPELINE-ROW-003.8 order names, each
 * with a selector stable on both surfaces. Order here is the order asserted
 * below, not the AC's left-to-right order — the two surfaces group these
 * differently around the title and step run, so what must match is that
 * *this* subsequence renders in the *same relative order* on both, not that
 * either surface's absolute layout matches the other.
 */
const INDICATOR_SELECTORS: Array<{ id: string; selector: string }> = [
  { id: "pr", selector: `[data-testid="pr-task-icon-${TASK.id}"]` },
  { id: "blocked", selector: '[data-testid="kanban-card-blocked-badge"]' },
  { id: "queued", selector: `text:${t("kanban:queuedForStep", { step: "Done" })}` },
  { id: "session-count", selector: `text:${t("kanban:sessionCount", { count: 2 })}` },
  { id: "review-state", selector: `text:${t("kanban:changesRequested")}` },
];

/** Resolves each selector against `container` and returns the ids present, ordered by DOM position. */
function orderedIndicatorIds(container: HTMLElement): string[] {
  const found = INDICATOR_SELECTORS.flatMap(({ id, selector }) => {
    const element = selector.startsWith("text:")
      ? Array.from(container.querySelectorAll("*")).find(
          (el) => el.textContent === selector.slice("text:".length) && el.children.length === 0,
        )
      : container.querySelector(selector);
    return element ? [{ id, element }] : [];
  });
  return found
    .sort((a, b) => {
      const position = a.element.compareDocumentPosition(b.element);
      // eslint-disable-next-line no-bitwise
      if (position & Node.DOCUMENT_POSITION_FOLLOWING) return -1;
      // eslint-disable-next-line no-bitwise
      if (position & Node.DOCUMENT_POSITION_PRECEDING) return 1;
      return 0;
    })
    .map(({ id }) => id);
}

describe("Shared-source parity — inline status strip (AC-UI-PIPELINE-ROW-003.8)", () => {
  it("renders the same indicator set, in the same relative order, on both surfaces", async () => {
    const { container: cardContainer } = renderKanbanCard();
    await waitFor(() =>
      expect(cardContainer.querySelector(`[data-testid="pr-task-icon-${TASK.id}"]`)).not.toBeNull(),
    );
    const cardOrder = orderedIndicatorIds(cardContainer);
    // Every indicator in the fixture must actually be present on the card —
    // otherwise a fixture that silently fails to seed one would make this
    // comparison vacuously pass.
    expect(cardOrder).toEqual(["pr", "blocked", "queued", "session-count", "review-state"]);
    cleanup();

    const { container: rowContainer } = renderPipelineRow();
    await waitFor(() =>
      expect(rowContainer.querySelector(`[data-testid="pr-task-icon-${TASK.id}"]`)).not.toBeNull(),
    );
    const rowOrder = orderedIndicatorIds(rowContainer);

    expect(rowOrder).toEqual(cardOrder);
    // Exactly one session-count element on the row, at the card's >1
    // threshold — the badges block is the row's only session count.
    expect(screen.getAllByText(t("kanban:sessionCount", { count: 2 }))).toHaveLength(1);
  });
});
