import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ControlProviderUsage } from "./control-provider-usage";
import { ApiError } from "@/lib/api/client";

const fixture = vi.hoisted(() => ({
  limits: true,
  data: { owner: {}, identity: "owner", workspaces: [{ id: "w1", name: "One" }] },
  state: {
    auth: { mode: "enabled", authenticated: true, user: { id: "owner" } },
    agentProfiles: { items: [{ id: "profile-one", workspace_id: "w1" }] },
  },
  unsafeUsage: vi.fn(async () => ({ executions: [] })),
  safeUsage: vi.fn(),
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => fixture.limits }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: typeof fixture.state) => unknown) => select(fixture.state),
}));
vi.mock("./integrated-control-provider", () => ({
  useIntegratedControl: () => ({ data: fixture.data }),
}));
vi.mock("@/lib/api/domains/provider-usage-api", () => ({
  readProfileExecutionUsage: fixture.unsafeUsage,
  readCachedProfileExecutionUsage: fixture.safeUsage,
}));
vi.mock("@/lib/api/domains/routing-circuits-api", () => ({
  getRoutingCircuits: async () => ({ circuits: [] }),
}));
vi.mock("./control-model-activity", () => ({
  ControlModelActivity: () => <div data-testid="recorded-model-activity" />,
}));
vi.mock("./control-model-blocks", () => ({
  ControlModelBlocks: () => <div data-testid="workspace-routing-health" />,
}));

afterEach(cleanup);
beforeEach(() => {
  fixture.limits = true;
  fixture.unsafeUsage.mockClear();
  fixture.state.agentProfiles.items = [{ id: "profile-one", workspace_id: "w1" }];
  fixture.safeUsage.mockReset();
  fixture.safeUsage.mockImplementation(async (profile_id: string) => ({
    profile_id,
    cached_only: true,
    executions: [],
  }));
});

describe("integrated control usage safety", () => {
  it("never reads the provider-refreshing usage endpoint on mount or control refresh", async () => {
    const view = render(<ControlProviderUsage />);
    await act(async () => {});
    fixture.data = { ...fixture.data };
    view.rerender(<ControlProviderUsage />);
    await act(async () => {});
    expect(fixture.unsafeUsage).not.toHaveBeenCalled();
    expect(fixture.safeUsage).toHaveBeenCalled();
    expect(screen.getByTestId("control-usage-unavailable")).toBeTruthy();
  });

  it("keeps recorded model activity and routing health visible when quota usage is disabled", () => {
    fixture.limits = false;
    render(<ControlProviderUsage workspaceId="w1" />);
    expect(screen.getByTestId("recorded-model-activity")).toBeTruthy();
    expect(screen.getByTestId("workspace-routing-health")).toBeTruthy();
    expect(screen.getByRole("link").getAttribute("href")).toBe("/settings/agents/dynamic");
  });

  it("shows one source warning when several profiles have an unsupported cached route", async () => {
    fixture.state.agentProfiles.items.push({ id: "profile-two", workspace_id: "w1" });
    fixture.safeUsage.mockRejectedValue(new ApiError("unsupported", 404, null));
    render(<ControlProviderUsage />);
    expect(
      await screen.findAllByText(
        "This build does not provide this overview. These values are unconfirmed.",
      ),
    ).toHaveLength(1);
    expect(fixture.unsafeUsage).not.toHaveBeenCalled();
  });
});
