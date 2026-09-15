import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import { reviewPanelId } from "@/lib/state/dockview-review-panel-id";
import { AddPanelMenuItems, type AddPanelMenuState } from "./dockview-add-panel-items";
import { pluginRegistry } from "@/lib/plugins/registry";
import { reviewItemId } from "./review-selection";

const {
  mockAddMRPanel,
  mockAddPRPanel,
  mockAddReviewPanel,
  mockAddTodosPanel,
  mockAddPromptHistoryPanel,
  mockListTaskCanvases,
  mockAddCanvasPanel,
} = vi.hoisted(() => ({
  mockAddMRPanel: vi.fn(),
  mockAddPRPanel: vi.fn(),
  mockAddReviewPanel: vi.fn(),
  mockAddTodosPanel: vi.fn(),
  mockAddPromptHistoryPanel: vi.fn(),
  mockListTaskCanvases: vi.fn(),
  mockAddCanvasPanel: vi.fn(),
}));

const mockDockviewStore = vi.hoisted(() => ({
  api: null as null | {
    getPanel: (id: string) => { params?: Record<string, unknown> } | undefined;
    addPanel: typeof mockAddCanvasPanel;
  },
  centerGroupId: "group-center",
  addBrowserPanel: vi.fn(),
  addVscodePanel: vi.fn(),
  addPlanPanel: vi.fn(),
  addPluginPanel: vi.fn(),
  addTodosPanel: mockAddTodosPanel,
  addPromptHistoryPanel: mockAddPromptHistoryPanel,
  addChangesPanel: vi.fn(),
  addFilesPanel: vi.fn(),
  addPRPanel: mockAddPRPanel,
  addMRPanel: mockAddMRPanel,
  addReviewPanel: mockAddReviewPanel,
  addTerminalPanel: vi.fn(),
}));
const CENTER_GROUP_ID = mockDockviewStore.centerGroupId;
const SECONDARY_GROUP_ID = "group-secondary";

const mockNormalizedReviews = vi.hoisted(() => ({
  reviews: [] as ReviewItemSummary[],
}));
const mockCanvasFeature = vi.hoisted(() => ({ enabled: true }));

const mockAppState = vi.hoisted(() => ({
  workspaceId: "workspace-1",
  workspaces: { activeId: "workspace-1" },
  tasks: { activeSessionId: "session-1", activeTaskId: null },
  kanban: { tasks: [] },
  taskSessionsByTask: {
    itemsByTaskId: { "task-1": [] },
    loadingByTaskId: {},
    loadedByTaskId: { "task-1": true },
  },
  connection: { status: "connected" },
  taskPRs: { byTaskId: {} },
  taskMRs: { byWorkspaceId: {} },
  taskSessions: { items: {} },
  repositories: { itemsByWorkspaceId: {} },
  userShells: { byEnvironmentId: {} },
  updateUserShell: vi.fn(),
  removeUserShell: vi.fn(),
  agentProfiles: { items: [] },
}));
const TEST_WORKSPACE_ID = mockAppState.workspaceId;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector(mockAppState),
  useAppStoreApi: () => ({ getState: () => mockAppState }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: unknown) => unknown) => selector(mockDockviewStore),
}));

vi.mock("@/hooks/use-environment-session-id", () => ({
  useEnvironmentId: () => null,
}));

vi.mock("@/hooks/domains/workspace/use-repository-scripts", () => ({
  useRepositoryScripts: () => ({ scripts: [] }),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mockCanvasFeature.enabled,
}));

vi.mock("@/lib/api/domains/canvas-api", () => ({
  listTaskCanvases: mockListTaskCanvases,
}));

vi.mock("./review-panel-provider", () => ({
  useNormalizedTaskReviews: () => mockNormalizedReviews.reviews,
}));

const PR_SUBMENU_TEST_ID = "add-panel-pr-submenu";
const INVOKING_GROUP = "group-center";
const PR_ITEM_TEST_ID_PREFIX = "add-panel-pr-item-";
const GITLAB_ORIGIN = "https://gitlab.example";
const TEST_TIMESTAMP = "2026-07-31T00:00:00Z";

/** Builds an open TaskPR for the acme/kandev task with the given id, number, and repo. */
function makePR(id: string, number: number, repo = "kandev"): TaskPR {
  return {
    id,
    workspace_id: TEST_WORKSPACE_ID,
    task_id: "task-1",
    owner: "acme",
    repo,
    pr_number: number,
    pr_url: `https://github.com/acme/${repo}/pull/${number}`,
    pr_title: `PR ${number}`,
    head_branch: `feature/${number}`,
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: TEST_TIMESTAMP,
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: TEST_TIMESTAMP,
  };
}

