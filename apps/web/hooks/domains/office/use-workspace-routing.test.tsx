import { describe, expect, it, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useWorkspaceRouting } from "./use-workspace-routing";

const mocks = vi.hoisted(() => ({
  getWorkspaceRouting: vi.fn(),
  retryProvider: vi.fn(),
  updateWorkspaceRouting: vi.fn(),
  updateSeatAuthorExclusion: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-extended-api", () => ({
  getWorkspaceRouting: mocks.getWorkspaceRouting,
  retryProvider: mocks.retryProvider,
  updateWorkspaceRouting: mocks.updateWorkspaceRouting,
  updateSeatAuthorExclusion: mocks.updateSeatAuthorExclusion,
}));

const setKnownProviders = vi.fn();
const setWorkspaceRouting = vi.fn();
let routingByWorkspace: Record<string, unknown> = {};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (state: unknown) => unknown) =>
    sel({
      office: {
        routing: { byWorkspace: routingByWorkspace, knownProviders: [] },
      },
      setKnownProviders,
      setWorkspaceRouting,
    }),
}));

describe("useWorkspaceRouting", () => {
  beforeEach(() => {
    setKnownProviders.mockReset();
    setWorkspaceRouting.mockReset();
    routingByWorkspace = {};
    mocks.getWorkspaceRouting.mockReset();
    mocks.getWorkspaceRouting.mockResolvedValue({
      config: {
        enabled: false,
        provider_order: [],
        default_tier: "balanced",
        provider_profiles: {},
      },
      known_providers: ["claude-acp"],
    });
  });

  it("fetches once on mount when there is no cached config", async () => {
    const { unmount } = renderHook(() => useWorkspaceRouting("ws-1"));
    await waitFor(() => expect(mocks.getWorkspaceRouting).toHaveBeenCalledTimes(1));
    expect(setKnownProviders).toHaveBeenCalled();
    expect(setWorkspaceRouting).toHaveBeenCalled();
    unmount();
  });

  it("does not call setInterval (no polling)", () => {
    const spy = vi.spyOn(globalThis, "setInterval");
    const { unmount } = renderHook(() => useWorkspaceRouting("ws-1"));
    expect(spy).not.toHaveBeenCalled();
    unmount();
    spy.mockRestore();
  });

  it("fetches execution profiles even when routing config is cached", async () => {
    routingByWorkspace = { "ws-1": { enabled: false } };
    renderHook(() => useWorkspaceRouting("ws-1"));
    await waitFor(() => expect(mocks.getWorkspaceRouting).toHaveBeenCalledWith("ws-1"));
  });

  it("discards an in-flight response after the workspace changes", async () => {
    let resolveFirst!: (value: unknown) => void;
    let resolveSecond!: (value: unknown) => void;
    mocks.getWorkspaceRouting
      .mockReturnValueOnce(new Promise((resolve) => (resolveFirst = resolve)))
      .mockReturnValueOnce(new Promise((resolve) => (resolveSecond = resolve)));
    const { result, rerender } = renderHook(
      ({ workspace }: { workspace: string }) => useWorkspaceRouting(workspace),
      { initialProps: { workspace: "ws-1" } },
    );
    rerender({ workspace: "ws-2" });
    resolveFirst({ execution_profiles: [{ id: "stale" }] });
    resolveSecond({ execution_profiles: [{ id: "current" }] });

    await waitFor(() => expect(result.current.executionProfiles).toEqual([{ id: "current" }]));
  });

  it("exposes author exclusion warnings from load and from save", async () => {
    const loaded = [
      { code: "routing_disabled", workflow_name: "W", step_name: "Review", role: "reviewer" },
    ];
    const saved = [
      {
        code: "single_provider",
        workflow_name: "W",
        step_name: "Review",
        role: "reviewer",
        provider_id: "claude-acp",
      },
    ];
    mocks.getWorkspaceRouting.mockResolvedValue({
      config: {
        enabled: false,
        provider_order: [],
        default_tier: "balanced",
        provider_profiles: {},
      },
      known_providers: [],
      author_exclusion_warnings: loaded,
    });
    mocks.updateWorkspaceRouting.mockResolvedValue({ ok: true, author_exclusion_warnings: saved });
    const { result } = renderHook(() => useWorkspaceRouting("ws-1"));
    await waitFor(() => expect(result.current.authorExclusionWarnings).toEqual(loaded));

    let returned: unknown;
    await act(async () => {
      returned = await result.current.update({
        enabled: true,
        provider_order: ["claude-acp"],
        default_tier: "balanced",
        provider_profiles: {},
      });
    });
    expect(returned).toEqual(saved);
    expect(result.current.authorExclusionWarnings).toEqual(saved);
  });
});

describe("useWorkspaceRouting seat steps", () => {
  beforeEach(() => {
    routingByWorkspace = {};
    mocks.getWorkspaceRouting.mockReset();
  });

  it("exposes seat steps and saves switch changes", async () => {
    const step = {
      workflow_id: "wf-1",
      workflow_name: "Office Default",
      step_id: "step-review",
      step_name: "Review",
      role: "reviewer",
      enabled: false,
    };
    const warnings = [
      {
        code: "single_provider",
        workflow_name: "Office Default",
        step_name: "Review",
        role: "reviewer",
        provider_id: "codex-acp",
      },
    ];
    mocks.getWorkspaceRouting.mockResolvedValue({
      config: {
        enabled: true,
        provider_order: [],
        default_tier: "frontier",
        provider_profiles: {},
      },
      known_providers: [],
      author_exclusion_steps: [step],
    });
    mocks.updateSeatAuthorExclusion.mockResolvedValue({
      author_exclusion_steps: [{ ...step, enabled: true }],
      author_exclusion_warnings: warnings,
    });
    const { result } = renderHook(() => useWorkspaceRouting("ws-1"));
    await waitFor(() => expect(result.current.authorExclusionSteps).toEqual([step]));

    let returned: unknown;
    await act(async () => {
      returned = await result.current.updateAuthorExclusion([
        { step_id: "step-review", role: "reviewer", enabled: true },
      ]);
    });
    expect(mocks.updateSeatAuthorExclusion).toHaveBeenCalledWith("ws-1", [
      { step_id: "step-review", role: "reviewer", enabled: true },
    ]);
    expect(returned).toEqual(warnings);
    expect(result.current.authorExclusionSteps).toEqual([{ ...step, enabled: true }]);
    expect(result.current.authorExclusionWarnings).toEqual(warnings);
  });
});
