import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import type { TaskPR } from "@/lib/types/github";
import { AddPanelMenuItems, type AddPanelMenuState } from "./dockview-add-panel-items";
import { pluginRegistry } from "@/lib/plugins/registry";

const { mockAddPRPanel, mockAddTodosPanel } = vi.hoisted(() => ({
  mockAddPRPanel: vi.fn(),
  mockAddTodosPanel: vi.fn(),
}));

const mockDockviewStore = vi.hoisted(() => ({
  api: null,
  centerGroupId: "group-center",
  addBrowserPanel: vi.fn(),
  addVscodePanel: vi.fn(),
  addPlanPanel: vi.fn(),
  addPluginPanel: vi.fn(),
  addTodosPanel: mockAddTodosPanel,
  addChangesPanel: vi.fn(),
  addFilesPanel: vi.fn(),
  addPRPanel: mockAddPRPanel,
  addMRPanel: vi.fn(),
  addTerminalPanel: vi.fn(),
}));

const mockAppState = vi.hoisted(() => ({
  tasks: { activeSessionId: "session-1", activeTaskId: null },
  taskSessions: { items: {} },
  repositories: { itemsByWorkspaceId: {} },
  userShells: { byEnvironmentId: {} },
  updateUserShell: vi.fn(),
  removeUserShell: vi.fn(),
  agentProfiles: { items: [] },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector(mockAppState),
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

const PR_SUBMENU_TEST_ID = "add-panel-pr-submenu";
const PR_ITEM_TEST_ID_PREFIX = "add-panel-pr-item-";

function makePR(id: string, number: number, repo = "kandev"): TaskPR {
  return {
    id,
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
    created_at: "2026-07-31T00:00:00Z",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "2026-07-31T00:00:00Z",
  };
}

function renderMenu(state: Partial<AddPanelMenuState> = {}) {
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
          groupId="group-center"
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

async function openPRSubmenu() {
  const trigger = screen.getByTestId(PR_SUBMENU_TEST_ID);
  fireEvent.click(trigger);
  return screen.findByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-web-42`);
}

beforeEach(() => {
  mockAddPRPanel.mockClear();
  mockAddTodosPanel.mockClear();
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
    renderMenu({ prs: [makePR("pr-1", 42, "web"), makePR("pr-2", 77, "api")] });
    await openPRSubmenu();

    fireEvent.click(screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-api-77`));
    expect(mockAddPRPanel).toHaveBeenCalledWith("acme/api/77");
  });

  it("keeps the inline row clickable when exactly one PR is linked", () => {
    renderMenu({ prs: [makePR("pr-1", 42)] });
    fireEvent.click(screen.getByTestId(`${PR_ITEM_TEST_ID_PREFIX}acme-kandev-42`));
    expect(mockAddPRPanel).toHaveBeenCalledWith("acme/kandev/42");
  });
});

describe("AddPanelMenuItems — plugin task panels (AC1)", () => {
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
      { groupId: "group-center" },
    );
  });
});

describe("AddPanelMenuItems — port forwarding preference", () => {
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

describe("AddPanelMenuItems — Todos", () => {
  it("renders an always-available Todos row that opens/focuses the panel in the invoking group", () => {
    renderMenu();
    const item = screen.getByText("Todos");
    expect(item).toBeTruthy();

    fireEvent.click(item);
    expect(mockAddTodosPanel).toHaveBeenCalledWith({ groupId: "group-center" });
  });

  it("hides the Todos row for a passthrough session, matching the Plan row's guard", () => {
    renderMenu({ isPassthrough: true });
    expect(screen.queryByText("Todos")).toBeNull();
    expect(screen.queryByText("Plan")).toBeNull();
  });
});
