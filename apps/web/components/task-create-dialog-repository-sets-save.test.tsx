import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

import type { RepositorySet } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";

const NAME_INPUT = "repository-set-name";
const SUBMIT = "repository-set-save-submit";
const REPO_WEB = "repo-web";
const REPO_GATEWAY = "repo-gateway";
const SET_NAME = "Full-stack";

const mockCreateRepositorySet = vi.fn();
const mockUpsertRepositorySet = vi.fn();

vi.mock("@/lib/api", () => ({
  createRepositorySet: (...args: unknown[]) => mockCreateRepositorySet(...args),
}));

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (state: any) => unknown) =>
    selector({ upsertRepositorySet: mockUpsertRepositorySet }),
}));

import { SaveRepositorySetDialog } from "@/components/task-create-dialog-repository-sets-save";

function createdSet(name: string): RepositorySet {
  return {
    id: "set-1",
    workspace_id: workspaceId("ws-1"),
    name,
    description: "",
    repositories: [{ repository_id: repositoryId(REPO_WEB), position: 0 }],
    created_at: "2026-08-17T09:00:00Z",
    updated_at: "2026-08-17T09:00:00Z",
  };
}

const ROWS: TaskRepoRow[] = [
  { key: "row-0", repositoryId: REPO_WEB, branch: "main" },
  { key: "row-1", repositoryId: REPO_GATEWAY, branch: "develop" },
];

function renderDialog(overrides: { rows?: TaskRepoRow[]; onOpenChange?: () => void } = {}) {
  const onOpenChange = overrides.onOpenChange ?? vi.fn();
  render(
    <SaveRepositorySetDialog
      open
      onOpenChange={onOpenChange}
      workspaceId="ws-1"
      rows={overrides.rows ?? ROWS}
    />,
  );
  return { onOpenChange };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockCreateRepositorySet.mockResolvedValue(createdSet(SET_NAME));
});

afterEach(() => cleanup());

describe("SaveRepositorySetDialog", () => {
  it("creates a set from the form's selected repositories in row order", async () => {
    renderDialog();

    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: SET_NAME } });
    fireEvent.click(screen.getByTestId(SUBMIT));

    await waitFor(() => expect(mockCreateRepositorySet).toHaveBeenCalledTimes(1));
    expect(mockCreateRepositorySet).toHaveBeenCalledWith("ws-1", {
      name: SET_NAME,
      description: "",
      repositories: [
        { repositoryId: REPO_WEB, baseBranch: "main" },
        { repositoryId: REPO_GATEWAY, baseBranch: "develop" },
      ],
    });
  });

  it("puts the created set into the store and closes", async () => {
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: SET_NAME } });
    fireEvent.click(screen.getByTestId(SUBMIT));

    await waitFor(() => expect(mockUpsertRepositorySet).toHaveBeenCalledTimes(1));
    expect(mockUpsertRepositorySet).toHaveBeenCalledWith("ws-1", createdSet(SET_NAME));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("keeps submit unavailable until a name is entered", () => {
    renderDialog();

    const submit = screen.getByTestId(SUBMIT) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);

    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: "  " } });
    expect((screen.getByTestId(SUBMIT) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: SET_NAME } });
    expect((screen.getByTestId(SUBMIT) as HTMLButtonElement).disabled).toBe(false);
  });

  it("reports a duplicate name inline and keeps the dialog open with the draft intact", async () => {
    mockCreateRepositorySet.mockRejectedValue(
      Object.assign(
        new Error('repository set name already used: "Full-stack" already uses this name'),
        {
          status: 409,
        },
      ),
    );
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: SET_NAME } });
    fireEvent.click(screen.getByTestId(SUBMIT));

    await waitFor(() => expect(screen.queryByTestId("repository-set-save-error")).not.toBeNull());
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect((screen.getByTestId(NAME_INPUT) as HTMLInputElement).value).toBe(SET_NAME);
    expect(mockUpsertRepositorySet).not.toHaveBeenCalled();
  });

  it("surfaces any other failure without losing the entered name", async () => {
    mockCreateRepositorySet.mockRejectedValue(new Error("offline"));
    renderDialog();

    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: SET_NAME } });
    fireEvent.click(screen.getByTestId(SUBMIT));

    await waitFor(() => expect(screen.queryByTestId("repository-set-save-error")).not.toBeNull());
    expect((screen.getByTestId(NAME_INPUT) as HTMLInputElement).value).toBe(SET_NAME);
  });

  it("says nothing can be saved when no workspace repository row is filled", () => {
    renderDialog({ rows: [{ key: "row-0", branch: "" }] });

    expect(screen.queryByTestId("repository-set-save-empty")).not.toBeNull();
    expect(screen.queryByTestId(SUBMIT)).toBeNull();
  });

  it("excludes a discovered local-path row, which is not a workspace repository", async () => {
    renderDialog({
      rows: [
        { key: "row-0", localPath: "/src/web", branch: "main" },
        { key: "row-1", repositoryId: REPO_GATEWAY, branch: "main" },
      ],
    });

    fireEvent.change(screen.getByTestId(NAME_INPUT), { target: { value: SET_NAME } });
    fireEvent.click(screen.getByTestId(SUBMIT));

    await waitFor(() => expect(mockCreateRepositorySet).toHaveBeenCalledTimes(1));
    expect(mockCreateRepositorySet.mock.calls[0][1].repositories).toEqual([
      { repositoryId: REPO_GATEWAY, baseBranch: "main" },
    ]);
    // The user is told which rows could not be included.
    expect(screen.queryByTestId("repository-set-save-excluded")).not.toBeNull();
  });
});
