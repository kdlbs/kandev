import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";

import type { Repository, RepositorySet } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";

const REPO_WEB = "repo-web";
const REPO_GATEWAY = "repo-gateway";
const FULL_STACK = "Full-stack";
const BACKEND = "Backend";
const NAME_INPUT = "repository-set-editor-name";
const ADD_REPOSITORY = "repository-set-add-repository";
const SAVE = "repository-set-editor-save";
const SET_ROW = "repository-set-row";
const CREATE = "repository-set-create";
const DELETE = "repository-set-delete-set-1";
const DELETE_SECOND = "repository-set-delete-set-2";
const DELETE_CONFIRM_POPOVER = "repository-set-delete-confirm-popover";
const DELETE_CONFIRM = "repository-set-delete-confirm";
const DELETE_INLINE = "repository-set-delete-inline-confirmation";
const DELETE_ERROR = "repository-set-delete-error";

const mockCreate = vi.fn();
const mockUpdate = vi.fn();
const mockDelete = vi.fn();
const mockUpsert = vi.fn();
const mockRemove = vi.fn();
let mockSets: RepositorySet[] = [];
let finePointer = true;

vi.mock("@/lib/api", () => ({
  createRepositorySet: (...args: unknown[]) => mockCreate(...args),
  updateRepositorySet: (...args: unknown[]) => mockUpdate(...args),
  deleteRepositorySet: (...args: unknown[]) => mockDelete(...args),
}));

vi.mock("@/hooks/domains/workspace/use-repository-sets", () => ({
  useRepositorySets: () => ({ sets: mockSets, isLoading: false, refresh: vi.fn() }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: finePointer, isMobile: !finePointer }),
}));

vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: () => ({
    branches: [
      { name: "main", type: "local", remote: "" },
      { name: "develop", type: "local", remote: "" },
    ],
    isLoaded: true,
    isLoading: false,
    refresh: vi.fn(),
  }),
}));

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (state: any) => unknown) =>
    selector({ upsertRepositorySet: mockUpsert, removeRepositorySet: mockRemove }),
}));

import { WorkspaceRepositorySetsSection } from "./workspace-repository-sets-section";

function repository(id: string): Repository {
  return { id: repositoryId(id), name: id } as unknown as Repository;
}

const AVAILABLE = [repository(REPO_WEB), repository(REPO_GATEWAY)];

function repositorySet(
  id: string,
  name: string,
  ids: string[],
  bases: string[] = [],
): RepositorySet {
  return {
    id,
    workspace_id: workspaceId("ws-1"),
    name,
    description: "",
    repositories: ids.map((member, position) => ({
      repository_id: repositoryId(member),
      position,
      ...(bases[position] ? { base_branch: bases[position] } : {}),
    })),
    created_at: "2026-08-17T09:00:00Z",
    updated_at: "2026-08-17T09:00:00Z",
  };
}

function renderSection(options: { readOnly?: boolean } = {}) {
  render(
    <TooltipProvider>
      <WorkspaceRepositorySetsSection
        workspaceId="ws-1"
        repositories={AVAILABLE}
        readOnly={options.readOnly ?? false}
      />
    </TooltipProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  finePointer = true;
  mockSets = [repositorySet("set-1", FULL_STACK, [REPO_WEB, REPO_GATEWAY])];
  mockCreate.mockResolvedValue(repositorySet("set-2", "Backend", [REPO_GATEWAY]));
  mockUpdate.mockResolvedValue(repositorySet("set-1", "Renamed", [REPO_WEB]));
  mockDelete.mockResolvedValue(undefined);
});

afterEach(() => cleanup());

