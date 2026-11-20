import { test, expect } from "../../fixtures/office-fixture";
import { balancedExecutionProfileRouting } from "../../helpers/office-routing";

type RunRow = { id: string; task_id?: string; agent_profile_id: string };
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

      const launchedProvider = async (agentId: string): Promise<string> => {
        const runs = (await officeApi.listRuns(officeSeed.workspaceId)) as { runs?: RunRow[] };
        for (const run of runs.runs ?? []) {
          if (run.task_id !== task.id || run.agent_profile_id !== agentId) continue;
          const { attempts } = await officeApi.listRouteAttempts(run.id);
          const launched = attempts.find((a) => a.outcome === "launched");
          if (launched) return launched.provider_id;
        }
        return "";
      };

      await expect
        .poll(() => launchedProvider(officeSeed.agentId), {
          timeout: 40_000,
          message: "work run never launched",
        })
        .not.toBe("");
      const author = await launchedProvider(officeSeed.agentId);
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

      await expect
        .poll(() => launchedProvider(reviewer.id), {
          // A first claim can hit checkout contention with the finishing
          // Work run and is retried after its backoff (about a minute).
          timeout: 120_000,
          message: "reviewer run never launched",
        })
        .toBe(other);

      const runs = (await officeApi.listRuns(officeSeed.workspaceId)) as { runs?: RunRow[] };
      const reviewRun = (runs.runs ?? []).find(
        (r) => r.task_id === task.id && r.agent_profile_id === reviewer.id,
      );
      await testPage.goto(`/office/agents/${reviewer.id}/runs/${reviewRun!.id}`);
      const events = testPage.getByTestId("events-log");
      await expect(events).toContainText("routing.author_exclusion", { timeout: 15_000 });
      await expect(events).toContainText(`"author_provider_id":"${author}"`);
    } finally {
      await apiClient.updateWorkflowStep(reviewStep!.id, { events: originalEvents });
    }
  });
});
