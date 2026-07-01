import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { ModeSelector } from "./mode-selector";

const SESSION_ID = "session-1";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Infinity,
      },
    },
  });
}

function seedSettingsQueries(queryClient: QueryClient) {
  queryClient.setQueryData(qk.settings.agents(), { agents: [] });
  queryClient.setQueryData(qk.settings.availableAgents(), { agents: [], tools: [] });
}

function renderModeSelector(queryClient: QueryClient, initialState?: Partial<AppState>) {
  seedSettingsQueries(queryClient);

  return render(
    <QueryClientProvider client={queryClient}>
      <StateProvider initialState={initialState}>
        <TooltipProvider>
          <ModeSelector sessionId={SESSION_ID} />
        </TooltipProvider>
      </StateProvider>
    </QueryClientProvider>,
  );
}

describe("ModeSelector", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders live session modes from the Query cache", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.sessionRuntime.mode(SESSION_ID), {
      currentModeId: "plan",
      availableModes: [
        { id: "plan", name: "Plan" },
        { id: "build", name: "Build" },
      ],
    });

    renderModeSelector(queryClient);

    expect(screen.getByTestId("session-mode-selector").textContent).toContain("Plan");
  });

  it("falls back to the session snapshot mode when no live mode has arrived", () => {
    const queryClient = createQueryClient();
    const session = {
      id: SESSION_ID,
      task_id: "task-1",
      state: "RUNNING",
      started_at: "2026-06-24T00:00:00Z",
      updated_at: "2026-06-24T00:00:01Z",
      agent_profile_snapshot: { mode: "review_mode" },
    } as unknown as TaskSession;

    renderModeSelector(queryClient, {
      taskSessions: { items: { [SESSION_ID]: session } },
    });

    expect(screen.getByTestId("session-mode-selector").textContent).toContain("Review Mode");
  });
});
