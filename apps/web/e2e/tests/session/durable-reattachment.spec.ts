import { expect, test } from "../../fixtures/test-base";
import { triggerActualDeliveryDisconnect } from "../../helpers/durable-reattachment";
import { SessionPage } from "../../pages/session-page";

test.describe("durable delivery reattachment", () => {
  test.describe.configure({ retries: 0 });

  test("persists an actual disconnect and keeps retry and Stop bound to the original session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const result = await triggerActualDeliveryDisconnect(
      testPage,
      apiClient,
      seedData,
      "Durable agentctl disconnect reattachment",
    );
    const { traffic, task, sessionId, recovery } = result;
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await testPage.reload();
    await session.waitForLoad();
    const banner = testPage.getByTestId("failed-session-banner");
    await expect(banner).toBeVisible({ timeout: 30_000 });
    await expect(banner).toContainText(
      "Delivery was interrupted. The prompt outcome is uncertain.",
    );
    expect(recovery.phase).toBe("uncertain");

    const promptFramesBeforeRetry = traffic.frames.filter(
      (frame) =>
        frame.direction === "sent" &&
        /prompt|message\.added|chat\.submit/i.test(frame.action ?? ""),
    ).length;
    await banner.getByTestId("recovery-retry-connection-button").click();
    await expect(banner.getByTestId("session-recovery-error")).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(() =>
        traffic.frames.some(
          (frame) =>
            frame.direction === "sent" &&
            frame.action === "session.recover" &&
            frame.sessionId === sessionId,
        ),
      )
      .toBe(true);
    expect(
      traffic.frames.filter(
        (frame) =>
          frame.direction === "sent" &&
          /prompt|message\.added|chat\.submit/i.test(frame.action ?? ""),
      ),
    ).toHaveLength(promptFramesBeforeRetry);

    await banner.getByTestId("recovery-stop-button").click();
    await expect(banner.getByTestId("delivery-stop-outcome-unconfirmed")).toBeVisible({
      timeout: 10_000,
    });
    await expect
      .poll(() =>
        traffic.frames.some(
          (frame) =>
            frame.direction === "sent" &&
            frame.action === "session.stop" &&
            frame.sessionId === sessionId,
        ),
      )
      .toBe(true);
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
  });

  test("reconciles an actual updates-stream loss without replaying the active prompt", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Durable reattachment recovers one live agent stream",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.sendMessage("/slow 5s");
    await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    const listed = await apiClient.listTaskSessions(task.id);
    const active = listed.sessions.find((candidate) => candidate.is_primary);
    if (!active) throw new Error("created task has no primary session");

    const disconnected = await apiClient.disconnectSessionAgentUpdatesStream(active.id);
    expect(disconnected.disconnected).toBe(true);
    await expect(
      session.chat.getByText("Slow response complete after 5s.", { exact: false }),
    ).toBeVisible({
      timeout: 60_000,
    });
    let latestSettlement: unknown;
    try {
      await expect
        .poll(
          async () => {
            const [current, turns] = await Promise.all([
              apiClient.listTaskSessions(task.id),
              apiClient.listSessionTurns(active.id),
            ]);
            const recovered = current.sessions.find((candidate) => candidate.id === active.id);
            latestSettlement = {
              state: recovered?.state,
              agent_execution_id: recovered?.agent_execution_id,
              recovery: recovered?.metadata?.agent_delivery_recovery,
              turns: turns.turns,
            };
            return recovered?.state ?? "";
          },
          {
            timeout: 30_000,
            message: "the recovered terminal event must settle the original session",
          },
        )
        .toBe("WAITING_FOR_INPUT");
    } catch (error) {
      throw new Error(`${String(error)}\nLatest settlement: ${JSON.stringify(latestSettlement)}`);
    }
    const beforeNextPrompt = await apiClient.listSessionMessages(active.id);
    expect(
      beforeNextPrompt.messages.filter(
        (message) => message.author_type === "user" && message.content.trim() === "/slow 5s",
      ),
    ).toHaveLength(1);
    await session.sendMessage("/slow 1s");
    await expect(
      session.chat.getByText("Running slow response (1s total)...", { exact: false }),
    ).toBeVisible({
      timeout: 30_000,
    });
    await expect(
      session.chat.getByText("Slow response complete after 1s.", { exact: false }),
    ).toBeVisible({
      timeout: 30_000,
    });
    const afterNextPrompt = await apiClient.listSessionMessages(active.id);
    expect(
      afterNextPrompt.messages.filter(
        (message) => message.author_type === "user" && message.content.trim() === "/slow 5s",
      ),
    ).toHaveLength(1);
    expect(
      afterNextPrompt.messages.filter(
        (message) => message.author_type === "user" && message.content.trim() === "/slow 1s",
      ),
    ).toHaveLength(1);
  });
});
