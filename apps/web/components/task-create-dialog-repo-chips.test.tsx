import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import type { Branch, Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "./task-create-dialog-types";
import { TooltipProvider } from "@kandev/ui/tooltip";

// The unified branch mock lets tests override results and inspect whether
// chips select an id-based or path-based source.
const lastBranchSource = vi.hoisted((): { value: unknown } => ({ value: null }));
const mockBranches = vi.hoisted((): { value: { branches: Branch[]; isLoading: boolean } } => ({
  value: { branches: [], isLoading: false },
}));

vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: (source: unknown) => {
    lastBranchSource.value = source;
    return mockBranches.value;
  },
}));

// The Remote-mode branch of RepoChipsRow renders RemoteRepoChipsRow, which
// in turn renders RemoteRepoChip — a heavy popover with its own GitHub
// hook. Stub the chip here so tests for this row stay focused on the
// branching logic (workspace chips vs. remote chips vs. folder picker).
vi.mock("./task-create-dialog-remote-repo-chip", () => ({
  RemoteRepoChip: () => <div data-testid="remote-repo-chip" />,
  selectedRemoteRepositoryIdentity: () => null,
}));

import { RepoChipsRow } from "./task-create-dialog-repo-chips";

afterEach(cleanup);

const REPO_FRONT_ID = "repo-front";
const REPO_BACK_ID = "repo-back";
const REPO_CHIP_TRIGGER = "repo-chip-trigger";
const DISCOVERED_REPO_PATH = "/home/me/projects/local-project";

function makeRepo(id: string, name: string): Repository {
  return {
    id,
    workspace_id: "ws-1",
    name,
    source_type: "local",
    local_path: `/repos/${name}`,
    default_branch: "main",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  } as Repository;
}

function row(overrides: Partial<TaskRepoRow> = {}): TaskRepoRow {
  return { key: `row-${Math.random()}`, branch: "", ...overrides };
}

function makeFs(overrides: Partial<DialogFormState>): DialogFormState {
  const branchesByUrl = {
    branches: (url: string) =>
      url === (overrides.remoteRepos?.[0]?.url ?? "")
        ? ((overrides as Partial<DialogFormState>).branchesByUrl?.branches(url) ?? [])
        : [],
    loading: () => false,
    ensure: () => undefined,
  };
  return {
    repositories: [] as TaskRepoRow[],
    useRemote: false,
    discoveredRepositories: [],
    remoteRepos: [] as DialogFormState["remoteRepos"],
    setRemoteRepos: vi.fn(),
    addRemoteRepo: vi.fn(),
    removeRemoteRepo: vi.fn(),
    updateRemoteRepo: vi.fn(),
    githubUrlError: null,
    branchesByUrl,
    prInfoByUrl: {
      info: () => undefined,
      loading: () => false,
      ensure: () => undefined,
      clear: () => undefined,
    },
    addRepository: vi.fn(),
    removeRepository: vi.fn(),
    updateRepository: vi.fn(),
    setRepositories: vi.fn(),
    ...overrides,
  } as unknown as DialogFormState;
}

const NOOP = (_key: string, _value: string) => undefined;

function renderInProvider(ui: Parameters<typeof render>[0]) {
  return render(<TooltipProvider>{ui}</TooltipProvider>);
}

