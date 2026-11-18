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

const store = vi.hoisted(() => ({
  office: { routing: { byWorkspace: {}, knownProviders: [] } },
  setKnownProviders: () => undefined,
  setWorkspaceRouting: () => undefined,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (state: unknown) => unknown) => sel(store),
}));

const config = {
  enabled: true,
  provider_order: [],
  default_tier: "balanced",
  provider_profiles: {},
};
const stepFor = (ws: string) => ({
  workflow_id: `wf-${ws}`,
  workflow_name: `Workflow ${ws}`,
  step_id: `step-${ws}`,
  step_name: "Review",
  role: "reviewer",
  enabled: true,
});
const warningFor = (ws: string) => ({
  code: "single_provider",
  workflow_name: `Workflow ${ws}`,
  step_name: "Review",
  role: "reviewer",
  provider_id: "claude-acp",
});
const loaded = (ws: string) => ({
  config,
  known_providers: [],
  execution_profiles: [{ id: `profile-${ws}` }],
  author_exclusion_steps: [stepFor(ws)],
  author_exclusion_warnings: [warningFor(ws)],
});

function renderForWorkspace(initial: string) {
  return renderHook(({ workspace }: { workspace: string }) => useWorkspaceRouting(workspace), {
    initialProps: { workspace: initial },
  });
}

describe("useWorkspaceRouting after switching workspace", () => {
  beforeEach(() => {
    mocks.getWorkspaceRouting.mockReset();
    mocks.updateSeatAuthorExclusion.mockReset();
  });

  it("hides the previous workspace's switches while the new one loads or fails", async () => {
    mocks.getWorkspaceRouting
      .mockResolvedValueOnce(loaded("a"))
      .mockRejectedValueOnce(new Error("offline"));
    const { result, rerender } = renderForWorkspace("ws-a");
    await waitFor(() => expect(result.current.authorExclusionSteps).toEqual([stepFor("a")]));

    rerender({ workspace: "ws-b" });
    expect(result.current.authorExclusionSteps).toEqual([]);
    expect(result.current.authorExclusionWarnings).toEqual([]);
    expect(result.current.executionProfiles).toEqual([]);
    await waitFor(() => expect(result.current.error).toBe("offline"));
    expect(result.current.authorExclusionSteps).toEqual([]);
  });

  it("ignores a save response that arrives after the user left the workspace", async () => {
    let resolveSave!: (value: unknown) => void;
    mocks.getWorkspaceRouting.mockResolvedValueOnce(loaded("a")).mockResolvedValueOnce(loaded("b"));
    mocks.updateSeatAuthorExclusion.mockReturnValueOnce(
      new Promise((resolve) => (resolveSave = resolve)),
    );
    const { result, rerender } = renderForWorkspace("ws-a");
    await waitFor(() => expect(result.current.authorExclusionSteps).toEqual([stepFor("a")]));

    let save!: Promise<unknown>;
    act(() => {
      save = result.current.updateAuthorExclusion([
        { step_id: "step-a", role: "reviewer", enabled: false },
      ]);
    });
    rerender({ workspace: "ws-b" });
    await waitFor(() => expect(result.current.authorExclusionSteps).toEqual([stepFor("b")]));

    await act(async () => {
      resolveSave({
        author_exclusion_steps: [{ ...stepFor("a"), enabled: false }],
        author_exclusion_warnings: [],
      });
      await save;
    });
    expect(result.current.authorExclusionSteps).toEqual([stepFor("b")]);
    expect(result.current.authorExclusionWarnings).toEqual([warningFor("b")]);
  });
});
