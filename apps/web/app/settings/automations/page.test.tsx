import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// Regression coverage for the top-level /settings/automations page.
//
// The original implementation was an async server-style component that called
// `await listWorkspaces({ cache: "no-store" })` in its render body. Rendered on
// the client under <Suspense fallback={null}>, React re-invoked it on every
// suspense retry, issuing a fresh uncached fetch each time — an infinite
// render/refetch loop that left the panel blank and flooded the backend with
// ~500 req/s. The page must instead read workspaces from the hydrated store and
// never fetch during render.

type Workspace = { id: string; name: string; description?: string | null };

let mockWorkspaces: Workspace[] = [];
const replaceSpy = vi.fn();
const listWorkspacesSpy = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { workspaces: { items: Workspace[] } }) => unknown) =>
    selector({ workspaces: { items: mockWorkspaces } }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({
    replace: replaceSpy,
    push: vi.fn(),
    refresh: vi.fn(),
    back: vi.fn(),
    forward: vi.fn(),
    prefetch: vi.fn(),
  }),
}));

vi.mock("@/lib/api", () => ({
  listWorkspaces: (...args: unknown[]) => {
    listWorkspacesSpy(...args);
    return Promise.resolve({ workspaces: mockWorkspaces });
  },
}));

import AutomationsTopLevelPage from "./page";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  mockWorkspaces = [];
});

describe("AutomationsTopLevelPage", () => {
  it("renders a workspace picker from store state without fetching", () => {
    mockWorkspaces = [
      { id: "ws-1", name: "Alpha" },
      { id: "ws-2", name: "Beta", description: "second workspace" },
    ];

    render(<AutomationsTopLevelPage />);

    expect(screen.getByText("Alpha")).toBeTruthy();
    expect(screen.getByText("Beta")).toBeTruthy();
    // The core regression: no data fetch happens during render.
    expect(listWorkspacesSpy).not.toHaveBeenCalled();
    expect(replaceSpy).not.toHaveBeenCalled();
  });

  it("redirects to the single workspace's automations when exactly one exists", () => {
    mockWorkspaces = [{ id: "only-ws", name: "Solo" }];

    render(<AutomationsTopLevelPage />);

    // Exactly once — `useRouter()` returns a stable (useMemo([])) reference, so the
    // effect must not re-fire and re-redirect on re-render.
    expect(replaceSpy).toHaveBeenCalledTimes(1);
    expect(replaceSpy).toHaveBeenCalledWith("/settings/workspace/only-ws/automations");
    expect(listWorkspacesSpy).not.toHaveBeenCalled();
  });

  it("shows an empty state when there are no workspaces", () => {
    mockWorkspaces = [];

    render(<AutomationsTopLevelPage />);

    expect(screen.getByText("No workspaces yet")).toBeTruthy();
    expect(listWorkspacesSpy).not.toHaveBeenCalled();
    expect(replaceSpy).not.toHaveBeenCalled();
  });
});
