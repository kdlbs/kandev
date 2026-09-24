"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
} from "react";
import type {
  TaskRemoteRepoRow,
  TaskRepoRow,
  TaskRepositorySelection,
  TaskWorkspaceFolderSelection,
} from "@/components/task-create-dialog-types";

export type RepositorySelectionState = {
  selections: TaskRepositorySelection[];
  /** True after the user changes the draft, so async defaults cannot replace it. */
  touched: boolean;
  /** True when the selection differs from the values loaded for the open form. */
  dirty: boolean;
};

export type NewRepositorySelection =
  | Omit<Extract<TaskRepositorySelection, { kind: "local" }>, "key">
  | Omit<Extract<TaskRepositorySelection, { kind: "remote" }>, "key">
  | Omit<TaskWorkspaceFolderSelection, "key">;

type SelectionUpdater<T> = T[] | ((rows: T[]) => T[]);

export type RepositorySelectionAction =
  | { type: "append"; selection: TaskRepositorySelection }
  | { type: "remove"; key: string }
  | { type: "replace"; key: string; selection: TaskRepositorySelection }
  | { type: "set-local"; rows: SelectionUpdater<TaskRepoRow> }
  | { type: "hydrate-local"; rows: SelectionUpdater<TaskRepoRow> }
  | { type: "set-remote"; rows: SelectionUpdater<TaskRemoteRepoRow> }
  | { type: "set-folders"; rows: SelectionUpdater<TaskWorkspaceFolderSelection> }
  | { type: "set-dirty"; dirty: boolean }
  | { type: "hydrate"; selections: TaskRepositorySelection[] }
  | { type: "reset"; selections: TaskRepositorySelection[] };

/** Resolves the canonical list while keeping legacy mode-shaped test doubles usable. */
export function resolveRepositorySelections(input: {
  repositorySelections?: TaskRepositorySelection[];
  repositories: TaskRepoRow[];
  remoteRepos: TaskRemoteRepoRow[];
  useRemote?: boolean;
}): TaskRepositorySelection[] {
  if (input.repositorySelections) return input.repositorySelections;
  if (input.useRemote) {
    return input.remoteRepos.map((row) => ({ kind: "remote" as const, ...row }));
  }
  return input.repositories.map((row) => ({ kind: "local" as const, ...row }));
}

export function repositorySelectionReducer(
  state: RepositorySelectionState,
  action: RepositorySelectionAction,
): RepositorySelectionState {
  if (action.type === "set-dirty") return { ...state, dirty: action.dirty };
  if (action.type === "hydrate") {
    return state.touched ? state : { selections: action.selections, touched: false, dirty: false };
  }
  if (action.type === "reset") {
    return { selections: action.selections, touched: false, dirty: false };
  }

  if (action.type === "append") {
    return markSelectionChanged({ ...state, selections: [...state.selections, action.selection] });
  }
  if (action.type === "remove") {
    return markSelectionChanged({
      ...state,
      selections: state.selections.filter((selection) => selection.key !== action.key),
    });
  }
  if (action.type === "replace") {
    return markSelectionChanged({
      ...state,
      selections: state.selections.map((selection) =>
        selection.key === action.key ? action.selection : selection,
      ),
    });
  }

  if (action.type === "set-local") {
    const current = state.selections.filter((selection) => selection.kind === "local");
    const rows = resolveSelectionUpdater(action.rows, current);
    return replaceKind(state, "local", rows);
  }

  if (action.type === "hydrate-local") {
    if (state.touched || state.selections.some((selection) => selection.kind === "remote")) {
      return state;
    }
    const current = state.selections.filter((selection) => selection.kind === "local");
    const rows = resolveSelectionUpdater(action.rows, current);
    return replaceKind(state, "local", rows, false);
  }

  if (action.type === "set-folders") {
    const current = state.selections.filter(isFolderSelection);
    const rows = resolveSelectionUpdater(action.rows, current);
    return replaceKind(state, "folder", rows);
  }

  const current = state.selections.filter((selection) => selection.kind === "remote");
  const rows = resolveSelectionUpdater(action.rows, current);
  return replaceKind(state, "remote", rows);
}

function resolveSelectionUpdater<T>(updater: SelectionUpdater<T>, current: T[]): T[] {
  return typeof updater === "function" ? updater(current) : updater;
}

