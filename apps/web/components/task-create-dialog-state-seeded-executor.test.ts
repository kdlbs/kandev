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
  // The submit flow decides whether the user "changed" the runner by
  // comparing the final selection to whatever the dialog itself seeded, so
  // only a write that actually originated from autopick/stored-profile
  // seeding may become that baseline — never a value the user picked.
  it("is null until a value is seeded", () => {
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    expect(result.current.seededExecutorProfileId).toBeNull();
  });

  it("captures a seed write and keeps it despite later user changes", () => {
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));

    act(() => {
      result.current.setExecutorProfileIdFromSeed(SEEDED_AUTOPICKED_PROFILE);
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

  // Regression: a user fast enough to pick a runner before the seed effect
  // fires (e.g. an edit-mode stored profile that arrives a tick after open)
  // must not have that pick mistaken for the seed. Racing the user's
  // setExecutorProfileId ahead of setExecutorProfileIdFromSeed must not let
  // the user's own choice masquerade as "what the dialog put there".
  it("does not treat a user pick that arrives before the seed write as the seed", () => {
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));

    act(() => {
      result.current.setExecutorProfileId("profile-user-picked-first");
    });
    expect(result.current.seededExecutorProfileId).toBeNull();

    act(() => {
      result.current.setExecutorProfileIdFromSeed(SEEDED_AUTOPICKED_PROFILE);
    });
    expect(result.current.seededExecutorProfileId).toBe(SEEDED_AUTOPICKED_PROFILE);
  });

  it("resets to null on the next open cycle", () => {
    const { result, rerender } = renderHook(
      ({ open }: { open: boolean }) => useDialogFormState(open, "ws-1", null),
      { initialProps: { open: true } },
    );

    act(() => {
      result.current.setExecutorProfileIdFromSeed("profile-first-cycle");
    });
    expect(result.current.seededExecutorProfileId).toBe("profile-first-cycle");

    // Close then reopen: a rising edge bumps openCycle and the reset effects
    // clear executorProfileId back to "".
    rerender({ open: false });
    rerender({ open: true });

    expect(result.current.seededExecutorProfileId).toBeNull();
  });
});