// eslint-disable-next-line max-lines-per-function -- test describe block, splitting hurts readability
describe("RepoChipsRow", () => {
  it("keeps the compact Repo, Remote, and None source-mode controls and test IDs", () => {
    const onToggleRemote = vi.fn();
    const onToggleNoRepository = vi.fn();
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({ repositories: [row({ key: "r0", repositoryId: REPO_FRONT_ID })] })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
        onToggleRemote={onToggleRemote}
        onToggleNoRepository={onToggleNoRepository}
      />,
    );

    expect(screen.getByTestId("source-mode-workspace").textContent).toBe("Repo");
    expect(screen.getByTestId("source-mode-remote").textContent).toBe("Remote");
    expect(screen.getByTestId("source-mode-scratch").textContent).toBe("None");
    expect(screen.getByTestId("source-mode-workspace").className).not.toContain("min-h-11");
    fireEvent.click(screen.getByTestId("source-mode-remote"));
    expect(onToggleRemote).toHaveBeenCalledOnce();
  });

  it("renders one chip per row plus an Add button", () => {
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({ repositories: [row({ key: "r0", repositoryId: REPO_FRONT_ID })] })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend"), makeRepo(REPO_BACK_ID, "backend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    expect(screen.getAllByTestId("repo-chip")).toHaveLength(1);
    expect(screen.getByTestId("add-repository")).toBeTruthy();
  });

  it("renders one chip per row across multiple repos", () => {
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({
          repositories: [
            row({ key: "r0", repositoryId: REPO_FRONT_ID }),
            row({ key: "r1", repositoryId: REPO_BACK_ID, branch: "main" }),
          ],
        })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend"), makeRepo(REPO_BACK_ID, "backend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    expect(screen.getAllByTestId("repo-chip")).toHaveLength(2);
  });

  it("renders the remote chips row in Remote mode (workspace chips suppressed)", () => {
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({
          useRemote: true,
          remoteRepos: [{ key: "remote-0", url: "", branch: "", source: "paste" }],
        })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
        onToggleRemote={() => undefined}
      />,
    );
    expect(screen.getByTestId("repo-chips-row")).toBeTruthy();
    expect(screen.queryAllByTestId("repo-chip")).toHaveLength(0);
    expect(screen.getByTestId("remote-repo-chips-row")).toBeTruthy();
  });

  it("hides the chip row when the task is already started", () => {
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({ repositories: [row({ key: "r0", repositoryId: REPO_FRONT_ID })] })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    expect(screen.queryByTestId("repo-chips-row")).toBeNull();
  });

  it("local-executor row autoselects the workspace's current branch when available", () => {
    mockBranches.value = {
      branches: [
        { name: "main", type: "local" } as Branch,
        { name: "feature/x", type: "local" } as Branch,
      ],
      isLoading: false,
    };
    const onRowBranchChange = vi.fn();
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({
          repositories: [row({ key: "r0", repositoryId: REPO_FRONT_ID })],
          currentLocalBranch: "feature/x",
          currentLocalBranchLoading: false,
        })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={onRowBranchChange}
        isLocalExecutor
      />,
    );
    // The autoselect effect prefers preferredDefaultBranch (currentLocalBranch
    // for local mode) over the last-used / main fallback. This is what surfaces
    // the workspace's actual on-disk branch in the chip and ensures the submit
    // payload always carries an explicit value (not "" → backend default).
    expect(onRowBranchChange).toHaveBeenCalledWith("r0", "feature/x");
  });

  it("uses the symbolic current branch for an unborn local repository", () => {
    mockBranches.value = { branches: [], isLoading: false };
    const onRowBranchChange = vi.fn();
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({
          repositories: [row({ key: "r0", repositoryId: REPO_FRONT_ID })],
          currentLocalBranch: "main",
          currentLocalBranchLoading: false,
        })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={onRowBranchChange}
        isLocalExecutor
      />,
    );

    expect(onRowBranchChange).toHaveBeenCalledWith("r0", "main");
  });

  it("local-executor row shows the loading placeholder while resolving the current branch", () => {
    mockBranches.value = { branches: [], isLoading: false };
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({
          repositories: [row({ key: "r0", repositoryId: REPO_FRONT_ID })],
          currentLocalBranch: "",
          currentLocalBranchLoading: true,
        })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
        isLocalExecutor
      />,
    );
    // The chip shouldn't lie about an unset state during the brief window
    // before local-status resolves; preferredDefaultBranchLoading drives the
    // "loading…" placeholder.
    expect(screen.getByText(/loading…/i)).toBeTruthy();
  });

  it("keeps Add enabled when all repos are selected (multi-branch: same repo + different branch is valid)", () => {
    // With multi-branch support, the same repo can appear on multiple rows as long
    // as each row uses a different branch. Therefore Add is always enabled when
    // any workspace repos exist — the user can always add another branch of an
    // existing repo.
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({
          repositories: [
            row({ key: "r0", repositoryId: REPO_FRONT_ID }),
            row({ key: "r1", repositoryId: REPO_BACK_ID }),
          ],
        })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend"), makeRepo(REPO_BACK_ID, "backend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    expect((screen.getByTestId("add-repository") as HTMLButtonElement).disabled).toBe(false);
  });

  it("calls fs.addRepository when the + button is clicked", () => {
    const fs = makeFs({ repositories: [row({ key: "r0", repositoryId: REPO_FRONT_ID })] });
    renderInProvider(
      <RepoChipsRow
        fs={fs}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend"), makeRepo(REPO_BACK_ID, "backend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    fireEvent.click(screen.getByTestId("add-repository"));
    expect(fs.addRepository).toHaveBeenCalledOnce();
  });

  it("removing a chip calls fs.removeRepository(key)", () => {
    const fs = makeFs({
      repositories: [
        row({ key: "r0", repositoryId: REPO_FRONT_ID, branch: "main" }),
        row({ key: "r1", repositoryId: REPO_BACK_ID, branch: "develop" }),
      ],
    });
    renderInProvider(
      <RepoChipsRow
        fs={fs}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend"), makeRepo(REPO_BACK_ID, "backend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    fireEvent.click(screen.getAllByTestId("remove-repo-chip")[0]);
    expect(fs.removeRepository).toHaveBeenCalledWith("r0");
  });

  // Regression: discovered (on-machine) repos must surface in the picker
  // dropdown alongside workspace repos. A previous rewrite passed [] for
  // discovered repos and lost them.
  it("includes discovered (on-machine) repos in the picker dropdown", () => {
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({
          repositories: [row({ key: "r0" })],
          discoveredRepositories: [
            { path: DISCOVERED_REPO_PATH, name: "local-project" },
            // Same path as a workspace repo — should NOT appear (dedup by path).
            { path: "/repos/frontend", name: "frontend-dup" },
          ] as unknown as DialogFormState["discoveredRepositories"],
        })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    fireEvent.click(screen.getByTestId(REPO_CHIP_TRIGGER));
    expect(screen.getByText("frontend")).toBeTruthy();
    expect(screen.getByText("~/projects/local-project")).toBeTruthy();
    expect(screen.queryByText("frontend-dup")).toBeNull();
  });

  // Regression: picking a discovered repo passes its path as the value;
  // onRowRepositoryChange must receive it (the handler then resolves to a
  // workspace id or local path). Previously the chip wrote the path into
  // a workspace `repository_id`, causing FK failures on submit.
  it("calls onRowRepositoryChange with the discovered path when picked", () => {
    const onRowRepositoryChange = vi.fn();
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({
          repositories: [row({ key: "r0" })],
          discoveredRepositories: [
            { path: DISCOVERED_REPO_PATH, name: "local-project" },
          ] as unknown as DialogFormState["discoveredRepositories"],
        })}
        repositories={[]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={onRowRepositoryChange}
        onRowBranchChange={NOOP}
      />,
    );
    fireEvent.click(screen.getByTestId(REPO_CHIP_TRIGGER));
    fireEvent.click(screen.getByText("~/projects/local-project"));
    expect(onRowRepositoryChange).toHaveBeenCalledWith("r0", DISCOVERED_REPO_PATH);
  });

  // Regression: discovered (path-keyed) rows used to call the branch loader
  // with no path source, so their branch picker stayed empty. The chip must
  // build a `kind: "path"` source for discovered rows so the unified hook
  // hits the path-based query param instead of trying an id lookup.
  it("discovered rows build a path-source for the branch loader", () => {
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({
          repositories: [row({ key: "r0", localPath: "/home/me/projects/proj" })],
        })}
        repositories={[]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    expect(lastBranchSource.value).toEqual({
      kind: "path",
      workspaceId: "ws-1",
      path: "/home/me/projects/proj",
    });
  });

  it("workspace rows build an id-source for the branch loader", () => {
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({ repositories: [row({ key: "r0", repositoryId: REPO_FRONT_ID })] })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    expect(lastBranchSource.value).toEqual({
      kind: "id",
      workspaceId: "ws-1",
      repositoryId: REPO_FRONT_ID,
    });
  });

  // Regression: remote branches must keep their "origin/" prefix so users
  // can distinguish a local "main" from "origin/main". A prior rewrite
  // dropped the prefix, producing two indistinguishable rows.
  it("remote branches keep their origin/ prefix and don't collide with local names", () => {
    mockBranches.value = {
      isLoading: false,
      branches: [
        { name: "main", type: "local" },
        { name: "main", type: "remote", remote: "origin" },
      ] as unknown as Branch[],
    };
    renderInProvider(
      <RepoChipsRow
        fs={makeFs({ repositories: [row({ key: "r0", repositoryId: REPO_FRONT_ID })] })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted={false}
        workspaceId="ws-1"
        onRowRepositoryChange={NOOP}
        onRowBranchChange={NOOP}
      />,
    );
    fireEvent.click(screen.getByTestId("branch-chip-trigger"));
    expect(screen.getByText("main")).toBeTruthy();
    expect(screen.getByText("origin/main")).toBeTruthy();
    mockBranches.value = { branches: [], isLoading: false };
  });
});