function replaceKind(
  state: RepositorySelectionState,
  kind: TaskRepositorySelection["kind"],
  rows: Array<TaskRepoRow | TaskRemoteRepoRow | TaskWorkspaceFolderSelection>,
  markChanged = true,
): RepositorySelectionState {
  const rowsByKey = new Map(rows.map((row) => [row.key, row]));
  const existingKeys = new Set(
    state.selections
      .filter((selection) => selection.kind === kind)
      .map((selection) => selection.key),
  );
  const next = state.selections.flatMap((selection) => {
    if (selection.kind !== kind) return [selection];
    const row = rowsByKey.get(selection.key);
    return row ? [{ ...row, kind } as TaskRepositorySelection] : [];
  });
  for (const row of rows) {
    if (!existingKeys.has(row.key)) {
      next.push({ ...row, kind } as TaskRepositorySelection);
    }
  }
  const nextState = { ...state, selections: next };
  return markChanged ? markSelectionChanged(nextState) : nextState;
}

function markSelectionChanged(state: RepositorySelectionState): RepositorySelectionState {
  return { ...state, touched: true, dirty: true };
}

/**
 * Stores one ordered list while exposing the old local and remote projections
 * used by boundary adapters. New task creation should use `repositorySelections`.
 */
export function useRepositorySelectionState() {
  const [state, dispatch] = useReducer(repositorySelectionReducer, {
    selections: [],
    touched: false,
    dirty: false,
  });
  const nextKeyRef = useRef(0);
  const rowActions = useRepositorySelectionRowActions(state, dispatch, nextKeyRef);
  const stateActions = useRepositorySelectionStateActions(dispatch);
  const repositories = useMemo(
    () => state.selections.filter(isLocalSelection).map(stripSelectionKind),
    [state.selections],
  );
  const remoteRepos = useMemo(
    () => state.selections.filter(isRemoteSelection).map(stripSelectionKind),
    [state.selections],
  );
  const workspaceFolders = useMemo(
    () => state.selections.filter(isFolderSelection).map(stripSelectionKind),
    [state.selections],
  );

  return {
    repositorySelections: state.selections,
    repositorySelectionsTouched: state.touched,
    ...rowActions,
    repositories,
    repositoriesDirty: state.dirty,
    ...stateActions,
    remoteRepos,
    workspaceFolders,
  };
}

function useRepositorySelectionRowActions(
  state: RepositorySelectionState,
  dispatch: Dispatch<RepositorySelectionAction>,
  nextKeyRef: MutableRefObject<number>,
) {
  const allocateKey = useCallback(
    (prefix: "row" | "remote" | "folder") => {
      let key = "";
      const taken = new Set(state.selections.map((selection) => selection.key));
      do {
        nextKeyRef.current += 1;
        key = `${prefix}-${nextKeyRef.current}`;
      } while (taken.has(key));
      return key;
    },
    [state.selections, nextKeyRef],
  );
  const addRepository = useCallback(() => {
    dispatch({
      type: "append",
      selection: { kind: "local", key: allocateKey("row"), branch: "" },
    });
  }, [allocateKey, dispatch]);
  const appendRepositorySelection = useCallback(
    (selection: NewRepositorySelection) => {
      const emptyPlaceholder =
        state.selections.length === 1 && isEmptyLocalSelection(state.selections[0]);
      if (emptyPlaceholder) {
        const key = state.selections[0].key;
        dispatch({
          type: "replace",
          key,
          selection: { ...selection, key } as TaskRepositorySelection,
        });
        return key;
      }
      const key = allocateKey(selection.kind === "remote" ? "remote" : "row");
      dispatch({ type: "append", selection: { ...selection, key } as TaskRepositorySelection });
      return key;
    },
    [allocateKey, dispatch, state.selections],
  );
  const addRemoteRepo = useCallback(() => {
    dispatch({
      type: "append",
      selection: {
        kind: "remote",
        key: allocateKey("remote"),
        url: "",
        branch: "",
        source: "paste",
      },
    });
  }, [allocateKey, dispatch]);
  const appendFolderSelection = useCallback(
    (selection: Omit<TaskWorkspaceFolderSelection, "key">) => {
      const key = allocateKey("folder");
      dispatch({ type: "append", selection: { ...selection, key } });
      return key;
    },
    [allocateKey, dispatch],
  );
  const removeRepository = useCallback(
    (key: string) => dispatch({ type: "remove", key }),
    [dispatch],
  );
  const removeRemoteRepo = useCallback(
    (key: string) => dispatch({ type: "remove", key }),
    [dispatch],
  );
  const updateRepository = useCallback(
    (key: string, patch: Partial<TaskRepoRow>) => {
      const selection = state.selections.find((candidate) => candidate.key === key);
      if (!selection || selection.kind !== "local") return;
      dispatch({ type: "replace", key, selection: { ...selection, ...patch } });
    },
    [dispatch, state.selections],
  );
  const updateRemoteRepo = useUpdateRemoteRepo(state, dispatch);
  return {
    addRepository,
    appendRepositorySelection,
    addRemoteRepo,
    appendFolderSelection,
    removeRepository,
    removeRemoteRepo,
    updateRepository,
    updateRemoteRepo,
  };
}