describe("WorkspaceRepositorySetsSection", () => {
  it("lists each set with its member repositories", () => {
    renderSection();

    const rows = screen.getAllByTestId(SET_ROW);
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain(FULL_STACK);
    expect(rows[0].textContent).toContain(REPO_WEB);
    expect(rows[0].textContent).toContain(REPO_GATEWAY);
  });

  it("shows a saved base branch in the set summary", () => {
    mockSets = [repositorySet("set-1", FULL_STACK, [REPO_WEB], ["develop"])];
    renderSection();

    expect(screen.getByTestId(`repository-set-member-base-${REPO_WEB}`).textContent).toContain(
      "develop",
    );
  });

  it("says so when the workspace has no sets yet", () => {
    mockSets = [];
    renderSection();

    expect(screen.queryByTestId("repository-sets-empty")).not.toBeNull();
    expect(screen.queryAllByTestId(SET_ROW)).toHaveLength(0);
  });

  it("creates a set from the editor", async () => {
    renderSection();

    fireEvent.click(screen.getByTestId(CREATE));
    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: "Backend" } });
    fireEvent.click(screen.getByTestId(ADD_REPOSITORY));
    fireEvent.click(screen.getByRole("option", { name: REPO_GATEWAY }));
    fireEvent.click(screen.getByTestId(SAVE));

    await waitFor(() => expect(mockCreate).toHaveBeenCalledTimes(1));
    expect(mockCreate).toHaveBeenCalledWith("ws-1", {
      name: "Backend",
      description: "",
      repositories: [{ repositoryId: REPO_GATEWAY, baseBranch: "" }],
    });
    expect(mockUpsert).toHaveBeenCalledWith(
      "ws-1",
      repositorySet("set-2", "Backend", [REPO_GATEWAY]),
    );
  });

  it("keeps save unavailable until the editor has a name and a member", () => {
    renderSection();
    fireEvent.click(screen.getByTestId(CREATE));

    expect((screen.getByTestId(SAVE) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: "Backend" } });
    // A named set with no member is still not saveable: the API requires one.
    expect((screen.getByTestId(SAVE) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(screen.getByTestId(ADD_REPOSITORY));
    fireEvent.click(screen.getByRole("option", { name: REPO_GATEWAY }));
    expect((screen.getByTestId(SAVE) as HTMLButtonElement).disabled).toBe(false);
  });
});

describe("WorkspaceRepositorySetsSection member editing", () => {
  it("edits an existing set, replacing its whole membership", async () => {
    renderSection();

    fireEvent.click(screen.getByTestId("repository-set-edit-set-1"));
    // Deselect one member, leaving repo-web.
    fireEvent.click(screen.getByTestId(`repository-set-remove-${REPO_GATEWAY}`));
    fireEvent.click(screen.getByTestId(SAVE));

    await waitFor(() => expect(mockUpdate).toHaveBeenCalledTimes(1));
    expect(mockUpdate).toHaveBeenCalledWith("set-1", {
      name: FULL_STACK,
      description: "",
      repositories: [{ repositoryId: REPO_WEB, baseBranch: "" }],
    });
  });

  it("saves a selected base branch and can reset it to the task default", async () => {
    renderSection();

    fireEvent.click(screen.getByTestId(CREATE));
    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: BACKEND } });
    fireEvent.click(screen.getByTestId(ADD_REPOSITORY));
    fireEvent.click(screen.getByRole("option", { name: REPO_GATEWAY }));
    fireEvent.click(screen.getByTestId(`repository-set-base-${REPO_GATEWAY}`));
    const develop = screen
      .getAllByRole("option")
      .find((option) => option.textContent?.includes("develop"));
    expect(develop).toBeDefined();
    fireEvent.click(develop!);

    expect((screen.getByTestId("repository-set-reset-bases") as HTMLButtonElement).disabled).toBe(
      false,
    );
    fireEvent.click(screen.getByTestId("repository-set-reset-bases"));
    expect((screen.getByTestId("repository-set-reset-bases") as HTMLButtonElement).disabled).toBe(
      true,
    );

    fireEvent.click(screen.getByTestId(`repository-set-base-${REPO_GATEWAY}`));
    const developAgain = screen
      .getAllByRole("option")
      .find((option) => option.textContent?.includes("develop"));
    expect(developAgain).toBeDefined();
    fireEvent.click(developAgain!);
    fireEvent.click(screen.getByTestId(SAVE));

    await waitFor(() => expect(mockCreate).toHaveBeenCalledTimes(1));
    expect(mockCreate).toHaveBeenCalledWith("ws-1", {
      name: BACKEND,
      description: "",
      repositories: [{ repositoryId: REPO_GATEWAY, baseBranch: "develop" }],
    });
  });

  it("reports a duplicate name inline and keeps the editor open", async () => {
    const { ApiError } = await import("@/lib/api/client");
    mockCreate.mockRejectedValue(
      new ApiError("conflict", 409, { error: 'repository set name already used: "Full-stack"' }),
    );
    renderSection();

    fireEvent.click(screen.getByTestId(CREATE));
    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: FULL_STACK } });
    fireEvent.click(screen.getByTestId(ADD_REPOSITORY));
    fireEvent.click(screen.getByRole("option", { name: REPO_WEB }));
    fireEvent.click(screen.getByTestId(SAVE));

    await waitFor(() => expect(screen.queryByTestId("repository-set-editor-error")).not.toBeNull());
    expect(screen.queryByTestId(NAME_INPUT)).not.toBeNull();
    expect(mockUpsert).not.toHaveBeenCalled();
  });
});

