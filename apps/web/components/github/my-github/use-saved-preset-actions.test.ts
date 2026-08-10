import { act, renderHook } from "@testing-library/react";
import { IconInbox } from "@tabler/icons-react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PresetOption } from "./search-bar";
import { useSavedPresetActions } from "./use-saved-preset-actions";
import { useSavedPresets, type SavedPreset } from "./use-saved-presets";

const mockToast = vi.fn();

vi.mock("./use-saved-presets", () => ({
  useSavedPresets: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

const QUERY = "assignee:@me is:open";
const REPO = "kdlbs/kandev";
const FIRST_WORKSPACE_ID = "workspace-1";
const SECOND_WORKSPACE_ID = "workspace-2";

const savedPreset: SavedPreset = {
  id: "saved-1",
  kind: "issue",
  label: "Assigned in Kandev",
  customQuery: QUERY,
  repoFilter: REPO,
  createdAt: "2026-07-17T00:00:00Z",
  isDefault: false,
};

const prPreset: PresetOption = {
  value: "review_requested",
  label: "Review requested",
  filter: "review-requested:@me is:open",
  group: "inbox",
  icon: IconInbox,
};

const issuePreset: PresetOption = {
  value: "assigned",
  label: "Assigned",
  filter: QUERY,
  group: "inbox",
  icon: IconInbox,
};

type SavedPresetStore = ReturnType<typeof useSavedPresets>;
type Options = Parameters<typeof useSavedPresetActions>[0];

function makeStore(overrides: Partial<SavedPresetStore> = {}): SavedPresetStore {
  return {
    presets: [],
    save: vi.fn(async () => null),
    remove: vi.fn(async () => false),
    setDefault: vi.fn(async () => false),
    ...overrides,
  } as SavedPresetStore;
}

function renderActions(
  overrides: Partial<Options> = {},
  savedPresetStore = makeStore({ presets: [savedPreset] }),
) {
  const setProgrammaticSelection = vi.fn();
  const setQueryImmediate = vi.fn();
  const setRepoFilter = vi.fn();
  const markSearchInteracted = vi.fn();
  const options: Options = {
    workspaceId: FIRST_WORKSPACE_ID,
    selection: { kind: "issue", source: "saved", id: savedPreset.id },
    customQuery: QUERY,
    resolvedPrPresets: [prPreset],
    resolvedIssuePresets: [issuePreset],
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
    savedPresetStore,
    markSearchInteracted,
    ...overrides,
  };
  const rendered = renderHook((currentOptions: Options) => useSavedPresetActions(currentOptions), {
    initialProps: options,
  });
  return {
    ...rendered,
    rerender: (nextOverrides: Partial<Options> = {}) =>
      rendered.rerender({ ...options, ...nextOverrides }),
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
    markSearchInteracted,
  };
}

async function expectLatestPresetCatalogAfterDelete() {
  let finishRemove!: (removed: boolean) => void;
  const remove = vi.fn(
    () =>
      new Promise<boolean>((resolve) => {
        finishRemove = resolve;
      }),
  );
  const refreshedIssuePreset: PresetOption = {
    ...issuePreset,
    value: "mentioned",
    label: "Mentioned",
    filter: "mentions:@me is:open",
  };
  const { result, rerender, setProgrammaticSelection, setQueryImmediate } = renderActions(
    {},
    makeStore({ presets: [savedPreset], remove }),
  );

  let deletion!: Promise<void>;
  act(() => {
    deletion = result.current.onDeleteSaved(savedPreset.id);
  });
  rerender({ resolvedIssuePresets: [refreshedIssuePreset] });

  await act(async () => {
    finishRemove(true);
    await deletion;
  });

  expect(setProgrammaticSelection).toHaveBeenCalledWith({
    kind: "issue",
    source: "preset",
    id: refreshedIssuePreset.value,
  });
  expect(setQueryImmediate).toHaveBeenCalledWith(refreshedIssuePreset.filter);
}

async function expectStaleSaveOutcomeIgnored() {
  let finishSave!: () => void;
  const save = vi.fn(
    () =>
      new Promise<SavedPreset>((resolve) => {
        finishSave = () => resolve(savedPreset);
      }),
  );
  const {
    result,
    rerender,
    markSearchInteracted,
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
  } = renderActions({}, makeStore({ save }));

  let mutation!: Promise<void>;
  act(() => {
    mutation = result.current.onConfirmSave(savedPreset.label, savedPreset.repoFilter);
  });
  rerender({ workspaceId: SECOND_WORKSPACE_ID });
  await act(async () => {
    finishSave();
    await mutation;
  });

  expect(markSearchInteracted).not.toHaveBeenCalled();
  expect(setProgrammaticSelection).not.toHaveBeenCalled();
  expect(setQueryImmediate).not.toHaveBeenCalled();
  expect(setRepoFilter).not.toHaveBeenCalled();
}

async function expectStaleDeleteOutcomeIgnored() {
  let finishRemove!: () => void;
  const remove = vi.fn(
    () =>
      new Promise<boolean>((resolve) => {
        finishRemove = () => resolve(true);
      }),
  );
  const {
    result,
    rerender,
    markSearchInteracted,
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
  } = renderActions({}, makeStore({ remove }));

  let mutation!: Promise<void>;
  act(() => {
    mutation = result.current.onDeleteSaved(savedPreset.id);
  });
  rerender({ workspaceId: SECOND_WORKSPACE_ID });
  await act(async () => {
    finishRemove();
    await mutation;
  });

  expect(markSearchInteracted).not.toHaveBeenCalled();
  expect(setProgrammaticSelection).not.toHaveBeenCalled();
  expect(setQueryImmediate).not.toHaveBeenCalled();
  expect(setRepoFilter).not.toHaveBeenCalled();
}

async function expectStaleDefaultOutcomeIgnored() {
  let finishDefault!: () => void;
  const setDefault = vi.fn(
    () =>
      new Promise<boolean>((resolve) => {
        finishDefault = () => resolve(true);
      }),
  );
  const { result, rerender, markSearchInteracted } = renderActions({}, makeStore({ setDefault }));

  let mutation!: Promise<void>;
  act(() => {
    mutation = result.current.onToggleSavedDefault(savedPreset);
  });
  rerender({ workspaceId: SECOND_WORKSPACE_ID });
  await act(async () => {
    finishDefault();
    await mutation;
  });

  expect(markSearchInteracted).not.toHaveBeenCalled();
}

beforeEach(() => {
  vi.mocked(useSavedPresets).mockReset();
  mockToast.mockReset();
});

describe("useSavedPresetActions save actions", () => {
  it("ignores a completed save after changing workspace", expectStaleSaveOutcomeIgnored);

  it("saves the current query, commits it, selects it, and applies its repository", async () => {
    const save = vi.fn(async () => savedPreset);
    const store = makeStore({ presets: [savedPreset], save });
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions({ selection: { kind: "issue", source: "preset", id: "assigned" } }, store);

    await act(async () => result.current.onConfirmSave("Assigned in Kandev", REPO));

    expect(useSavedPresets).not.toHaveBeenCalled();
    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(save).toHaveBeenCalledWith({
      kind: "issue",
      label: "Assigned in Kandev",
      customQuery: QUERY,
      repoFilter: REPO,
    });
    expect(save.mock.invocationCallOrder[0]).toBeLessThan(
      markSearchInteracted.mock.invocationCallOrder[0],
    );
    expect(setProgrammaticSelection).toHaveBeenCalledWith({
      kind: "issue",
      source: "saved",
      id: savedPreset.id,
    });
    expect(setRepoFilter).toHaveBeenCalledWith(REPO);
    expect(setQueryImmediate).toHaveBeenCalledWith(QUERY);
    expect(result.current.savedPresets).toEqual([savedPreset]);
  });

  it("leaves selection and repository unchanged when saving returns null", async () => {
    const save = vi.fn(async () => null);
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions({}, makeStore({ save }));

    await act(async () => result.current.onConfirmSave("Unavailable", REPO));

    expect(markSearchInteracted).not.toHaveBeenCalled();
    expect(save).toHaveBeenCalledWith({
      kind: "issue",
      label: "Unavailable",
      customQuery: QUERY,
      repoFilter: REPO,
    });
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();
  });

  it("reports a save persistence failure without selecting the optimistic preset", async () => {
    const failure = Promise.reject(new Error("settings down"));
    void failure.catch(() => undefined);
    const save = vi.fn(() => failure);
    const { result, setProgrammaticSelection, markSearchInteracted } = renderActions(
      {},
      makeStore({ save }),
    );

    await act(async () => result.current.onConfirmSave("Unavailable", REPO));

    expect(mockToast).toHaveBeenCalledWith({
      description: "Failed to save saved query",
      variant: "error",
    });
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(markSearchInteracted).not.toHaveBeenCalled();
  });
});

describe("useSavedPresetActions delete actions", () => {
  it("ignores a completed delete after changing workspace", expectStaleDeleteOutcomeIgnored);

  it("deletes the active saved query and selects the first same-kind preset", async () => {
    const remove = vi.fn(async () => true);
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions({}, makeStore({ presets: [savedPreset], remove }));

    await act(async () => result.current.onDeleteSaved(savedPreset.id));

    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(remove).toHaveBeenCalledWith(savedPreset.id);
    expect(remove.mock.invocationCallOrder[0]).toBeLessThan(
      markSearchInteracted.mock.invocationCallOrder[0],
    );
    expect(remove.mock.invocationCallOrder[0]).toBeLessThan(
      setProgrammaticSelection.mock.invocationCallOrder[0],
    );
    expect(setProgrammaticSelection).toHaveBeenCalledWith({
      kind: "issue",
      source: "preset",
      id: issuePreset.value,
    });
    expect(setQueryImmediate).toHaveBeenCalledWith(issuePreset.filter);
    expect(setRepoFilter).toHaveBeenCalledWith("");
  });

  it("uses the latest preset catalog when an active deletion finishes", async () => {
    await expectLatestPresetCatalogAfterDelete();
  });

  it("deletes an inactive saved query without changing the active search", async () => {
    const remove = vi.fn(async () => true);
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions(
      { selection: { kind: "issue", source: "saved", id: "saved-active" } },
      makeStore({ presets: [savedPreset], remove }),
    );

    await act(async () => result.current.onDeleteSaved(savedPreset.id));

    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(remove).toHaveBeenCalledWith(savedPreset.id);
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();
  });

  it("does not mark search interaction when the saved query no longer exists", async () => {
    const remove = vi.fn(async () => false);
    const { result, markSearchInteracted } = renderActions({}, makeStore({ remove }));

    await act(async () => result.current.onDeleteSaved(savedPreset.id));

    expect(markSearchInteracted).not.toHaveBeenCalled();
  });

  it("reports a delete persistence failure", async () => {
    const failure = Promise.reject(new Error("settings down"));
    void failure.catch(() => undefined);
    const remove = vi.fn(() => failure);
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions({}, makeStore({ remove }));

    await act(async () => result.current.onDeleteSaved(savedPreset.id));

    expect(mockToast).toHaveBeenCalledWith({
      description: "Failed to delete saved query",
      variant: "error",
    });
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();
    expect(markSearchInteracted).not.toHaveBeenCalled();
  });
});

describe("useSavedPresetActions default deletion", () => {
  it("deletes an inactive default without changing the active search", async () => {
    const inactiveDefault = { ...savedPreset, isDefault: true };
    const remove = vi.fn(async () => true);
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions(
      { selection: { kind: "issue", source: "saved", id: "saved-active" } },
      makeStore({ presets: [inactiveDefault], remove }),
    );

    await act(async () => result.current.onDeleteSaved(inactiveDefault.id));

    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(remove).toHaveBeenCalledWith(inactiveDefault.id);
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();
  });
});

describe("useSavedPresetActions default actions", () => {
  it(
    "ignores a completed default toggle after changing workspace",
    expectStaleDefaultOutcomeIgnored,
  );

  it("keeps action handlers stable across unchanged renders", () => {
    const { result, rerender } = renderActions();
    const initialConfirm = result.current.onConfirmSave;
    const initialDelete = result.current.onDeleteSaved;
    const initialToggle = result.current.onToggleSavedDefault;

    rerender();

    expect(result.current.onConfirmSave).toBe(initialConfirm);
    expect(result.current.onDeleteSaved).toBe(initialDelete);
    expect(result.current.onToggleSavedDefault).toBe(initialToggle);
  });

  it("does not mark search interaction when the default mutation is unavailable", async () => {
    const { result, markSearchInteracted } = renderActions();

    await act(async () => {
      await result.current.onToggleSavedDefault(savedPreset);
    });

    expect(markSearchInteracted).not.toHaveBeenCalled();
  });

  it("ignores a second default toggle while the first is pending", async () => {
    let finish!: () => void;
    const setDefault = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finish = () => resolve(true);
        }),
    );
    const { result } = renderActions({}, makeStore({ presets: [savedPreset], setDefault }));
    const toggle = result.current.onToggleSavedDefault;

    let firstMutation!: Promise<void>;
    let secondMutation!: Promise<void>;
    act(() => {
      firstMutation = toggle(savedPreset);
      secondMutation = toggle(savedPreset);
    });

    expect(setDefault).toHaveBeenCalledOnce();

    await act(async () => {
      finish();
      await Promise.all([firstMutation, secondMutation]);
    });
  });

  it("sets a future default without changing the current search and exposes pending state", async () => {
    let finish!: () => void;
    const setDefault = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finish = () => resolve(true);
        }),
    );
    const store = makeStore({ presets: [savedPreset], setDefault });
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions({}, store);

    let mutation!: Promise<void>;
    act(() => {
      mutation = result.current.onToggleSavedDefault(savedPreset);
    });

    expect(markSearchInteracted).not.toHaveBeenCalled();
    expect(setDefault).toHaveBeenCalledWith("issue", savedPreset.id);
    expect(result.current.defaultMutationPending).toBe(true);
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();

    await act(async () => {
      finish();
      await mutation;
    });
    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(result.current.defaultMutationPending).toBe(false);
  });

  it("clears an existing default and reports persistence failure", async () => {
    const currentDefault = { ...savedPreset, isDefault: true };
    const setDefault = vi.fn(async () => {
      throw new Error("settings down");
    });
    const { result, markSearchInteracted } = renderActions(
      {},
      makeStore({ presets: [currentDefault], setDefault }),
    );

    await act(async () => {
      await result.current.onToggleSavedDefault(currentDefault);
    });

    expect(setDefault).toHaveBeenCalledWith("issue", null);
    expect(mockToast).toHaveBeenCalledWith({
      description: "Failed to update default view",
      variant: "error",
    });
    expect(markSearchInteracted).not.toHaveBeenCalled();
    expect(result.current.defaultMutationPending).toBe(false);
  });
});