function useUpdateRemoteRepo(
  state: RepositorySelectionState,
  dispatch: Dispatch<RepositorySelectionAction>,
) {
  return useCallback(
    (key: string, patch: Partial<TaskRemoteRepoRow>) => {
      const selection = state.selections.find((candidate) => candidate.key === key);
      if (!selection || selection.kind !== "remote") return;
      dispatch({
        type: "replace",
        key,
        selection: { ...applyRemoteRepoPatch(selection, patch), kind: "remote" },
      });
    },
    [dispatch, state.selections],
  );
}

function useRepositorySelectionStateActions(dispatch: Dispatch<RepositorySelectionAction>) {
  const setRepositories = useCallback(
    (rows: SelectionUpdater<TaskRepoRow>) => dispatch({ type: "set-local", rows }),
    [dispatch],
  );
  const hydrateRepositories = useCallback(
    (rows: SelectionUpdater<TaskRepoRow>) => dispatch({ type: "hydrate-local", rows }),
    [dispatch],
  );
  const setRemoteRepos = useCallback(
    (rows: SelectionUpdater<TaskRemoteRepoRow>) => dispatch({ type: "set-remote", rows }),
    [dispatch],
  );
  const setWorkspaceFolders = useCallback(
    (rows: SelectionUpdater<TaskWorkspaceFolderSelection>) =>
      dispatch({ type: "set-folders", rows }),
    [dispatch],
  );
  const setRepositoriesDirty = useCallback(
    (dirty: boolean) => dispatch({ type: "set-dirty", dirty }),
    [dispatch],
  );
  const resetRepositorySelections = useCallback(
    (selections: TaskRepositorySelection[]) => dispatch({ type: "reset", selections }),
    [dispatch],
  );
  const hydrateRepositorySelections = useCallback(
    (selections: TaskRepositorySelection[]) => dispatch({ type: "hydrate", selections }),
    [dispatch],
  );
  return {
    setRepositories,
    hydrateRepositories,
    setRemoteRepos,
    setWorkspaceFolders,
    setRepositoriesDirty,
    resetRepositorySelections,
    hydrateRepositorySelections,
  };
}

function isLocalSelection(
  selection: TaskRepositorySelection,
): selection is Extract<TaskRepositorySelection, { kind: "local" }> {
  return selection.kind === "local";
}

function isRemoteSelection(
  selection: TaskRepositorySelection,
): selection is Extract<TaskRepositorySelection, { kind: "remote" }> {
  return selection.kind === "remote";
}

function isFolderSelection(
  selection: TaskRepositorySelection,
): selection is Extract<TaskRepositorySelection, { kind: "folder" }> {
  return selection.kind === "folder";
}

function stripSelectionKind<T extends TaskRepositorySelection>(selection: T): Omit<T, "kind"> {
  const { kind: _kind, ...row } = selection;
  return row;
}

function isEmptyLocalSelection(
  selection: TaskRepositorySelection | undefined,
): selection is Extract<TaskRepositorySelection, { kind: "local" }> {
  return Boolean(
    selection?.kind === "local" &&
    !selection.repositoryId &&
    !selection.localPath &&
    !selection.branch &&
    !selection.baseBranch &&
    !selection.branchPolicyId,
  );
}

/**
 * Manages the unified `repositories` list for task creation. Every chip
 * (one or many) is an entry; there is no "primary" or "extras" split.
 *
 * `nextKey` increments to give each row a stable client-side key without
 * relying on array indices (which would shift on removal and break
 * uncontrolled inputs).
 */