/** Builds an open TaskMR hosted on GITLAB_ORIGIN with the given id, number, and project path. */
function makeMR(id: string, number: number, projectPath = "acme/kandev"): TaskMR {
  return {
    id,
    task_id: "task-1",
    host: GITLAB_ORIGIN,
    project_path: projectPath,
    mr_iid: number,
    mr_url: `${GITLAB_ORIGIN}/${projectPath}/-/merge_requests/${number}`,
    mr_title: `MR ${number}`,
    head_branch: `feature/${number}`,
    base_branch: "main",
    author_username: "alice",
    state: "open",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    reviewer_count: 0,
    unapproved_reviewers: 0,
    unresolved_discussions: 0,
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
  };
}

/** Builds an open bitbucket ReviewItemSummary for the given change request number. */
function makeRegisteredReview(number: number): ReviewItemSummary {
  return {
    providerId: "bitbucket",
    reviewKey: `acme/kandev/${number}`,
    title: `Change request ${number}`,
    url: `https://bitbucket.example/acme/kandev/pull-requests/${number}`,
    connectionScope: "https://bitbucket.example",
    repositoryId: "acme/kandev",
    changeRequestNumber: number,
    state: "open",
  };
}

/** Stubs the dockview store API so getPanel reports the given open panels and their params. */
function setOpenPanels(panels: Record<string, Record<string, unknown>>) {
  mockDockviewStore.api = {
    getPanel: (id) => (Object.hasOwn(panels, id) ? { params: panels[id] } : undefined),
    addPanel: mockAddCanvasPanel,
  };
}

/** Renders AddPanelMenuItems inside an open dropdown with the given menu state and target group. */
function renderMenu(state: Partial<AddPanelMenuState> = {}, groupId = CENTER_GROUP_ID) {
  const fullState: AddPanelMenuState = {
    taskId: null,
    isPassthrough: false,
    hasChanges: false,
    hasFiles: false,
    prs: [],
    mrs: [],
    ...state,
  };
  return render(
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger>open</DropdownMenuTrigger>
      <DropdownMenuContent>
        <AddPanelMenuItems
          groupId={groupId}
          state={fullState}
          onNewSession={() => {}}
          onAddTerminal={() => {}}
          onRunScript={() => {}}
          onRunDevScript={() => {}}
        />
      </DropdownMenuContent>
    </DropdownMenu>,
  );
}

