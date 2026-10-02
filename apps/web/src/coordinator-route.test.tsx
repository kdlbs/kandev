import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  setActiveWorkspace: vi.fn(),
  items: [{ id: "ws-a" }, { id: "ws-b" }] as Array<{ id: string }>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { items: mocks.items, activeId: "ws-a" },
      setActiveWorkspace: mocks.setActiveWorkspace,
    }),
}));

vi.mock("@/app/coordinator/needs-you-page-client", () => ({
  NeedsYouPageClient: () => <div data-testid="needs-you-page" />,
}));
vi.mock("@/app/coordinator/queue-page-client", () => ({
  QueuePageClient: () => <div data-testid="queue-page" />,
}));
vi.mock("./spa-route-chrome", () => ({
  AuthRouteRedirect: () => <div data-testid="auth-redirect" />,
  RouteLoading: () => <div data-testid="route-loading" />,
}));

import { CoordinatorRoute } from "./coordinator-route";

describe("CoordinatorRoute workspace activation", () => {
  beforeEach(() => {
    mocks.setActiveWorkspace.mockReset();
    mocks.items = [{ id: "ws-a" }, { id: "ws-b" }];
  });
  afterEach(() => cleanup());

  // @covers AC-COORDINATOR-NEEDS-YOU-003.3
  it("makes the route's workspace the active one on mount", async () => {
    render(<CoordinatorRoute enabled view="needs-you" workspaceId="ws-b" coordinatorId="c-1" />);
    expect(await screen.findByTestId("needs-you-page")).not.toBeNull();
    expect(mocks.setActiveWorkspace).toHaveBeenCalledWith("ws-b");
  });

  it("does not activate a workspace the store does not know", async () => {
    render(<CoordinatorRoute enabled view="queue" workspaceId="ws-x" coordinatorId="c-1" />);
    expect(await screen.findByTestId("queue-page")).not.toBeNull();
    expect(mocks.setActiveWorkspace).not.toHaveBeenCalled();
  });

  it("does not activate anything when the feature is off", () => {
    render(
      <CoordinatorRoute enabled={false} view="needs-you" workspaceId="ws-b" coordinatorId="c-1" />,
    );
    expect(screen.getByTestId("auth-redirect")).not.toBeNull();
    expect(mocks.setActiveWorkspace).not.toHaveBeenCalled();
  });
});
