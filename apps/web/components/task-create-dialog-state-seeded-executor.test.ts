import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useDialogFormState } from "./task-create-dialog-state";

// `useBranchesByURL` triggers a real network ensure() when given a URL — stub
// it so the dialog state hook can mount in JSDOM without hitting fetch. The
// stubbed shape mirrors the production hook (branches/loading/ensure).
vi.mock("@/hooks/domains/github/use-branches-by-url", () => ({
  useBranchesByURL: () => ({
    branches: () => [],
    loading: () => false,
    ensure: () => undefined,
  }),
}));

vi.mock("@/hooks/domains/github/use-pr-info-by-url", async (importOriginal) => {
  const original =
    await importOriginal<typeof import("@/hooks/domains/github/use-pr-info-by-url")>();
  return {
    ...original,
    usePRInfoByURL: () => ({
      info: () => undefined,
      loading: () => false,
      ensure: () => undefined,
      clear: () => undefined,
    }),
  };
});

const SEEDED_AUTOPICKED_PROFILE = "profile-autopicked";

describe("useDialogFormState — seededExecutorProfileId", () => {
  // AC-TASKS-RUNNER-SWITCH-004.5b: the submit flow decides whether the user
  // "changed" the runner by comparing the final selection to whatever the
  // dialog itself seeded — not to a touched flag — so a value set by
  // autopick/stored-profile seeding must be captured once and held steady
  // even as the user experiments with other selections.
  it("is null until the first executorProfileId value is set", () => {
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    expect(result.current.seededExecutorProfileId).toBeNull();
  });

  it("captures the first non-empty executorProfileId and keeps it despite later user changes", () => {
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));

    act(() => {
      result.current.setExecutorProfileId(SEEDED_AUTOPICKED_PROFILE);
    });
    expect(result.current.seededExecutorProfileId).toBe(SEEDED_AUTOPICKED_PROFILE);

    act(() => {
      result.current.setExecutorProfileId("profile-user-chosen");
    });
    expect(result.current.seededExecutorProfileId).toBe(SEEDED_AUTOPICKED_PROFILE);

    // Changing back to the seeded value doesn't create a second "seed".
    act(() => {
      result.current.setExecutorProfileId(SEEDED_AUTOPICKED_PROFILE);
    });
    expect(result.current.seededExecutorProfileId).toBe(SEEDED_AUTOPICKED_PROFILE);
  });

  it("resets to null on the next open cycle", () => {
    const { result, rerender } = renderHook(
      ({ open }: { open: boolean }) => useDialogFormState(open, "ws-1", null),
      { initialProps: { open: true } },
    );

    act(() => {
      result.current.setExecutorProfileId("profile-first-cycle");
    });
    expect(result.current.seededExecutorProfileId).toBe("profile-first-cycle");

    // Close then reopen: a rising edge bumps openCycle and the reset effects
    // clear executorProfileId back to "".
    rerender({ open: false });
    rerender({ open: true });

    expect(result.current.seededExecutorProfileId).toBeNull();
  });
});