export function useRepositoriesState() {
  const [repositories, setRepositories] = useState<TaskRepoRow[]>([]);
  const [repositoriesDirty, setRepositoriesDirty] = useState(false);
  const nextKeyRef = useRef(0);

  const addRepository = useCallback(() => {
    nextKeyRef.current += 1;
    const key = `row-${nextKeyRef.current}`;
    setRepositories((rows) => [...rows, { key, branch: "" }]);
    setRepositoriesDirty(true);
  }, []);

  const removeRepository = useCallback((key: string) => {
    setRepositories((rows) => rows.filter((r) => r.key !== key));
    setRepositoriesDirty(true);
  }, []);

  const updateRepository = useCallback((key: string, patch: Partial<TaskRepoRow>) => {
    setRepositories((rows) => rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));
    setRepositoriesDirty(true);
  }, []);

  return {
    repositories,
    repositoriesDirty,
    setRepositories,
    setRepositoriesDirty,
    addRepository,
    removeRepository,
    updateRepository,
  };
}

/**
 * Manages the unified `remoteRepos` list for task creation. Mirrors
 * `useRepositoriesState` — same key-generation pattern, same shape of
 * add/update/remove operations — but rows carry a remote URL + branch
 * instead of a workspace repoId / localPath. Used by the GitHub Remote
 * mode of the task-create dialog.
 */
export function useRemoteReposState() {
  const [remoteRepos, setRemoteRepos] = useState<TaskRemoteRepoRow[]>([]);
  const nextKeyRef = useRef(0);

  const newKey = useCallback(() => {
    nextKeyRef.current += 1;
    return `remote-${nextKeyRef.current}`;
  }, []);

  // Allocates a fresh key by bumping the local counter until it lands on
  // one not already used by the current rows. The plain `newKey()` helper
  // can't see the rows list, so a hydrated state — e.g. `setRemoteRepos`
  // injecting `{key: "remote-1", …}` from initialValues — would collide
  // with the next addRemoteRepo() (counter starts at 0, first call hands
  // out "remote-1"). Using the setter callback gives us the current rows
  // synchronously so we can skip taken keys.
  const addRemoteRepo = useCallback(() => {
    setRemoteRepos((rows) => {
      const taken = new Set(rows.map((r) => r.key));
      let key: string;
      do {
        nextKeyRef.current += 1;
        key = `remote-${nextKeyRef.current}`;
      } while (taken.has(key));
      return [...rows, { key, url: "", branch: "", source: "paste" }];
    });
  }, []);

  const removeRemoteRepo = useCallback((key: string) => {
    setRemoteRepos((rows) => rows.filter((r) => r.key !== key));
  }, []);

  const updateRemoteRepo = useCallback((key: string, patch: Partial<TaskRemoteRepoRow>) => {
    setRemoteRepos((rows) =>
      rows.map((row) => (row.key === key ? applyRemoteRepoPatch(row, patch) : row)),
    );
  }, []);

  return {
    remoteRepos,
    setRemoteRepos,
    addRemoteRepo,
    removeRemoteRepo,
    updateRemoteRepo,
    newRemoteRepoKey: newKey,
  };
}

export function applyRemoteRepoPatch(
  row: TaskRemoteRepoRow,
  patch: Partial<TaskRemoteRepoRow>,
): TaskRemoteRepoRow {
  if (typeof patch.url !== "string" || patch.url.trim() === row.url.trim()) {
    return { ...row, ...patch };
  }
  return {
    ...row,
    remoteUrl: undefined,
    checkoutOptions: undefined,
    provider: undefined,
    providerHost: undefined,
    providerScope: undefined,
    providerRepoId: undefined,
    providerOwner: undefined,
    providerName: undefined,
    fullName: undefined,
    prNumber: undefined,
    prBaseBranch: undefined,
    prHeadBranch: undefined,
    ...patch,
  };
}

/**
 * Mirrors `useRepositoryAutoSelectEffect` for the remote-repo list: when the
 * user flips Remote mode on and the list is empty, seed a single empty paste
 * row so the URL input has somewhere to land. The list is NOT cleared on
 * Remote → off (toggle-back is non-destructive).
 *
 * Shared between the create-task dialog and the New Subtask form so both
 * surfaces get the same auto-seed behavior on Remote toggle.
 */
export function useRemoteReposSeedEffect(
  useRemote: boolean,
  rows: TaskRemoteRepoRow[],
  setRemoteRepos: React.Dispatch<React.SetStateAction<TaskRemoteRepoRow[]>>,
) {
  useEffect(() => {
    if (!useRemote || rows.length > 0) return;
    setRemoteRepos([{ key: "remote-0", url: "", branch: "", source: "paste" }]);
  }, [useRemote, rows.length, setRemoteRepos]);
}
