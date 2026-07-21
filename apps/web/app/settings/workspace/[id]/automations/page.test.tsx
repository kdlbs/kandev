import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// Regression coverage for the workspace-scoped
// /settings/workspace/[id]/automations page.
//
// The original implementation was an async server-style component that awaited
// a `params` promise in its render body (`async function AutomationsPage`).
// Rendered on the client under <Suspense>, React logged two React errors:
// "<AutomationsPage> is an async Client Component. Only Server Components can be
// async" and "A component was suspended by an uncached promise". The page must
// be a plain synchronous client component that takes `workspaceId` directly and
// delegates data loading to <AutomationsListPage>.

const { listPageSpy } = vi.hoisted(() => ({ listPageSpy: vi.fn() }));

vi.mock("@/components/automations/automations-list-page", () => ({
  AutomationsListPage: (props: { workspaceId: string }) => {
    listPageSpy(props);
    return <div data-testid="automations-list-page">{props.workspaceId}</div>;
  },
}));

import AutomationsPage from "./page";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AutomationsPage (workspace-scoped)", () => {
  it("is a synchronous component, not an async one", () => {
    // An async function's constructor name is "AsyncFunction"; a plain one is
    // "Function". This is the core regression: an async client component throws.
    expect(AutomationsPage.constructor.name).toBe("Function");
  });

  it("renders the automations list for the given workspace without suspending", () => {
    const element = render(<AutomationsPage workspaceId="ws-42" />);

    // Renders synchronously (no thrown promise / suspense fallback) and forwards
    // the workspace id straight through to the list page.
    expect(screen.getByTestId("automations-list-page").textContent).toBe("ws-42");
    expect(listPageSpy).toHaveBeenCalledTimes(1);
    expect(listPageSpy).toHaveBeenCalledWith({ workspaceId: "ws-42" });

    element.unmount();
  });
});
