import { act, renderHook } from "@testing-library/react";
import { IconInbox } from "@tabler/icons-react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PresetOption } from "./search-bar";
import { useSavedPresetActions } from "./use-saved-preset-actions";
import { useSavedPresets, type SavedPreset } from "./use-saved-presets";

vi.mock("./use-saved-presets", () => ({
  useSavedPresets: vi.fn(),
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

type Options = Parameters<typeof useSavedPresetActions>[0];

function renderActions(overrides: Partial<Options> = {}) {
  const setProgrammaticSelection = vi.fn();
  const setQueryImmediate = vi.fn();
  const setRepoFilter = vi.fn();
  const options: Options = {
    workspaceId: "workspace-1",
    selection: { kind: "issue", source: "saved", id: savedPreset.id },
    customQuery: QUERY,
    resolvedPrPresets: [prPreset],
    resolvedIssuePresets: [issuePreset],
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
    ...overrides,
  };

  return {
    ...renderHook(() => useSavedPresetActions(options)),
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
  };
}

describe("useSavedPresetActions", () => {
  beforeEach(() => {
    vi.mocked(useSavedPresets).mockReset();
  });

  it("saves the current query, commits it, selects it, and applies its repository", () => {
    const save = vi.fn(() => savedPreset);
    const remove = vi.fn();
    vi.mocked(useSavedPresets).mockReturnValue({ presets: [savedPreset], save, remove });
    const { result, setProgrammaticSelection, setQueryImmediate, setRepoFilter } = renderActions({
      selection: { kind: "issue", source: "preset", id: "assigned" },
    });

    act(() => result.current.onConfirmSave("Assigned in Kandev", REPO));

    expect(useSavedPresets).toHaveBeenCalledWith("workspace-1");
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
    vi.mocked(useSavedPresets).mockReturnValue({ presets: [], save, remove: vi.fn() });
    const { result, setProgrammaticSelection, setQueryImmediate, setRepoFilter } = renderActions();

    act(() => result.current.onConfirmSave("Unavailable", REPO));

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

  it("deletes the active saved query and selects the first same-kind preset", () => {
    const remove = vi.fn();
    vi.mocked(useSavedPresets).mockReturnValue({ presets: [savedPreset], save: vi.fn(), remove });
    const { result, setProgrammaticSelection, setQueryImmediate, setRepoFilter } = renderActions();

    act(() => result.current.onDeleteSaved(savedPreset.id));

    expect(remove).toHaveBeenCalledWith(savedPreset.id);
    expect(setProgrammaticSelection).toHaveBeenCalledWith({
      kind: "issue",
      source: "preset",
      id: issuePreset.value,
    });
    expect(setQueryImmediate).toHaveBeenCalledWith(issuePreset.filter);
    expect(setRepoFilter).toHaveBeenCalledWith("");
    expect(remove.mock.invocationCallOrder[0]).toBeLessThan(
      setProgrammaticSelection.mock.invocationCallOrder[0],
    );
  });

  it("deletes an inactive saved query without changing the active search", () => {
    const remove = vi.fn();
    vi.mocked(useSavedPresets).mockReturnValue({ presets: [savedPreset], save: vi.fn(), remove });
    const { result, setProgrammaticSelection, setQueryImmediate, setRepoFilter } = renderActions({
      selection: { kind: "issue", source: "saved", id: "saved-active" },
    });

    act(() => result.current.onDeleteSaved(savedPreset.id));

    expect(remove).toHaveBeenCalledWith(savedPreset.id);
    expect(setProgrammaticSelection).not.toHaveBeenCalled();
    expect(setQueryImmediate).not.toHaveBeenCalled();
    expect(setRepoFilter).not.toHaveBeenCalled();
  });
});
