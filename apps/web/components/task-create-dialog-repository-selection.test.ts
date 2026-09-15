import { describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import {
  repositorySelectionReducer,
  useRepositorySelectionState,
  type RepositorySelectionState,
  type RepositorySelectionAction,
} from "./task-create-dialog-repositories-state";
import type { TaskRepositorySelection } from "./task-create-dialog-types";

const LOCAL_SELECTION_KEY = "local-1";
const LOCAL_REPOSITORY_ID = "repo-local";
const REMOTE_SELECTION_KEY = "remote-1";
const REMOTE_REPOSITORY_URL = "https://git.example/remote";

const local = (key: string, repositoryId: string): TaskRepositorySelection => ({
  kind: "local",
  key,
  repositoryId,
  branch: "main",
});

const remote = (key: string, url: string): TaskRepositorySelection => ({
  kind: "remote",
  key,
  url,
  branch: "develop",
  source: "paste",
});

function reduce(
  state: RepositorySelectionState,
  ...actions: RepositorySelectionAction[]
): RepositorySelectionState {
  return actions.reduce(repositorySelectionReducer, state);
}

describe("repositorySelectionReducer", () => {
  it("keeps local and remote selections in their insertion order", () => {
    const state = reduce(
      { selections: [], touched: false, dirty: false },
      { type: "append", selection: local(LOCAL_SELECTION_KEY, LOCAL_REPOSITORY_ID) },
      { type: "append", selection: remote(REMOTE_SELECTION_KEY, REMOTE_REPOSITORY_URL) },
      { type: "append", selection: local("local-2", "repo-second") },
    );

    expect(state.selections).toEqual([
      local(LOCAL_SELECTION_KEY, LOCAL_REPOSITORY_ID),
      remote(REMOTE_SELECTION_KEY, REMOTE_REPOSITORY_URL),
      local("local-2", "repo-second"),
    ]);
    expect(state.touched).toBe(true);
    expect(state.dirty).toBe(true);
  });

  it("removes only the requested selection and preserves the remaining order", () => {
    const initial = {
      selections: [
        local(LOCAL_SELECTION_KEY, LOCAL_REPOSITORY_ID),
        remote(REMOTE_SELECTION_KEY, REMOTE_REPOSITORY_URL),
      ],
      touched: true,
      dirty: true,
    };

    expect(
      repositorySelectionReducer(initial, { type: "remove", key: LOCAL_SELECTION_KEY }).selections,
    ).toEqual([remote(REMOTE_SELECTION_KEY, REMOTE_REPOSITORY_URL)]);
  });

  it("does not hydrate over a selection the user has touched", () => {
    const initial = {
      selections: [local(LOCAL_SELECTION_KEY, LOCAL_REPOSITORY_ID)],
      touched: true,
      dirty: true,
    };
    const next = repositorySelectionReducer(initial, {
      type: "hydrate",
      selections: [remote(REMOTE_SELECTION_KEY, REMOTE_REPOSITORY_URL)],
    });

    expect(next).toBe(initial);
  });

  it("hydrates local rows without marking an untouched draft as edited", () => {
    const initial = { selections: [], touched: false, dirty: false };
    const next = repositorySelectionReducer(initial, {
      type: "hydrate-local",
      rows: [{ key: "row-0", repositoryId: LOCAL_REPOSITORY_ID, branch: "" }],
    });

    expect(next).toEqual({
      selections: [{ kind: "local", key: "row-0", repositoryId: LOCAL_REPOSITORY_ID, branch: "" }],
      touched: false,
      dirty: false,
    });
  });

  it("does not hydrate local rows after an explicit edit", () => {
    const initial = {
      selections: [],
      touched: true,
      dirty: true,
    };
    const next = repositorySelectionReducer(initial, {
      type: "hydrate-local",
      rows: [{ key: "row-0", repositoryId: LOCAL_REPOSITORY_ID, branch: "" }],
    });

    expect(next).toBe(initial);
  });

  it("force-resets the draft for a new dialog open", () => {
    const next = repositorySelectionReducer(
      {
        selections: [local(LOCAL_SELECTION_KEY, LOCAL_REPOSITORY_ID)],
        touched: true,
        dirty: true,
      },
      {
        type: "reset",
        selections: [remote(REMOTE_SELECTION_KEY, REMOTE_REPOSITORY_URL)],
      },
    );

    expect(next).toEqual({
      selections: [remote(REMOTE_SELECTION_KEY, REMOTE_REPOSITORY_URL)],
      touched: false,
      dirty: false,
    });
  });
});

describe("repositorySelectionReducer row reconciliation", () => {
  it("updates rows by key without reordering interleaved selections", () => {
    const initial = {
      selections: [
        local("local-1", "repo-first"),
        remote(REMOTE_SELECTION_KEY, REMOTE_REPOSITORY_URL),
        local("local-2", "repo-second"),
      ],
      touched: true,
      dirty: true,
    };

    const next = repositorySelectionReducer(initial, {
      type: "set-local",
      rows: [{ key: "local-2", repositoryId: "repo-second-updated", branch: "release" }],
    });

    expect(next.selections).toEqual([
      remote(REMOTE_SELECTION_KEY, REMOTE_REPOSITORY_URL),
      {
        kind: "local",
        key: "local-2",
        repositoryId: "repo-second-updated",
        branch: "release",
      },
    ]);
  });
});

describe("useRepositorySelectionState", () => {
  it("replaces the seeded empty row when the picker adds the first repository", () => {
    const { result } = renderHook(() => useRepositorySelectionState());

    act(() => {
      result.current.setRepositories([{ key: "row-0", branch: "" }]);
    });

    act(() => {
      result.current.appendRepositorySelection({
        kind: "remote",
        url: "https://github.com/acme/remote",
        branch: "main",
        source: "picker",
      });
    });

    expect(result.current.repositorySelections).toEqual([
      expect.objectContaining({
        kind: "remote",
        url: "https://github.com/acme/remote",
        branch: "main",
      }),
    ]);
  });
});
