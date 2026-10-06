import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useLayoutEffect, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { CoordinatorsListPage } from "./coordinators-list-page";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { Coordinator, CoordinatorListResponse } from "@/lib/api/domains/coordinator-api";

const transport = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/client", async (original) => ({
  ...(await original<typeof import("@/lib/api/client")>()),
  fetchJson: transport,
}));

type Read = {
  workspace: string;
  resolve: (value: CoordinatorListResponse) => void;
  reject: (error: Error) => void;
  settled: boolean;
};
const reads: Read[] = [];
let store: StoreApi<AppState>;
function CaptureStore() {
  store = useAppStoreApi();
  return null;
}
function Providers({ children }: { children: ReactNode }) {
  return (
    <StateProvider
      initialState={{
        settingsData: { agentsLoaded: true, executorsLoaded: true },
        availableAgents: { items: [], tools: [], loaded: true, loading: false },
      }}
    >
      <CaptureStore />
      <TooltipProvider>{children}</TooltipProvider>
    </StateProvider>
  );
}
function row(workspace: string, id: string): Coordinator {
  return {
    id,
    workspace_id: workspace,
    name: id,
    agent_profile_id: "agent",
    executor_profile_id: "executor",
    context: "",
    conversation_task_id: null,
    created_at: "2026-10-06T00:00:00Z",
    updated_at: "2026-10-06T00:00:00Z",
  };
}
async function settle(index: number, result: string | Error | null) {
  const read = reads[index];
  await act(async () => {
    read.settled = true;
    if (result instanceof Error) read.reject(result);
    else read.resolve({ coordinators: result ? [row(read.workspace, result)] : [] });
  });
}
function expectLinks(workspace: string, id: string) {
  expect(screen.getByTestId(`coordinator-name-link-${id}`).getAttribute("href")).toBe(
    `/settings/workspaces/${workspace}/coordinators/${id}`,
  );
  expect(screen.getByTestId(`coordinator-open-${id}`).getAttribute("href")).toBe(
    `/workspaces/${workspace}/coordinator/${id}`,
  );
  expect(screen.getByTestId(`coordinator-configure-${id}`).getAttribute("href")).toBe(
    `/settings/workspaces/${workspace}/coordinators/${id}`,
  );
}

beforeEach(() => {
  reads.length = 0;
  transport.mockReset();
  transport.mockImplementation((url: string) => {
    const match = url.match(/^\/api\/v1\/workspaces\/([^/]+)\/coordinators$/);
    if (match)
      return new Promise<CoordinatorListResponse>((resolve, reject) =>
        reads.push({ workspace: match[1], resolve, reject, settled: false }),
      );
    throw new Error(`Unexpected rendered list transport: ${url}`);
  });
});
afterEach(async () => {
  cleanup();
  for (let index = 0; index < reads.length; index++) {
    if (!reads[index].settled) await settle(index, null);
  }
});

// @covers AC-COORDINATOR-COORDINATORS-004.8
describe("Coordinator list publication rendered list", () => {
  it("hides foreign links at the workspace commit before passive loading starts", async () => {
    const commits: Array<{ workspace: string; foreignLink: string | null }> = [];
    function CommitObserver({ workspace }: { workspace: string }) {
      useLayoutEffect(() => {
        commits.push({
          workspace,
          foreignLink:
            screen.queryByTestId("coordinator-name-link-A-visible")?.getAttribute("href") ?? null,
        });
      }, [workspace]);
      return null;
    }
    const content = (workspace: string) => (
      <Providers>
        <CoordinatorsListPage workspaceId={workspace} />
        <CommitObserver workspace={workspace} />
      </Providers>
    );
    const view = render(content("A"));
    await settle(0, "A-visible");
    view.rerender(content("B"));
    expect(commits.find((entry) => entry.workspace === "B")?.foreignLink).toBeNull();
    await settle(1, "B-current");
    expectLinks("B", "B-current");
  });

  it.each(["success", "failure"])("retains A links after abandoned B %s", async (outcome) => {
    const content = (workspace: string) => (
      <Providers>
        <CoordinatorsListPage workspaceId={workspace} />
      </Providers>
    );
    const view = render(content("A"));
    await settle(0, "A-visible");
    expectLinks("A", "A-visible");
    view.rerender(content("B"));
    expect(screen.getByTestId("coordinators-loading")).toBeTruthy();
    view.rerender(content("A"));
    await settle(1, outcome === "success" ? "B-foreign" : new Error("B failed"));
    expect(screen.queryByTestId("coordinator-name-link-B-foreign")).toBeNull();
    expect(screen.queryByTestId("coordinators-load-error")).toBeNull();
    expectLinks("A", "A-visible");
    expect(store.getState().coordinators.loading).toBe(false);
  });

  it("shows current failure, repeated Retry failure and successful empty recovery", async () => {
    render(
      <Providers>
        <CoordinatorsListPage workspaceId="A" />
      </Providers>,
    );
    await settle(0, new Error("current failure"));
    expect(screen.getByTestId("coordinators-load-error")).toBeTruthy();
    expect(screen.queryByTestId("coordinators-empty-state")).toBeNull();
    expect(store.getState().coordinators.loaded).toBe(false);
    fireEvent.click(screen.getByTestId("coordinators-retry-button"));
    await settle(1, new Error("retry failure"));
    fireEvent.click(screen.getByTestId("coordinators-retry-button"));
    await settle(2, null);
    expect(screen.queryByTestId("coordinators-load-error")).toBeNull();
    expect(screen.getByTestId("coordinators-empty-state")).toBeTruthy();
    expect(store.getState().coordinators).toMatchObject({
      items: [],
      loaded: true,
      loading: false,
    });
  });
});
