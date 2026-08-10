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
type CurrentOptions = Parameters<typeof useSavedPresetActions>[0];
type FutureOptions = Omit<CurrentOptions, "workspaceId"> & {
  savedPresetStore: SavedPresetStore;
  markSearchInteracted: () => void;
};
type FutureActions = ReturnType<typeof useSavedPresetActions> & {
  onToggleSavedDefault?: (preset: SavedPreset) => Promise<void>;
  defaultMutationPending?: boolean;
};

function makeStore(overrides: Partial<SavedPresetStore> = {}): SavedPresetStore {
  return {
    presets: [],
    save: vi.fn(() => null),
    remove: vi.fn(),
    setDefault: vi.fn(async () => undefined),
    ...overrides,
  } as SavedPresetStore;
}

function renderActions(
  overrides: Partial<FutureOptions> = {},
  savedPresetStore = makeStore({ presets: [savedPreset] }),
) {
  const setProgrammaticSelection = vi.fn();
  const setQueryImmediate = vi.fn();
  const setRepoFilter = vi.fn();
  const markSearchInteracted = vi.fn();
  const options: FutureOptions = {
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
  vi.mocked(useSavedPresets).mockReturnValue(options.savedPresetStore);

  return {
    ...renderHook(() => useSavedPresetActions(options as unknown as CurrentOptions)),
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
    markSearchInteracted,
  };
}

function toggleDefault(current: ReturnType<typeof useSavedPresetActions>) {
  const action = (current as FutureActions).onToggleSavedDefault;
  expect(action, "onToggleSavedDefault action").toBeTypeOf("function");
  return action as NonNullable<FutureActions["onToggleSavedDefault"]>;
}

beforeEach(() => {
  vi.mocked(useSavedPresets).mockReset();
  mockToast.mockReset();
});

describe("useSavedPresetActions query actions", () => {
  it("saves the current query, commits it, selects it, and applies its repository", () => {
    const save = vi.fn(() => savedPreset);
    const store = makeStore({ presets: [savedPreset], save });
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions({ selection: { kind: "issue", source: "preset", id: "assigned" } }, store);

    act(() => result.current.onConfirmSave("Assigned in Kandev", REPO));

    expect(useSavedPresets).not.toHaveBeenCalled();
    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(save).toHaveBeenCalledWith({
      kind: "issue",
      label: "Assigned in Kandev",
      customQuery: QUERY,
      repoFilter: REPO,
    });
    expect(setProgrammaticSelection).toHaveBeenCalledWith({
      kind: "issue",
      source: "saved",
      id: savedPreset.id,
    });
    expect(setRepoFilter).toHaveBeenCalledWith(REPO);
    expect(setQueryImmediate).toHaveBeenCalledWith(QUERY);
    expect(result.current.savedPresets).toEqual([savedPreset]);
  });

  it("leaves selection and repository unchanged when saving returns null", () => {
    const save = vi.fn(() => null);
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions({}, makeStore({ save }));

    act(() => result.current.onConfirmSave("Unavailable", REPO));

    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();
  });

  it("deletes the active saved query and selects the first same-kind preset", () => {
    const remove = vi.fn();
    const {
      result,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      markSearchInteracted,
    } = renderActions({}, makeStore({ presets: [savedPreset], remove }));

    act(() => result.current.onDeleteSaved(savedPreset.id));

    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(remove).toHaveBeenCalledWith(savedPreset.id);
    expect(setProgrammaticSelection).toHaveBeenCalledWith({
      kind: "issue",
      source: "preset",
      id: issuePreset.value,
    });
    expect(setQueryImmediate).toHaveBeenCalledWith(issuePreset.filter);
    expect(setRepoFilter).toHaveBeenCalledWith("");
  });

  it("deletes an inactive saved query without changing the active search", () => {
    const remove = vi.fn();
    const { result, setProgrammaticSelection, setQueryImmediate, setRepoFilter } = renderActions(
      { selection: { kind: "issue", source: "saved", id: "saved-active" } },
      makeStore({ presets: [savedPreset], remove }),
    );

    act(() => result.current.onDeleteSaved(savedPreset.id));

    expect(remove).toHaveBeenCalledWith(savedPreset.id);
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();
  });
});

describe("useSavedPresetActions default actions", () => {
  it("ignores a second default toggle while the first is pending", async () => {
    let finish!: () => void;
    const setDefault = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    const { result } = renderActions({}, makeStore({ presets: [savedPreset], setDefault }));
    const toggle = toggleDefault(result.current);

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
        new Promise<void>((resolve) => {
          finish = resolve;
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
      mutation = toggleDefault(result.current)(savedPreset);
    });

    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(setDefault).toHaveBeenCalledWith("issue", savedPreset.id);
    expect((result.current as FutureActions).defaultMutationPending).toBe(true);
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();

    await act(async () => {
      finish();
      await mutation;
    });
    expect((result.current as FutureActions).defaultMutationPending).toBe(false);
  });

  it("clears an existing default and reports persistence failure", async () => {
    const currentDefault = { ...savedPreset, isDefault: true };
    const setDefault = vi.fn(async () => {
      throw new Error("settings down");
    });
    const { result } = renderActions({}, makeStore({ presets: [currentDefault], setDefault }));

    await act(async () => {
      await toggleDefault(result.current)(currentDefault);
    });

    expect(setDefault).toHaveBeenCalledWith("issue", null);
    expect(mockToast).toHaveBeenCalledWith({
      description: "Failed to update default view",
      variant: "error",
    });
    expect((result.current as FutureActions).defaultMutationPending).toBe(false);
  });
});
