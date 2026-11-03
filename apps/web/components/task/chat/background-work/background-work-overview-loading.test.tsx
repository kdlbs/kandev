import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultFeaturesState } from "@/lib/state/slices/features/features-slice";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { BackgroundWorkPanel } from "./background-work-panel";

const backgroundWorkApi = vi.hoisted(() => {
  let resolveList: ((value: { workloads: unknown[] }) => void) | undefined;
  return {
    listBackgroundWorkloads: vi.fn(
      () =>
        new Promise((resolve) => {
          resolveList = resolve;
        }),
    ),
    resolveList: (workloads: unknown[]) => {
      resolveList?.({ workloads });
    },
  };
});

vi.mock("@/lib/api/domains/background-work-api", () => ({
  listBackgroundWorkloads: backgroundWorkApi.listBackgroundWorkloads,
  executeBackgroundAction: vi.fn(),
}));

const SESSION_ID = "session-initial-load";

function workload(workId: string, state: WorkloadRunObservation["state"]): WorkloadRunObservation {
  return {
    work_id: workId,
    title: workId,
    kind: "shell",
    state,
    capabilities: {
      discovery: "snapshot",
      output: "stream",
      transcript: false,
      parentage: false,
      reasoning_summary: false,
      attributable_usage: false,
    },
    revision: 1,
  };
}

function renderPanel() {
  return render(
    <StateProvider
      initialState={{ features: { ...defaultFeaturesState.features, agentBackgroundWork: true } }}
    >
      <BackgroundWorkPanel panelId="background-work" params={{ sessionId: SESSION_ID }} />
    </StateProvider>,
  );
}

afterEach(() => cleanup());

describe("BackgroundWorkPanel initial tab readiness", () => {
  it.each(["running", "waiting"] as const)(
    "waits for the first read and then selects Active for %s work",
    async (state) => {
      renderPanel();

      expect(screen.getByText("Loading background work...")).toBeTruthy();
      expect(screen.queryByTestId("background-work-tab-active")).toBeNull();

      act(() => backgroundWorkApi.resolveList([workload("active-work", state)]));

      await waitFor(() => {
        expect(screen.getByTestId("background-work-tab-active").getAttribute("data-state")).toBe(
          "active",
        );
      });
      expect(screen.getByTestId("background-workload-card-active-work")).toBeTruthy();
    },
  );

  it("waits for the first read and then selects History for terminal-only work", async () => {
    renderPanel();

    expect(screen.getByText("Loading background work...")).toBeTruthy();
    act(() => backgroundWorkApi.resolveList([workload("completed", "completed")]));

    await waitFor(() => {
      expect(screen.getByTestId("background-work-tab-history").getAttribute("data-state")).toBe(
        "active",
      );
    });
    expect(screen.getByTestId("background-workload-card-completed")).toBeTruthy();
  });
});
