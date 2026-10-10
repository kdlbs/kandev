import { expect, test } from "../../fixtures/test-base";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import { attachMessageAddCapture } from "../../helpers/ws-capture";
import {
  observeAgentRuntimeAvailability,
  waitForAgentRuntimeReplacement,
} from "../../helpers/agent-runtime-availability";
import {
  captureSilentRestartSnapshot,
  countSessionMessageAdds,
  expectSilentRestartRestoration,
} from "../../helpers/silent-restart-recovery";
import { SessionPage } from "../../pages/session-page";

type RuntimeStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      agentRuntime: {
        status: string;
        boot_id?: string;
        runtime_epoch?: number;
      } | null;
    };
  };
};

test.describe("Agent runtime replacement", () => {
  test.describe.configure({ retries: 0 });

  test("restores an interrupted conversation silently after backend restart", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
    backend,
  }) => {
    test.setTimeout(300_000);
    const traffic = attachGatewayTrafficCapture(testPage);
    const messageAdds = attachMessageAddCapture(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Agent runtime replacement keeps this task open",
      seedData.agentProfileId,
      {
        description: "/slow 30",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    const bootID = await apiClient.getBackendBootID();
    const runtime = await testPage.evaluate(
      () => (window as RuntimeStoreWindow).__KANDEV_E2E_STORE__?.getState().agentRuntime ?? null,
    );
    if (!runtime || runtime.status !== "available" || runtime.runtime_epoch === undefined) {
      throw new Error("local runtime is not available before the child-death test");
    }
    const pageTimeOrigin = await testPage.evaluate(() => performance.timeOrigin);
    await observeAgentRuntimeAvailability(testPage);

    const killed = await apiClient.killLocalAgentRuntimeChild();
    expect(killed.killed).toBe(true);
    expect(killed.runtime_epoch).toBe(runtime.runtime_epoch);
    await waitForAgentRuntimeReplacement(testPage, runtime.runtime_epoch, bootID);

    expect(await apiClient.getBackendBootID()).toBe(bootID);
    expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(pageTimeOrigin);
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.some((item) => {
          const error = item.metadata?.last_agent_error as { code?: string } | undefined;
          return error?.code === "DURABLE_DELIVERY_UNCERTAIN";
        });
      })
      .toBe(true);
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
    await expect(testPage.getByTestId("agent-runtime-alert")).toHaveCount(0);
    await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toHaveCount(1);
    await expect(
      session.chat.getByText("Delivery was interrupted. The prompt outcome is uncertain.", {
        exact: true,
      }),
    ).toBeVisible({ timeout: 30_000 });
    await expect(testPage.getByTestId("interrupted-sessions-notice")).toHaveCount(0);
    await expect(testPage.getByTestId("interrupted-session-continuation")).toHaveCount(0);
    await expect(session.chat.getByTestId("recovery-stop-button")).toBeEnabled();
    await expect(session.chat.getByText("Slow response complete", { exact: false })).toHaveCount(0);
    if (!task.session_id) throw new Error("missing interrupted session");
    const sessionId = task.session_id;
    const beforeRestart = await captureSilentRestartSnapshot(apiClient, task.id, sessionId);
    const messageAddsBeforeRestart = countSessionMessageAdds(
      messageAdds.frames,
      task.id,
      sessionId,
    );
    const sessionRecoveriesBeforeRestart = traffic.frames.filter(
      (frame) =>
        frame.direction === "sent" &&
        frame.action === "session.recover" &&
        frame.sessionId === sessionId,
    ).length;

    await backend.restart();
    expect(await apiClient.getBackendBootID()).not.toBe(bootID);
    await testPage.reload();
    await session.waitForLoad();
    await expectSilentRestartRestoration(
      testPage,
      apiClient,
      task.id,
      beforeRestart,
      traffic.frames,
    );
    expect(countSessionMessageAdds(messageAdds.frames, task.id, sessionId)).toBe(
      messageAddsBeforeRestart,
    );
    expect(
      traffic.frames.filter(
        (frame) =>
          frame.direction === "sent" &&
          frame.action === "session.recover" &&
          frame.sessionId === sessionId,
      ),
    ).toHaveLength(sessionRecoveriesBeforeRestart);

    await session.waitForChatIdle({ timeout: 60_000, requireEditable: true });
    await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toHaveCount(1);
    await prCapture.screenshot("desktop-silent-restart-restoration", {
      caption: "The existing conversation is ready after a backend restart",
    });

    await session.sendMessage("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
    await session.waitForChatIdle({ timeout: 45_000 });
    expect(countSessionMessageAdds(messageAdds.frames, task.id, sessionId)).toBe(
      messageAddsBeforeRestart + 1,
    );
    expect(
      (await apiClient.listSessionMessages(sessionId)).messages.filter(
        (message) => message.author_type === "user" && message.content === "/e2e:simple-message",
      ),
    ).toHaveLength(1);
    expect((await apiClient.listSessionTurns(sessionId)).turns).toHaveLength(
      beforeRestart.turnIds.length + 1,
    );
  });
});