/** Opens the PR submenu trigger and resolves the acme-web-42 PR row once it renders. */
async function openPRSubmenu() {
  const trigger = screen.getByTestId(PR_SUBMENU_TEST_ID);
  fireEvent.click(trigger);
  return screen.findByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-web-42`);
}

beforeEach(() => {
  mockCanvasFeature.enabled = true;
  mockDockviewStore.api = {
    getPanel: () => undefined,
    addPanel: mockAddCanvasPanel,
  };
  mockNormalizedReviews.reviews = [];
  mockListTaskCanvases.mockReset();
  mockListTaskCanvases.mockResolvedValue({ canvases: [] });
  mockAddCanvasPanel.mockClear();
  mockAddMRPanel.mockClear();
  mockAddPRPanel.mockClear();
  mockAddReviewPanel.mockClear();
  mockAddTodosPanel.mockClear();
  mockAddPromptHistoryPanel.mockClear();
});

afterEach(() => cleanup());

describe("AddPanelMenuItems — linked PR rows", () => {
  it("renders no PR row and no submenu trigger when no PRs are linked", () => {
    renderMenu();
    expect(screen.queryByTestId(PR_SUBMENU_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(new RegExp(`^${PR_ITEM_TEST_ID_PREFIX}`))).toBeNull();
  });

  it("renders a single PR as one inline row without a submenu trigger", () => {
    renderMenu({ prs: [makePR("pr-1", 42)] });
    const row = screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-kandev-42`);
    expect(row).toBeTruthy();
    expect(row.textContent).toContain("PR #42");
    expect(screen.queryByTestId(PR_SUBMENU_TEST_ID)).toBeNull();
  });

  it("renders a Pull requests submenu trigger instead of inline rows when multiple PRs are linked", () => {
    renderMenu({ prs: [makePR("pr-1", 42, "web"), makePR("pr-2", 77, "api")] });
    expect(screen.getByTestId(PR_SUBMENU_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(new RegExp(`^${PR_ITEM_TEST_ID_PREFIX}`))).toBeNull();
  });

  it("lists each PR inside the submenu with a repo-disambiguated label", async () => {
    renderMenu({ prs: [makePR("pr-1", 42, "web"), makePR("pr-2", 77, "api")] });
    await openPRSubmenu();

    const webRow = screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-web-42`);
    const apiRow = screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-api-77`);
    expect(webRow.textContent).toContain("PR #42");
    expect(webRow.textContent).toContain("acme/web");
    expect(apiRow.textContent).toContain("PR #77");
    expect(apiRow.textContent).toContain("acme/api");
  });

  it("opens the selected PR panel with its task key and the active session", async () => {
    renderMenu({ prs: [makePR("pr-1", 42, "web"), makePR("pr-2", 77, "api")] }, SECONDARY_GROUP_ID);
    await openPRSubmenu();

    fireEvent.click(screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-api-77`));
    expect(mockAddPRPanel).toHaveBeenCalledWith("acme/api/77", {
      groupId: SECONDARY_GROUP_ID,
    });
  });

  it("keeps the inline row clickable when exactly one PR is linked", () => {
    renderMenu({ prs: [makePR("pr-1", 42)] });
    fireEvent.click(screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-kandev-42`));
    expect(mockAddPRPanel).toHaveBeenCalledWith("acme/kandev/42", {
      groupId: CENTER_GROUP_ID,
    });
  });

  it("hides a canonical open PR and renders the sole missing PR inline", () => {
    setOpenPanels({ "pr-detail": { prKey: "acme/web/42" } });

    renderMenu({ prs: [makePR("pr-1", 42, "web"), makePR("pr-2", 77, "api")] });

    expect(screen.queryByTestId(PR_SUBMENU_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-web-42`)).toBeNull();
    expect(screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-api-77`)).toBeTruthy();
  });

  it("hides a keyed open PR by exact task key", () => {
    setOpenPanels({ "pr-detail|acme/web/42": {} });

    renderMenu({ prs: [makePR("pr-1", 42, "web"), makePR("pr-2", 77, "api")] });

    expect(screen.queryByTestId(PR_SUBMENU_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-web-42`)).toBeNull();
    expect(screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-api-77`)).toBeTruthy();
  });

  it("uses the filtered PR count for the submenu and keeps other reviews available", async () => {
    setOpenPanels({ "pr-detail": { prKey: "acme/web/42" } });

    renderMenu({
      prs: [makePR("pr-1", 42, "web"), makePR("pr-2", 77, "api"), makePR("pr-3", 91, "docs")],
    });

    const trigger = screen.getByTestId(PR_SUBMENU_TEST_ID);
    expect(trigger.getAttribute("data-pr-count")).toBe("2");
    fireEvent.click(trigger);
    expect(await screen.findByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-api-77`)).toBeTruthy();
    expect(screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-docs-91`)).toBeTruthy();
    expect(screen.queryByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-web-42`)).toBeNull();
  });

  it("offers a PR on the next menu mount after its exact panel closes", () => {
    setOpenPanels({ "pr-detail|acme/kandev/42": {} });
    const firstMenu = renderMenu({ prs: [makePR("pr-1", 42)] });
    expect(screen.queryByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-kandev-42`)).toBeNull();

    firstMenu.unmount();
    setOpenPanels({});
    renderMenu({ prs: [makePR("pr-1", 42)] });

    expect(screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-kandev-42`)).toBeTruthy();
  });
});

describe("AddPanelMenuItems — open review identities", () => {
  it("opens a missing GitLab MR in the invoking group", () => {
    const mr = makeMR("mr-1", 42);
    renderMenu({ mrs: [mr] }, SECONDARY_GROUP_ID);

    fireEvent.click(screen.getByTestId("add-panel-mr-item-mr-1"));

    expect(mockAddMRPanel).toHaveBeenCalledWith(mrTaskKey(mr), {
      groupId: SECONDARY_GROUP_ID,
    });
  });

  it("opens a missing registered-provider review in the invoking group", () => {
    const review = makeRegisteredReview(42);
    mockNormalizedReviews.reviews = [review];
    renderMenu({}, SECONDARY_GROUP_ID);

    fireEvent.click(screen.getByTestId(`add-panel-review-item-${reviewItemId(review)}`));

    expect(mockAddReviewPanel).toHaveBeenCalledWith(review, {
      groupId: SECONDARY_GROUP_ID,
    });
  });

  it("hides canonical and keyed GitLab MRs by exact task key", () => {
    const canonicalMR = makeMR("mr-1", 42, "acme/web");
    const keyedMR = makeMR("mr-2", 77, "acme/api");
    const missingMR = makeMR("mr-3", 91, "acme/docs");
    setOpenPanels({
      "pr-detail": { mrKey: mrTaskKey(canonicalMR) },
      [`mr-detail|${mrTaskKey(keyedMR)}`]: {},
    });

    renderMenu({ mrs: [canonicalMR, keyedMR, missingMR] });

    expect(screen.queryByTestId("add-panel-mr-item-mr-1")).toBeNull();
    expect(screen.queryByTestId("add-panel-mr-item-mr-2")).toBeNull();
    expect(screen.getByTestId("add-panel-mr-item-mr-3")).toBeTruthy();
  });

  it("uses the single-MR label when filtering leaves one visible MR", () => {
    const openMR = makeMR("mr-1", 42, "acme/web");
    const missingMR = makeMR("mr-2", 77, "acme/api");
    setOpenPanels({ "pr-detail": { mrKey: mrTaskKey(openMR) } });

    renderMenu({ mrs: [openMR, missingMR] });

    expect(screen.getByTestId("add-panel-mr-item-mr-2").textContent).toContain("Merge Request !77");
  });

  it("hides registered-provider reviews with exact canonical and keyed identities", () => {
    const canonicalReview = makeRegisteredReview(42);
    const keyedReview = makeRegisteredReview(77);
    const missingReview = makeRegisteredReview(91);
    setOpenPanels({
      "pr-detail": {
        providerId: canonicalReview.providerId,
        connectionScope: canonicalReview.connectionScope,
        repositoryId: canonicalReview.repositoryId,
        changeRequestNumber: canonicalReview.changeRequestNumber,
      },
      [reviewPanelId(keyedReview)]: {},
    });
    mockNormalizedReviews.reviews = [canonicalReview, keyedReview, missingReview];

    renderMenu();

    expect(
      screen.queryByTestId(`add-panel-review-item-${reviewItemId(canonicalReview)}`),
    ).toBeNull();
    expect(screen.queryByTestId(`add-panel-review-item-${reviewItemId(keyedReview)}`)).toBeNull();
    expect(screen.getByTestId(`add-panel-review-item-${reviewItemId(missingReview)}`)).toBeTruthy();
  });

  it("does not hide an individual review for a provider-neutral canonical selector", () => {
    const review = makeRegisteredReview(42);
    setOpenPanels({ "pr-detail": {} });
    mockNormalizedReviews.reviews = [review];

    renderMenu();

    expect(screen.getByTestId(`add-panel-review-item-${reviewItemId(review)}`)).toBeTruthy();
  });
});

describe("AddPanelMenuItems — plugin task panels (AC1)", () => {
  /** Stub task-panel component that renders nothing. */
  function Notes() {
    return null;
  }

  afterEach(() => {
    pluginRegistry.unregisterPlugin("kandev-plugin-notes");
  });

  it("renders no plugin row when no task panel is registered", () => {
    renderMenu();
    expect(screen.queryByTestId(/^add-panel-plugin-item-/)).toBeNull();
  });

  it("renders one row per registered task panel and opens it via addPluginPanel", () => {
    pluginRegistry
      .forPlugin("kandev-plugin-notes")
      .registerTaskPanel({ id: "notes", title: "Notes", Component: Notes });

    renderMenu();
    const row = screen.getByTestId("add-panel-plugin-item-kandev-plugin-notes-notes");
    expect(row.textContent).toContain("Notes");

    fireEvent.click(row);
    expect(mockDockviewStore.addPluginPanel).toHaveBeenCalledWith(
      "kandev-plugin-notes",
      "notes",
      "Notes",
      { groupId: CENTER_GROUP_ID },
    );
  });
});

describe("AddPanelMenuItems — task canvases", () => {
  it("offers an applicable canvas and opens it in the invoking dockview group", async () => {
    mockListTaskCanvases.mockResolvedValueOnce({
      canvases: [
        {
          id: "canvas-1",
          plugin_instance_id: "instance-1",
          plugin_id: "plugin-1",
          workspace_id: TEST_WORKSPACE_ID,
          task_id: "task-1",
          scope_kind: "task",
          title: "Release board",
          status: "active",
          active_release_id: "release-1",
          active_release_status: "valid",
        },
      ],
    });

    renderMenu({ taskId: "task-1" }, SECONDARY_GROUP_ID);

    const item = await screen.findByTestId("add-panel-canvas-item-canvas-1");
    expect(item.textContent).toContain("Release board");
    fireEvent.click(item);

    expect(mockAddCanvasPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "canvas:canvas-1",
        component: "canvas",
        title: "Release board",
        params: { canvasId: "canvas-1" },
        position: { referenceGroup: SECONDARY_GROUP_ID },
      }),
    );
  });

  it("shows pending and error task canvases and omits archived canvases", async () => {
    mockListTaskCanvases.mockResolvedValueOnce({
      canvases: [
        {
          id: "pending-canvas-1",
          plugin_instance_id: "instance-2",
          plugin_id: "plugin-1",
          workspace_id: TEST_WORKSPACE_ID,
          task_id: "task-1",
          scope_kind: "task",
          title: "Pending board",
          status: "pending",
        },
        {
          id: "error-canvas-1",
          plugin_instance_id: "instance-3",
          plugin_id: "plugin-1",
          workspace_id: TEST_WORKSPACE_ID,
          task_id: "task-1",
          scope_kind: "task",
          title: "Error board",
          status: "error",
        },
        {
          id: "archived-canvas-1",
          plugin_instance_id: "instance-4",
          plugin_id: "plugin-1",
          workspace_id: TEST_WORKSPACE_ID,
          task_id: "task-1",
          scope_kind: "task",
          title: "Archived board",
          status: "archived",
        },
      ],
    });

    renderMenu({ taskId: "task-1" });

    expect(await screen.findByTestId("add-panel-canvas-item-pending-canvas-1")).toBeTruthy();
    expect(screen.getByTestId("add-panel-canvas-item-error-canvas-1")).toBeTruthy();
    expect(screen.queryByTestId("add-panel-canvas-item-archived-canvas-1")).toBeNull();
  });

  it("does not load or render canvases while the feature is disabled", () => {
    mockCanvasFeature.enabled = false;

    renderMenu({ taskId: "task-1" });

    expect(mockListTaskCanvases).not.toHaveBeenCalled();
    expect(screen.queryByTestId(/^add-panel-canvas-item-/)).toBeNull();
  });
});

describe("AddPanelMenuItems — port forwarding preference", () => {
  /** Builds a default AddPanelMenuState port-forwarding object, merged with the given overrides. */
  function portForwardingState(
    overrides: Partial<NonNullable<AddPanelMenuState["portForwarding"]>> = {},
  ): NonNullable<AddPanelMenuState["portForwarding"]> {
    return {
      enabled: false,
      canToggle: true,
      isUpdating: false,
      dialogOpen: false,
      setDialogOpen: vi.fn(),
      togglePortForwarding: vi.fn(),
      ...overrides,
    };
  }

  it("renders a checkable row and opens after enabling", () => {
    const togglePortForwarding = vi.fn();
    renderMenu({
      taskId: null,
      portForwarding: portForwardingState({ togglePortForwarding }),
    });

    const row = screen.getByTestId("port-forwarding-menu-item");
    expect(row.getAttribute("role")).toBe("menuitemcheckbox");
    expect(row.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(row);

    expect(togglePortForwarding).toHaveBeenCalledWith({ openDialogOnEnable: true });
  });

  it("disables the row when the task session is not ready", () => {
    const togglePortForwarding = vi.fn();
    renderMenu({
      taskId: null,
      portForwarding: portForwardingState({ canToggle: false, togglePortForwarding }),
    });

    const row = screen.getByTestId("port-forwarding-menu-item");
    expect(row.getAttribute("data-disabled")).toBe("");
    fireEvent.click(row);
    expect(togglePortForwarding).not.toHaveBeenCalled();
  });
});

describe("AddPanelMenuItems — Prompt history", () => {
  it("renders a row that opens the panel in the invoking group", () => {
    renderMenu();
    const item = screen.getByTestId("add-panel-prompt-history-item");
    expect(item.textContent).toBe("Prompt history");

    fireEvent.click(item);
    expect(mockAddPromptHistoryPanel).toHaveBeenCalledWith({ groupId: INVOKING_GROUP });
  });

  it("hides the Prompt history row for a passthrough session", () => {
    renderMenu({ isPassthrough: true });
    expect(screen.queryByTestId("add-panel-prompt-history-item")).toBeNull();
  });
});

describe("AddPanelMenuItems — Todos", () => {
  it("renders an always-available Todos row that opens/focuses the panel in the invoking group", () => {
    renderMenu();
    const item = screen.getByText("Todos");
    expect(item).toBeTruthy();

    fireEvent.click(item);
    expect(mockAddTodosPanel).toHaveBeenCalledWith({ groupId: INVOKING_GROUP });
  });

  it("hides the Todos row for a passthrough session, matching the Plan row's guard", () => {
    renderMenu({ isPassthrough: true });
    expect(screen.queryByText("Todos")).toBeNull();
    expect(screen.queryByText("Plan")).toBeNull();
  });
});
