import { act, renderHook, waitFor } from "@testing-library/react";
import { IconInbox } from "@tabler/icons-react";
import { describe, expect, it, vi } from "vitest";
import type { PresetOption } from "./search-bar";
import type { SidebarSelection } from "./presets-sidebar";
import type { SavedPreset } from "./saved-preset-model";
import { useInitialSidebarSelection, useSidebarSelectionHandler } from "./use-sidebar-selection";

const prPreset: PresetOption = {
  value: "review-requested",
  label: "Review requested",
  filter: "review-requested:@me is:open",
  group: "inbox",
  icon: IconInbox,
};

const issuePreset: PresetOption = {
  value: "assigned",
  label: "Assigned",
  filter: "assignee:@me is:open",
  group: "inbox",
  icon: IconInbox,
};

const prPresets = [prPreset];
const issuePresets = [issuePreset];
const WORKSPACE_ID = "workspace-1";
const KIND_SWITCH = "kind-switch" as const;

const prDefault: SavedPreset = {
  id: "saved-pr",
  kind: "pr",
  label: "Kandev PRs",
  customQuery: "author:@me is:open",
  repoFilter: "kdlbs/kandev",
  createdAt: "2026-08-07T00:00:00Z",
  isDefault: true,
};

const issueDefault: SavedPreset = {
  ...prDefault,
  id: "saved-issue",
  kind: "issue",
  label: "Kandev issues",
  customQuery: "assignee:@me label:bug",
};

function createAutoResetControls() {
  let enabled = true;
  return {
    shouldAutoResetSearch: () => enabled,
    resetSearchOnWorkspaceChange: vi.fn(() => {
      enabled = true;
    }),
    disable: () => {
      enabled = false;
    },
  };
}

async function expectNewWorkspaceDefaultAfterManualSelection() {
  const setQueryImmediate = vi.fn();
  const setRepoFilter = vi.fn();
  const autoReset = createAutoResetControls();
  const { result, rerender } = renderHook(
    ({ workspaceId, savedPresets }) =>
      useInitialSidebarSelection({
        workspaceId,
        resolvedPrPresets: prPresets,
        shouldAutoResetSearch: autoReset.shouldAutoResetSearch,
        resetSearchOnWorkspaceChange: autoReset.resetSearchOnWorkspaceChange,
        setQueryImmediate,
        setRepoFilter,
        savedPresets,
      }),
    {
      initialProps: {
        workspaceId: WORKSPACE_ID,
        savedPresets: [] as SavedPreset[],
      },
    },
  );
  const manual: SidebarSelection = { kind: "pr", source: "preset", id: "manual" };
  act(() => {
    result.current.setUserSelection(manual);
    autoReset.disable();
  });
  autoReset.resetSearchOnWorkspaceChange.mockClear();

  rerender({ workspaceId: "workspace-2", savedPresets: [prDefault] });

  await waitFor(() => expect(result.current.selection.id).toBe(prDefault.id));
  expect(setQueryImmediate).toHaveBeenLastCalledWith(prDefault.customQuery);
  expect(setRepoFilter).toHaveBeenLastCalledWith(prDefault.repoFilter);
  expect(autoReset.resetSearchOnWorkspaceChange).toHaveBeenCalledOnce();
}

function expectConfiguredPresetFallbackOnKindSwitch() {
  const setUserSelection = vi.fn();
  const setQueryImmediate = vi.fn();
  const setRepoFilter = vi.fn();
  const { result } = renderHook(() =>
    useSidebarSelectionHandler({
      currentKind: "pr",
      savedPresets: [],
      resolvedPrPresets: prPresets,
      resolvedIssuePresets: issuePresets,
      setQueryImmediate,
      setRepoFilter,
      setUserSelection,
      markSearchInteracted: vi.fn(),
    }),
  );

  act(() => result.current({ kind: "issue", source: KIND_SWITCH }));

  expect(setUserSelection).toHaveBeenCalledWith({
    kind: "issue",
    source: "preset",
    id: issuePreset.value,
  });
  expect(setQueryImmediate).toHaveBeenCalledWith(issuePreset.filter);
  expect(setRepoFilter).toHaveBeenCalledWith("");
}

