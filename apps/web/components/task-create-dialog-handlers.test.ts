import { describe, it, expect, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useDialogHandlers } from "./task-create-dialog-handlers";
import type { DialogFormState } from "@/components/task-create-dialog-types";

/**
 * Minimal stand-in for `DialogFormState`. We only exercise the source-mode
 * toggles here, so most fields are populated with safe defaults and the rest
 * are filled in by an `as` cast. Mutating setters are wired to vi.fn() and
 * the simple state flags are recomputed on each set so a test can assert the
 * mutually-exclusive behavior end-to-end.
 */
type ToggleState = {
  useRemote: boolean;
  noRepository: boolean;
};

function makeFs(initial: Partial<ToggleState> = {}): DialogFormState {
  const state: ToggleState = {
    useRemote: initial.useRemote ?? false,
    noRepository: initial.noRepository ?? false,
  };
  const setUseRemote = vi.fn((v: boolean) => {
    state.useRemote = v;
  });
  const setNoRepository = vi.fn((v: boolean) => {
    state.noRepository = v;
  });
  const fs = {
    get useRemote() {
      return state.useRemote;
    },
    get noRepository() {
      return state.noRepository;
    },
    setUseRemote,
    setNoRepository,
    setGitHubUrlError: vi.fn(),
    setFreshBranchEnabled: vi.fn(),
    setCurrentLocalBranch: vi.fn(),
    setCurrentLocalBranchLoading: vi.fn(),
    setExecutorId: vi.fn(),
    setExecutorProfileId: vi.fn(),
    setWorkspacePath: vi.fn(),
    setAgentProfileId: vi.fn(),
    setTaskName: vi.fn(),
    setHasTitle: vi.fn(),
    setSelectedWorkflowId: vi.fn(),
    updateRepository: vi.fn(),
  } as unknown as DialogFormState;
  return fs;
}

describe("useDialogHandlers — Remote / no-repository mutual exclusion", () => {
  it("toggling Remote ON clears noRepository (mutually exclusive source modes)", () => {
    // Regression: handleToggleRemote used to flip useRemote without touching
    // noRepository, so the user could end up with both true at once (toggle
    // no-repo on, then toggle Remote on). The submit gate's mode-aware checks
    // then produced confusing results.
    const fs = makeFs({ useRemote: false, noRepository: true });
    const { result } = renderHook(() => useDialogHandlers(fs, []));

    act(() => {
      result.current.handleToggleRemote();
    });

    expect(fs.setUseRemote).toHaveBeenCalledWith(true);
    expect(fs.setNoRepository).toHaveBeenCalledWith(false);
  });

  it("toggling Remote OFF does NOT touch noRepository", () => {
    // When the user explicitly leaves Remote mode we shouldn't force noRepo
    // on or off — they may already be in some non-Remote workspace flow.
    const fs = makeFs({ useRemote: true, noRepository: false });
    const { result } = renderHook(() => useDialogHandlers(fs, []));

    act(() => {
      result.current.handleToggleRemote();
    });

    expect(fs.setUseRemote).toHaveBeenCalledWith(false);
    expect(fs.setNoRepository).not.toHaveBeenCalled();
  });

  it("toggling no-repository ON clears useRemote (mirror of the Remote handler)", () => {
    const fs = makeFs({ useRemote: true, noRepository: false });
    const { result } = renderHook(() => useDialogHandlers(fs, []));

    act(() => {
      result.current.handleToggleNoRepository();
    });

    expect(fs.setNoRepository).toHaveBeenCalledWith(true);
    expect(fs.setUseRemote).toHaveBeenCalledWith(false);
  });
});