describe("WorkspaceRepositorySetsSection deletion confirmation", () => {
  it("deletes a set after confirmation and drops it from the store", async () => {
    renderSection();

    fireEvent.click(screen.getByTestId(DELETE));
    const popover = screen.getByTestId(DELETE_CONFIRM_POPOVER);
    expect(screen.queryByRole("alertdialog")).toBeNull();
    fireEvent.click(within(popover).getByTestId(DELETE_CONFIRM));

    expect(screen.queryByTestId(DELETE_CONFIRM_POPOVER)).toBeNull();
    await waitFor(() => expect(mockDelete).toHaveBeenCalledWith("set-1"));
    expect(mockRemove).toHaveBeenCalledWith("ws-1", "set-1");
  });

  it("states that deleting a set leaves its repositories alone", () => {
    renderSection();

    fireEvent.click(screen.getByTestId(DELETE));

    const popover = screen.getByTestId(DELETE_CONFIRM_POPOVER);
    expect(popover.textContent).toContain("repositories");
    expect(screen.queryByTestId("repository-set-delete-dialog")).toBeNull();
  });

  it("cancels locally and returns focus to the delete control", async () => {
    renderSection();

    const trigger = screen.getByTestId(DELETE);
    fireEvent.click(trigger);
    fireEvent.click(
      within(screen.getByTestId(DELETE_CONFIRM_POPOVER)).getByRole("button", {
        name: "Cancel",
      }),
    );

    expect(mockDelete).not.toHaveBeenCalled();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it("keeps the row and shows local failure feedback when deletion fails", async () => {
    const { ApiError } = await import("@/lib/api/client");
    mockDelete.mockRejectedValue(new ApiError("server error", 500, null));
    renderSection();

    fireEvent.click(screen.getByTestId(DELETE));
    fireEvent.click(within(screen.getByTestId(DELETE_CONFIRM_POPOVER)).getByTestId(DELETE_CONFIRM));

    await waitFor(() => expect(screen.queryByTestId(DELETE_ERROR)).not.toBeNull());
    expect(
      screen.getByTestId(SET_ROW).querySelector(`[data-testid="${DELETE_ERROR}"]`),
    ).not.toBeNull();
    expect(mockRemove).not.toHaveBeenCalled();
  });

  it("morphs the delete action into touch-sized inline confirmation on coarse pointers", async () => {
    finePointer = false;
    renderSection();

    fireEvent.click(screen.getByTestId(DELETE));

    const inline = screen.getByTestId(DELETE_INLINE);
    expect(screen.queryByTestId(DELETE_CONFIRM_POPOVER)).toBeNull();
    expect(inline.textContent).toContain("repositories");
    expect(within(inline).getByTestId(DELETE_CONFIRM).className).toContain("h-11");
    expect(within(inline).getByTestId(DELETE_CONFIRM).className).toContain("min-w-11");

    fireEvent.click(within(inline).getByRole("button", { name: "Cancel" }));
    expect(mockDelete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId(DELETE));
    fireEvent.click(within(screen.getByTestId(DELETE_INLINE)).getByTestId(DELETE_CONFIRM));
    await waitFor(() => expect(mockRemove).toHaveBeenCalledWith("ws-1", "set-1"));
  });

  it("clears a previous deletion error before confirming a different set", async () => {
    const first = repositorySet("set-1", FULL_STACK, [REPO_WEB]);
    const second = repositorySet("set-2", BACKEND, [REPO_GATEWAY]);
    mockSets = [first, second];
    mockDelete.mockRejectedValueOnce(new Error("server error"));
    renderSection();

    fireEvent.click(screen.getByTestId(DELETE));
    fireEvent.click(within(screen.getByTestId(DELETE_CONFIRM_POPOVER)).getByTestId(DELETE_CONFIRM));
    await waitFor(() => expect(screen.queryByTestId(DELETE_ERROR)).not.toBeNull());

    fireEvent.click(screen.getByTestId(DELETE_SECOND));

    expect(screen.queryByTestId(DELETE_ERROR)).toBeNull();
  });
});

describe("WorkspaceRepositorySetsSection read-only behavior", () => {
  it("offers no create, edit, or delete control when read-only", () => {
    renderSection({ readOnly: true });

    expect(screen.queryByTestId(CREATE)).toBeNull();
    expect(screen.queryByTestId("repository-set-edit-set-1")).toBeNull();
    expect(screen.queryByTestId(DELETE)).toBeNull();
    // The list itself still renders, so the sets remain visible.
    expect(screen.queryAllByTestId(SET_ROW)).toHaveLength(1);
  });

  it("shows a set whose repositories were all deleted as empty rather than hiding it", () => {
    mockSets = [repositorySet("set-empty", "Orphaned", [])];
    renderSection();

    const rows = screen.getAllByTestId(SET_ROW);
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain("Orphaned");
    expect(rows[0].getAttribute("data-member-count")).toBe("0");
  });
});