function expectSameKindSentinelNoop() {
  const setUserSelection = vi.fn();
  const setQueryImmediate = vi.fn();
  const setRepoFilter = vi.fn();
  const markSearchInteracted = vi.fn();
  const { result } = renderHook(() =>
    useSidebarSelectionHandler({
      currentKind: "pr",
      savedPresets: [],
      resolvedPrPresets: prPresets,
      resolvedIssuePresets: issuePresets,
      setQueryImmediate,
      setRepoFilter,
      setUserSelection,
      markSearchInteracted,
    }),
  );

  act(() => result.current({ kind: "pr", source: KIND_SWITCH }));

  expect(markSearchInteracted).not.toHaveBeenCalled();
  expect(setUserSelection).not.toHaveBeenCalled();
  expect(setQueryImmediate).not.toHaveBeenCalled();
  expect(setRepoFilter).not.toHaveBeenCalled();
}

function expectStableHandlerWithRefreshedPresetCatalogs() {
  const setUserSelection = vi.fn();
  const setQueryImmediate = vi.fn();
  const setRepoFilter = vi.fn();
  const markSearchInteracted = vi.fn();
  const { result, rerender } = renderHook(
    ({ savedPresets, currentPrPresets, currentIssuePresets }) =>
      useSidebarSelectionHandler({
        currentKind: "pr",
        savedPresets,
        resolvedPrPresets: currentPrPresets,
        resolvedIssuePresets: currentIssuePresets,
        setQueryImmediate,
        setRepoFilter,
        setUserSelection,
        markSearchInteracted,
      }),
    {
      initialProps: {
        savedPresets: [prDefault],
        currentPrPresets: prPresets,
        currentIssuePresets: issuePresets,
      },
    },
  );
  const initialHandler = result.current;

  rerender({
    savedPresets: [prDefault, issueDefault],
    currentPrPresets: [...prPresets],
    currentIssuePresets: [...issuePresets],
  });

  expect(result.current).toBe(initialHandler);
  act(() => initialHandler({ kind: "issue", source: KIND_SWITCH }));
  expect(setUserSelection).toHaveBeenCalledWith({
    kind: "issue",
    source: "saved",
    id: issueDefault.id,
  });
  expect(setQueryImmediate).toHaveBeenCalledWith(issueDefault.customQuery);
  expect(setRepoFilter).toHaveBeenCalledWith(issueDefault.repoFilter);

  const refreshedIssuePreset = {
    ...issuePreset,
    value: "mentioned",
    filter: "mentions:@me is:open",
  };
  setUserSelection.mockClear();
  setQueryImmediate.mockClear();
  setRepoFilter.mockClear();
  rerender({
    savedPresets: [prDefault],
    currentPrPresets: [...prPresets],
    currentIssuePresets: [refreshedIssuePreset],
  });
  expect(result.current).toBe(initialHandler);
  act(() => initialHandler({ kind: "issue", source: KIND_SWITCH }));
  expect(setUserSelection).toHaveBeenCalledWith({
    kind: "issue",
    source: "preset",
    id: refreshedIssuePreset.value,
  });
  expect(setQueryImmediate).toHaveBeenCalledWith(refreshedIssuePreset.filter);
  expect(setRepoFilter).toHaveBeenCalledWith("");
}

