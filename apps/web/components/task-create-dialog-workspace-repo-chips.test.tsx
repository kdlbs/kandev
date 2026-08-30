import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "./task-create-dialog-types";
import { WorkspaceRepoChips } from "./task-create-dialog-workspace-repo-chips";

vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: () => ({ branches: [], isLoading: false }),
}));

vi.mock("@/hooks/domains/workspace/use-repository-branch-policies", () => ({
  useRepositoryBranchPolicies: () => ({
    policies: [
      {
        id: "policy-1",
        repository_id: "repo-front",
        name: "Feature branches",
        description: "Policy description",
        base_branch: "main",
        branch_template: "feature/{title}-{suffix}",
        pull_request_target: "develop",
        created_at: "2026-08-24T10:00:00Z",
        updated_at: "2026-08-24T10:00:00Z",
      },
    ],
  }),
}));

const FRONTEND_ID = "repo-front";
const BACKEND_ID = "repo-back";
const CHIP_TRIGGER = "repo-chip-trigger";
const ADDED_MARKER = "already-added-repository-marker";
const DISCOVERED_PATH = "/home/me/projects/local-project";
const NOOP = (_key: string, _value: string) => undefined;

function repository(id: string, name: string): Repository {
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

const repositories = [repository(FRONTEND_ID, "frontend"), repository(BACKEND_ID, "backend")];
const rows = [
  row({ key: "r0", repositoryId: FRONTEND_ID, branch: "main" }),
  row({ key: "r1", branch: "develop" }),
];

type ChipsProps = Parameters<typeof WorkspaceRepoChips>[0];

function chips(overrides: Partial<ChipsProps> = {}) {
  return (
    <WorkspaceRepoChips
      rows={rows}
      repositories={repositories}
      workspaceId="ws-1"
      canAddMore
      onAdd={vi.fn()}
      onRemove={vi.fn()}
      onRowRepositoryChange={NOOP}
      onRowBranchChange={NOOP}
      {...overrides}
    />
  );
}

function renderChips(overrides: Partial<ChipsProps> = {}) {
  return render(
    <StateProvider>
      <TooltipProvider>{chips(overrides)}</TooltipProvider>
    </StateProvider>,
  );
}

afterEach(cleanup);

describe("WorkspaceRepoChips duplicate policy", () => {
  it("excludes repositories already selected by another quick-chat row", () => {
    renderChips({ allowDuplicateRepositories: false });
    fireEvent.click(screen.getAllByTestId(CHIP_TRIGGER)[1]);

    expect(screen.queryByRole("option", { name: /^frontend/ })).toBeNull();
    expect(screen.getByRole("option", { name: /^backend/ })).toBeTruthy();
  });

  it("keeps task creation's same-repository different-branch option", () => {
    renderChips({ allowDuplicateRepositories: true });
    fireEvent.click(screen.getAllByTestId(CHIP_TRIGGER)[1]);

    expect(screen.getByRole("option", { name: /^frontend/ })).toBeTruthy();
    expect(screen.getByRole("option", { name: /^backend/ })).toBeTruthy();
  });

  it("does not expose repository creation when the caller does not opt in", () => {
    renderChips({ allowDuplicateRepositories: false });
    fireEvent.click(screen.getAllByTestId(CHIP_TRIGGER)[1]);

    expect(screen.queryByText("Create new repository")).toBeNull();
  });

  it("routes repository creation to the only row", () => {
    const onCreateRepository = vi.fn();
    renderChips({ rows: [rows[0]], onCreateRepository });

    fireEvent.click(screen.getByTestId(CHIP_TRIGGER));
    fireEvent.click(screen.getByTestId("create-local-repository-button"));

    expect(onCreateRepository).toHaveBeenCalledWith("r0");
  });

  it("refreshes repositories from the selector toolbar", () => {
    const onRefreshRepositories = vi.fn();
    renderChips({ rows: [rows[0]], onRefreshRepositories });

    fireEvent.click(screen.getByTestId(CHIP_TRIGGER));
    fireEvent.click(screen.getByTestId("repo-refresh-button"));

    expect(onRefreshRepositories).toHaveBeenCalledOnce();
  });

  it("does not expose repository creation for multi-repository tasks", () => {
    renderChips({ onCreateRepository: vi.fn() });
    fireEvent.click(screen.getAllByTestId(CHIP_TRIGGER)[1]);

    expect(screen.queryByText("Create new repository")).toBeNull();
  });
});

describe("WorkspaceRepoChips branch policy preview", () => {
  it("keeps policy choices on one line and moves details behind an info control", () => {
    renderChips({
      rows: [row({ key: "r0", repositoryId: FRONTEND_ID, branch: "main" })],
      showBranchPolicies: true,
    });

    fireEvent.click(screen.getByTestId("branch-chip-trigger"));

    const option = screen.getByRole("option", { name: /Feature branches/ });
    expect(option.textContent).not.toContain("feature/{title}-{suffix}");
    expect(
      screen.getByTestId("branch-policy-option-info-policy-1").getAttribute("aria-label"),
    ).toContain("feature/{title}-{suffix}");
  });
});

describe("WorkspaceRepoChips workspace markers", () => {
  it("marks another task row's workspace repository while keeping it selectable", () => {
    const onRowRepositoryChange = vi.fn();
    renderChips({ allowDuplicateRepositories: true, onRowRepositoryChange });
    fireEvent.click(screen.getAllByTestId(CHIP_TRIGGER)[1]);

    const selectedElsewhere = screen.getByRole("option", { name: /^frontend/ });
    const marker = within(selectedElsewhere).getByTestId(ADDED_MARKER);
    expect(marker.getAttribute("aria-label")).toBe("Already added");
    expect(marker.classList).toContain("text-primary");
    fireEvent.click(selectedElsewhere);
    expect(onRowRepositoryChange).toHaveBeenCalledWith("r1", FRONTEND_ID);
  });

  it("does not mark the only selected workspace repository when its row is reopened", () => {
    renderChips({
      allowDuplicateRepositories: true,
      rows: [row({ key: "r0", repositoryId: FRONTEND_ID })],
    });
    fireEvent.click(screen.getByTestId(CHIP_TRIGGER));
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("option", { name: /^frontend/ })).toBeNull();
    fireEvent.click(screen.getByTestId(CHIP_TRIGGER));

    expect(
      within(screen.getByRole("option", { name: /^frontend/ })).queryByTestId(ADDED_MARKER),
    ).toBeNull();
  });

  it("clears a workspace marker when the selecting sibling changes or is removed", () => {
    const { rerender } = renderChips({
      allowDuplicateRepositories: true,
      rows: [row({ key: "r0", repositoryId: FRONTEND_ID }), row({ key: "r1" })],
    });
    fireEvent.click(screen.getAllByTestId(CHIP_TRIGGER)[1]);
    expect(
      within(screen.getByRole("option", { name: /^frontend/ })).getByTestId(ADDED_MARKER),
    ).toBeTruthy();

    rerender(
      <StateProvider>
        <TooltipProvider>
          {chips({ rows: [row({ key: "r0", repositoryId: BACKEND_ID }), row({ key: "r1" })] })}
        </TooltipProvider>
      </StateProvider>,
    );
    expect(
      within(screen.getByRole("option", { name: /^frontend/ })).queryByTestId(ADDED_MARKER),
    ).toBeNull();
    expect(
      within(screen.getByRole("option", { name: /^backend/ })).getByTestId(ADDED_MARKER),
    ).toBeTruthy();

    rerender(
      <StateProvider>
        <TooltipProvider>{chips({ rows: [row({ key: "r1" })] })}</TooltipProvider>
      </StateProvider>,
    );
    expect(
      within(screen.getByRole("option", { name: /^backend/ })).queryByTestId(ADDED_MARKER),
    ).toBeNull();
  });
});

