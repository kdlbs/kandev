import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";

afterEach(cleanup);
import type { Repository } from "@/lib/types/http";
import type { DialogFormState, ExtraRepositoryRow } from "./task-create-dialog-types";

// Mock branches hook so the BranchSelector inside the row doesn't blow up.
vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useRepositoryBranches: () => ({ branches: [], isLoading: false }),
}));

// Defer the import until after mocks are set up.
async function loadComponent() {
  return (await import("./task-create-dialog-extra-repos")).ExtraRepositoryRows;
}

const REPO_FRONT_ID = "repo-front";
const REPO_BACK_ID = "repo-back";

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

function makeFs(overrides: Partial<DialogFormState>): DialogFormState {
  // Only the fields ExtraRepositoryRows actually reads need to be real.
  return {
    repositoryId: REPO_FRONT_ID,
    extraRepositories: [] as ExtraRepositoryRow[],
    addExtraRepository: vi.fn(),
    removeExtraRepository: vi.fn(),
    updateExtraRepository: vi.fn(),
    ...overrides,
  } as unknown as DialogFormState;
}

describe("ExtraRepositoryRows", () => {
  it("disables the Add button until a primary repo is selected", async () => {
    const ExtraRepositoryRows = await loadComponent();
    render(
      <ExtraRepositoryRows
        fs={makeFs({})}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted={false}
        primarySelected={false}
      />,
    );
    const btn = screen.getByTestId("add-extra-repository") as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });

  it("calls addExtraRepository when the Add button is clicked", async () => {
    const ExtraRepositoryRows = await loadComponent();
    const fs = makeFs({});
    render(
      <ExtraRepositoryRows
        fs={fs}
        // Need a second repo so the Add button is enabled (the primary
        // would otherwise be the only choice).
        repositories={[makeRepo(REPO_FRONT_ID, "frontend"), makeRepo(REPO_BACK_ID, "backend")]}
        isTaskStarted={false}
        primarySelected
      />,
    );
    fireEvent.click(screen.getByTestId("add-extra-repository"));
    expect(fs.addExtraRepository).toHaveBeenCalledOnce();
  });

  it("renders one editable row per extra repository", async () => {
    const ExtraRepositoryRows = await loadComponent();
    render(
      <ExtraRepositoryRows
        fs={makeFs({
          extraRepositories: [
            { key: "extra-1", repositoryId: "", branch: "" },
            { key: "extra-2", repositoryId: "", branch: "" },
          ],
        })}
        repositories={[
          makeRepo(REPO_FRONT_ID, "frontend"),
          makeRepo(REPO_BACK_ID, "backend"),
          makeRepo("repo-shared", "shared"),
        ]}
        isTaskStarted={false}
        primarySelected
      />,
    );
    expect(screen.getAllByTestId("extra-repository-row")).toHaveLength(2);
  });

  it("hides the section when the task is already started", async () => {
    const ExtraRepositoryRows = await loadComponent();
    const { container } = render(
      <ExtraRepositoryRows
        fs={makeFs({
          extraRepositories: [{ key: "extra-1", repositoryId: "", branch: "" }],
        })}
        repositories={[makeRepo(REPO_FRONT_ID, "frontend")]}
        isTaskStarted
        primarySelected
      />,
    );
    expect(container.querySelector("[data-testid='extra-repositories']")).toBeNull();
  });

  it("opening a row's dropdown shows the unused workspace repos", async () => {
    const ExtraRepositoryRows = await loadComponent();
    render(
      <ExtraRepositoryRows
        fs={makeFs({
          repositoryId: REPO_FRONT_ID,
          extraRepositories: [{ key: "extra-1", repositoryId: "", branch: "" }],
        })}
        repositories={[
          makeRepo(REPO_FRONT_ID, "frontend"),
          makeRepo(REPO_BACK_ID, "backend"),
          makeRepo("repo-shared", "shared"),
        ]}
        isTaskStarted={false}
        primarySelected
      />,
    );
    // Open the row's repository combobox.
    const triggers = screen.getAllByTestId("repository-selector");
    fireEvent.click(triggers[0]);
    // The primary repo (frontend) is excluded; the other two should be offered.
    expect(screen.queryByText("backend")).not.toBeNull();
    expect(screen.queryByText("shared")).not.toBeNull();
    expect(screen.queryByText("frontend")).toBeNull();
  });

  it("disables the Add button when no other workspace repos are available", async () => {
    const ExtraRepositoryRows = await loadComponent();
    render(
      <ExtraRepositoryRows
        fs={makeFs({ repositoryId: "repo-only" })}
        repositories={[makeRepo("repo-only", "only")]}
        isTaskStarted={false}
        primarySelected
      />,
    );
    const btn = screen.getByTestId("add-extra-repository") as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });
});
