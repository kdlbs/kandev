import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import type { WorkspaceRouting } from "@/lib/state/slices/office/types";
import type {
  AuthorExclusionStepChange,
  AuthorExclusionWarning,
} from "@/lib/types/author-exclusion";
import { useRoutingSave, type RoutingSaveArgs } from "./use-routing-save";

vi.mock("@/lib/toast/sonner", () => ({
  toast: { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
}));

const SAVED: WorkspaceRouting = {
  enabled: false,
  provider_order: [],
  default_tier: "balanced",
  provider_profiles: {},
  tier_per_reason: {},
};
const CHANGE: AuthorExclusionStepChange = { step_id: "s1", role: "reviewer", enabled: false };

function args(overrides: Partial<RoutingSaveArgs> = {}): RoutingSaveArgs {
  return {
    draft: SAVED,
    saved: SAVED,
    exclusionChanges: [],
    currentWarnings: [],
    setDraft: vi.fn(),
    resetExclusion: vi.fn(),
    updateRouting: vi.fn(async () => [] as AuthorExclusionWarning[]),
    updateAuthorExclusion: vi.fn(async () => [] as AuthorExclusionWarning[]),
    onSaved: vi.fn(),
    ...overrides,
  };
}

function renderSave(initial: RoutingSaveArgs) {
  return renderHook(
    (props: RoutingSaveArgs) => ({
      save: useRoutingSave(props),
      coordinator: useSettingsSaveCoordinator(),
    }),
    { initialProps: initial, wrapper: SettingsSaveProvider },
  );
}

afterEach(cleanup);

describe("useRoutingSave", () => {
  it("is not dirty without routing or switch changes", () => {
    const { result } = renderSave(args());
    expect(result.current.coordinator.hasDirty).toBe(false);
  });

  it("saves routing first, then the switches, through the shared save bar", async () => {
    const calls: string[] = [];
    const a = args({
      draft: { ...SAVED, enabled: true },
      exclusionChanges: [CHANGE],
      updateRouting: vi.fn(async () => {
        calls.push("routing");
        return [];
      }),
      updateAuthorExclusion: vi.fn(async () => {
        calls.push("switches");
        return [];
      }),
    });
    const { result } = renderSave(a);
    expect(result.current.coordinator.hasDirty).toBe(true);

    let outcome: Awaited<ReturnType<typeof result.current.coordinator.saveAll>> | undefined;
    await act(async () => {
      outcome = await result.current.coordinator.saveAll();
    });

    expect(calls).toEqual(["routing", "switches"]);
    expect(a.updateAuthorExclusion).toHaveBeenCalledWith([CHANGE]);
    expect(a.onSaved).toHaveBeenCalled();
    expect(outcome?.failedIds.size).toBe(0);
  });

  it("skips the routing write when only switches changed", async () => {
    const a = args({ exclusionChanges: [CHANGE] });
    const { result } = renderSave(a);
    await act(async () => {
      await result.current.coordinator.saveAll();
    });
    expect(a.updateRouting).not.toHaveBeenCalled();
    expect(a.updateAuthorExclusion).toHaveBeenCalledOnce();
  });

  it("reports a failed write to the save bar and keeps the error details", async () => {
    const a = args({
      exclusionChanges: [CHANGE],
      updateAuthorExclusion: vi.fn(async () => {
        throw new Error("boom");
      }),
    });
    const { result } = renderSave(a);
    let outcome: Awaited<ReturnType<typeof result.current.coordinator.saveAll>> | undefined;
    await act(async () => {
      outcome = await result.current.coordinator.saveAll();
    });
    expect(outcome?.failedIds.has("office-routing")).toBe(true);
    expect(result.current.coordinator.status).toBe("error");
    expect(result.current.save.saving).toBe(false);
  });

  it("discard restores the saved routing and clears the switch changes", async () => {
    const a = args({ draft: { ...SAVED, enabled: true }, exclusionChanges: [CHANGE] });
    const { result } = renderSave(a);
    const contributor = result.current.coordinator.contributorStates.find(
      (c) => c.id === "office-routing",
    );
    expect(contributor?.isDirty).toBe(true);
    await act(async () => {
      await result.current.save.discard();
    });
    expect(a.setDraft).toHaveBeenCalledWith(SAVED);
    expect(a.resetExclusion).toHaveBeenCalled();
  });
});
