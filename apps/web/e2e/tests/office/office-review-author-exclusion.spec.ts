import { test, expect } from "../../fixtures/office-fixture";
import { balancedExecutionProfileRouting } from "../../helpers/office-routing";

type RunRow = { id: string; task_id?: string; agent_profile_id: string; status: string };
type StepEvents = NonNullable<
  Parameters<import("../../helpers/api-client").ApiClient["updateWorkflowStep"]>[1]["events"]
>;

/**
 * REQ-OFFICE-REVIEW-AUTHOR-EXCL-002/004: with exclude_author_provider on the
 * Review seat action, the reviewer's run never launches on the provider that
 * did the task's work, and its run detail records the exclusion.
 *
 * Workspace order is claude-acp then codex-acp. Whichever provider the Work
 * run launched on (normally claude-acp; a launch failure can fall back), the
 * reviewer run must launch on the other one.
 */
test.describe("Office reviewer author-provider exclusion", () => {
  test.beforeEach(async ({ apiClient, officeSeed }) => {
    await apiClient.e2eReset(officeSeed.workspaceId, [officeSeed.workflowId]);
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("reviewer run launches on the provider that did not do the work", async ({
    testPage,
    backend,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    test.setTimeout(240_000);
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp,codex-acp" });
    const routing = await balancedExecutionProfileRouting(
      apiClient,
      officeApi,
      officeSeed.workspaceId,
      ["claude-acp", "codex-acp"],
    );
    await officeApi.updateRouting(officeSeed.workspaceId, routing);

    const { steps } = await apiClient.listWorkflowSteps(officeSeed.workflowId);
    const reviewStep = steps.find((s) => s.stage_type === "review");
    expect(reviewStep).toBeTruthy();
    const originalEvents = (reviewStep!.events ?? {}) as StepEvents;
    const guardedOnEnter = (originalEvents.on_enter ?? []).map((action) =>
      action.type === "ensure_participant_seat"
        ? { ...action, config: { ...action.config, exclude_author_provider: true } }
        : action,
    );
    await apiClient.updateWorkflowStep(reviewStep!.id, {
      events: { ...originalEvents, on_enter: guardedOnEnter },
    });

    try {
      const ceo = (await officeApi.getAgent(officeSeed.agentId)) as {
        executor_preference?: string;
      };
      const reviewer = (await officeApi.createAgent(officeSeed.workspaceId, {
        name: "Exclusion Reviewer",
        role: "specialist",
        executor_preference: ceo.executor_preference,
      })) as { id: string };

      const task = (await officeApi.createTask(officeSeed.workspaceId, "Author exclusion", {
        workflow_id: officeSeed.workflowId,
      })) as { id: string };
      await officeApi.assignTask(task.id, officeSeed.agentId);

      const finishedRoute = async (agentId: string) => {
        const { runs = [] } = (await officeApi.listRuns(officeSeed.workspaceId)) as {
          runs?: RunRow[];
        };
        const run = runs.find(
          (r) => r.task_id === task.id && r.agent_profile_id === agentId && r.status === "finished",
        );
        if (!run) return undefined;
        const { attempts } = await officeApi.listRouteAttempts(run.id);
        const launched = attempts.find((a) => a.outcome === "launched");
        return launched && { runId: run.id, providerId: launched.provider_id };
      };

      // An attempt is marked launched before the agent starts. Wait for the
      // Work run to finish before moving the task out of Work.
      let author: string | undefined;
      await expect
        .poll(
          async () => {
            author = (await finishedRoute(officeSeed.agentId))?.providerId;
            return author;
          },
          { timeout: 40_000, message: "work run never finished with a launched provider" },
        )
        .toBeDefined();
      const other = author === "claude-acp" ? "codex-acp" : "claude-acp";

      // Entering Review seats the reviewer and queues its run. The move is
      // refused while the Work session is still active, so retry until it lands.
      await expect
        .poll(
          async () => {
            try {
              await apiClient.moveTask(task.id, officeSeed.workflowId, reviewStep!.id);
              return "moved";
            } catch (err) {
              if (String(err).includes("task_move_active_session")) return "busy";
              throw err;
            }
          },
          { timeout: 40_000, message: "task never moved to Review" },
        )
        .toBe("moved");

      let reviewRunId: string | undefined;
      await expect
        .poll(
          async () => {
            const route = await finishedRoute(reviewer.id);
            reviewRunId = route?.runId;
            return route?.providerId;
          },
          { timeout: 120_000, message: "reviewer run never finished on the other provider" },
        )
        .toBe(other);

      await testPage.goto(`/office/agents/${reviewer.id}/runs/${reviewRunId}`);
      const events = testPage.getByTestId("events-log");
      await expect(events).toContainText("routing.author_exclusion", { timeout: 15_000 });
      await expect(events).toContainText(`"author_provider_id":"${author}"`);
    } finally {
      await apiClient.updateWorkflowStep(reviewStep!.id, { events: originalEvents });
    }
  });
});