describe("useInitialSidebarSelection", () => {
  it("does not reapply an unchanged default when preset references change", async () => {
    const setQueryImmediate = vi.fn();
    const setRepoFilter = vi.fn();
    const autoReset = createAutoResetControls();
    const { rerender } = renderHook(
      ({ savedPresets }) =>
        useInitialSidebarSelection({
          workspaceId: WORKSPACE_ID,
          resolvedPrPresets: prPresets,
          shouldAutoResetSearch: autoReset.shouldAutoResetSearch,
          resetSearchOnWorkspaceChange: autoReset.resetSearchOnWorkspaceChange,
          setQueryImmediate,
          setRepoFilter,
          savedPresets,
        }),
      { initialProps: { savedPresets: [prDefault] } },
    );
    await waitFor(() => expect(setQueryImmediate).toHaveBeenCalledOnce());

    rerender({ savedPresets: [{ ...prDefault }] });

    expect(setQueryImmediate).toHaveBeenCalledOnce();
    expect(setRepoFilter).toHaveBeenCalledOnce();
  });

  it("applies a saved PR default when saved queries hydrate", async () => {
    const setQueryImmediate = vi.fn();
    const setRepoFilter = vi.fn();
    const autoReset = createAutoResetControls();
    const { result, rerender } = renderHook(
      ({ savedPresets }) =>
        useInitialSidebarSelection({
          workspaceId: WORKSPACE_ID,
          resolvedPrPresets: prPresets,
          shouldAutoResetSearch: autoReset.shouldAutoResetSearch,
          resetSearchOnWorkspaceChange: autoReset.resetSearchOnWorkspaceChange,
          setQueryImmediate,
          setRepoFilter,
          savedPresets,
        }),
      { initialProps: { savedPresets: [] as SavedPreset[] } },
    );
    await waitFor(() => expect(result.current.selection.id).toBe(prPreset.value));

    rerender({ savedPresets: [prDefault] });

    await waitFor(() =>
      expect(result.current.selection).toEqual({
        kind: "pr",
        source: "saved",
        id: prDefault.id,
      }),
    );
    expect(setQueryImmediate).toHaveBeenLastCalledWith(prDefault.customQuery);
    expect(setRepoFilter).toHaveBeenLastCalledWith(prDefault.repoFilter);
  });

  it("does not apply a late default after manual selection", async () => {
    const setQueryImmediate = vi.fn();
    const setRepoFilter = vi.fn();
    const autoReset = createAutoResetControls();
    const { result, rerender } = renderHook(
      ({ savedPresets }) =>
        useInitialSidebarSelection({
          workspaceId: WORKSPACE_ID,
          resolvedPrPresets: prPresets,
          shouldAutoResetSearch: autoReset.shouldAutoResetSearch,
          resetSearchOnWorkspaceChange: autoReset.resetSearchOnWorkspaceChange,
          setQueryImmediate,
          setRepoFilter,
          savedPresets,
        }),
      { initialProps: { savedPresets: [] as SavedPreset[] } },
    );
    const manual: SidebarSelection = { kind: "pr", source: "preset", id: "manual" };
    act(() => result.current.setUserSelection(manual));
    setQueryImmediate.mockClear();
    setRepoFilter.mockClear();

    rerender({ savedPresets: [prDefault] });

    expect(result.current.selection).toEqual(manual);
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();
  });

  it("resets manual selection before applying a new workspace default", async () => {
    await expectNewWorkspaceDefaultAfterManualSelection();
  });
});

describe("useSidebarSelectionHandler", () => {
  it("applies the destination kind's saved default on a kind switch", () => {
    const setUserSelection = vi.fn();
    const setQueryImmediate = vi.fn();
    const setRepoFilter = vi.fn();
    const markSearchInteracted = vi.fn();
    const { result } = renderHook(() =>
      useSidebarSelectionHandler({
        currentKind: "pr",
        savedPresets: [prDefault, issueDefault],
        resolvedPrPresets: prPresets,
        resolvedIssuePresets: issuePresets,
        setQueryImmediate,
        setRepoFilter,
        setUserSelection,
        markSearchInteracted,
      }),
    );

    act(() => result.current({ kind: "issue", source: KIND_SWITCH }));

    expect(markSearchInteracted).toHaveBeenCalledOnce();
    expect(setUserSelection).toHaveBeenCalledWith({
      kind: "issue",
      source: "saved",
      id: issueDefault.id,
    });
    expect(setQueryImmediate).toHaveBeenCalledWith(issueDefault.customQuery);
    expect(setRepoFilter).toHaveBeenCalledWith(issueDefault.repoFilter);
  });

  it(
    "falls back to the first configured preset when the destination has no saved default",
    expectConfiguredPresetFallbackOnKindSwitch,
  );

  it("ignores a same-kind toggle sentinel", expectSameKindSentinelNoop);

  it(
    "keeps the handler stable while using refreshed preset catalogs",
    expectStableHandlerWithRefreshedPresetCatalogs,
  );

  it("keeps an explicit selection within the current kind", () => {
    const setUserSelection = vi.fn();
    const setQueryImmediate = vi.fn();
    const setRepoFilter = vi.fn();
    const requested: SidebarSelection = {
      kind: "pr",
      source: "preset",
      id: prPreset.value,
    };
    const { result } = renderHook(() =>
      useSidebarSelectionHandler({
        currentKind: "pr",
        savedPresets: [prDefault],
        resolvedPrPresets: prPresets,
        resolvedIssuePresets: issuePresets,
        setQueryImmediate,
        setRepoFilter,
        setUserSelection,
        markSearchInteracted: vi.fn(),
      }),
    );

    act(() => result.current(requested));

    expect(setUserSelection).toHaveBeenCalledWith(requested);
    expect(setQueryImmediate).toHaveBeenCalledWith(prPreset.filter);
    expect(setRepoFilter).toHaveBeenCalledWith("");
  });
});