describe("WorkspaceRepoChips discovered markers", () => {
  it("marks normalized discovered paths selected by another task row and clears on rerender", () => {
    const discoveredRepositories = [
      { path: DISCOVERED_PATH, name: "local-project" },
    ] as unknown as DialogFormState["discoveredRepositories"];
    const { rerender } = renderChips({
      allowDuplicateRepositories: true,
      rows: [row({ key: "r0", localPath: `${DISCOVERED_PATH}/` }), row({ key: "r1" })],
      discoveredRepositories,
    });
    fireEvent.click(screen.getAllByTestId(CHIP_TRIGGER)[1]);
    expect(
      within(screen.getByRole("option", { name: /^local-project/ })).getByTestId(ADDED_MARKER),
    ).toBeTruthy();

    rerender(
      <StateProvider>
        <TooltipProvider>
          {chips({
            rows: [
              row({ key: "r0", localPath: "/home/me/projects/another-project" }),
              row({ key: "r1" }),
            ],
            discoveredRepositories,
          })}
        </TooltipProvider>
      </StateProvider>,
    );
    expect(
      within(screen.getByRole("option", { name: /^local-project/ })).queryByTestId(ADDED_MARKER),
    ).toBeNull();

    rerender(
      <StateProvider>
        <TooltipProvider>
          {chips({ rows: [row({ key: "r1" })], discoveredRepositories })}
        </TooltipProvider>
      </StateProvider>,
    );
    expect(
      within(screen.getByRole("option", { name: /^local-project/ })).queryByTestId(ADDED_MARKER),
    ).toBeNull();
  });
});
