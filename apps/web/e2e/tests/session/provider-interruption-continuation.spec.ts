import { test, expect } from "../../fixtures/test-base";
import {
  createContinuationFixture,
  waitForContinuationMessage,
  assertNativeContinuationTrace,
} from "../../helpers/provider-interruption-continuation";
import { SessionPage } from "../../pages/session-page";
import fs from "node:fs";
import path from "node:path";

test.setTimeout(480_000);

for (const scenario of ["read", "output", "read-restore-transient"]) {
  test(`integration: ${scenario} restores the same native conversation without original prompt replay`, async ({
    backend,
    apiClient,
    seedData,
  }) => {
    const fixture = await createContinuationFixture(backend, apiClient, seedData, scenario);
    try {
      const result = await waitForContinuationMessage(
        apiClient,
        fixture.sessionId,
        (message) => message.content?.includes("Mock continuation complete:") === true,
      );
      expect(result.content).toContain("original=1 continuation=1");
      assertNativeContinuationTrace(
        fixture.tracePath,
        scenario,
        scenario === "read-restore-transient" ? 2 : 1,
      );
      await expect
        .poll(
          async () =>
            (await apiClient.listTaskSessions(fixture.taskId)).sessions.find(
              (session) => session.id === fixture.sessionId,
            )?.state,
          { timeout: 30_000, message: "accepted continuation reaches terminal settlement" },
        )
        .toBe("WAITING_FOR_INPUT");
      const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
      expect(
        messages.some((message) => message.content?.includes("partial history preserved")),
      ).toBe(true);
      expect(messages.filter((message) => message.metadata?.retrying === true)).toHaveLength(0);
      const { turns } = await apiClient.listSessionTurns(fixture.sessionId);
      const agentTurns = turns.filter((turn) => turn.metadata?.lifecycle_only !== true);
      expect(agentTurns).toHaveLength(2);
      expect(agentTurns.every((turn) => turn.completed_at)).toBe(true);
      const interrupted = agentTurns.filter((turn) => turn.metadata?.interrupted === true);
      expect(interrupted).toHaveLength(1);
      expect(interrupted[0].completed_at).toBe(interrupted[0].started_at);
    } catch (error) {
      const logPath = path.join(backend.tmpDir, ".kandev", "logs", "backend-logs.log");
      console.warn(String(error));
      console.warn(
        JSON.stringify(
          (await apiClient.listTaskSessions(fixture.taskId)).sessions.map(({ id, state }) => ({
            id,
            state,
          })),
        ),
      );
      const stacks = await fetch(`${backend.baseUrl}/debug/pprof/goroutine?debug=2`);
      if (stacks.ok) {
        const dump = await stacks.text();
        await test
          .info()
          .attach("continuation-goroutines", { body: dump, contentType: "text/plain" });
        console.warn("Attached continuation goroutine dump to the test result.");
      }
      if (fs.existsSync(logPath)) {
        console.warn(
          fs
            .readFileSync(logPath, "utf8")
            .split("\n")
            .filter((line) =>
              /agent.completed|stale resume|turn complete|continuation|prompt_generation|native conversation|network is unreachable|bootstrap/.test(
                line,
              ),
            )
            .slice(-40)
            .join("\n"),
        );
      }
      throw error;
    } finally {
      await fixture.dispose();
    }
  });
}

test("desktop: accepted continuation survives reload and can be cancelled", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createContinuationFixture(backend, apiClient, seedData, "read-hold");
  try {
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.content?.includes("Mock continuation accepted:") === true,
    );
    const notice = await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_phase === "continuing",
    );
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.transientRetryCard()).toContainText("Continuing");
    assertNativeContinuationTrace(fixture.tracePath, "read-hold");
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toHaveCount(1);
    const after = await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_phase === "continuing",
    );
    expect(after.id).toBe(notice.id);
    const viewer = await testPage.context().newPage();
    try {
      await viewer.goto(`/t/${fixture.taskId}`);
      const second = new SessionPage(viewer);
      await second.waitForLoad();
      await expect(second.transientRetryCard()).toHaveCount(1);
      await expect(second.transientRetryCard()).toContainText("Continuing");
      assertNativeContinuationTrace(fixture.tracePath, "read-hold");
    } finally {
      await viewer.close();
    }
    await session.recoveryCancelRetryButton().click();
    const cancelled = await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_disposition === "cancelled",
    );
    expect(cancelled.metadata?.attempts_started).toBe(1);
    await expect(session.recoveryResumeButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
    await expect(testPage.getByTestId("session-recovery-card")).not.toContainText("exhausted");
  } catch (error) {
    console.warn(
      JSON.stringify(
        (await apiClient.listTaskSessions(fixture.taskId)).sessions.map(({ id, state }) => ({
          id,
          state,
        })),
      ),
    );
    console.warn(JSON.stringify(await apiClient.listSessionMessages(fixture.sessionId)));
    console.warn(await testPage.locator("body").innerText());
    throw error;
  } finally {
    await fixture.dispose();
  }
});

for (const { scenario, enabled } of [
  { scenario: "write", enabled: true },
  { scenario: "pending", enabled: true },
  { scenario: "unknown", enabled: true },
  { scenario: "read", enabled: false },
]) {
  test(`desktop: ${scenario}, continuation=${enabled} requires manual recovery`, async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    const fixture = await createContinuationFixture(backend, apiClient, seedData, scenario, {
      enabled,
    });
    try {
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const recovery = await waitForContinuationMessage(
        apiClient,
        fixture.sessionId,
        (message) => message.metadata?.recovery_actions === true,
      );
      await expect(session.recoveryResumeButton()).toBeVisible();
      await expect(session.transientRetryCard()).toBeHidden();
      await expect(testPage.getByTestId("session-recovery-card")).not.toContainText(
        "after several retries",
      );
      expect(recovery.metadata?.attempts_started ?? 0).toBe(0);
      expect(recovery.metadata?.recovery_reason).toBe(enabled ? "unsafe_work" : "disabled");
      await expect(testPage.getByTestId("session-recovery-card")).toContainText(
        enabled ? "outcome is uncertain" : "Automatic continuation is disabled",
      );
    } catch (error) {
      console.warn(
        JSON.stringify(
          (await apiClient.listTaskSessions(fixture.taskId)).sessions.map(({ id, state }) => ({
            id,
            state,
          })),
        ),
      );
      console.warn(JSON.stringify(await apiClient.listSessionMessages(fixture.sessionId)));
      console.warn(await testPage.locator("body").innerText());
      throw error;
    } finally {
      await fixture.dispose();
    }
  });
}

test("desktop: Cancel while waiting prevents native restore", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createContinuationFixture(backend, apiClient, seedData, "read-hold");
  try {
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toContainText("Continuing in", { timeout: 30_000 });
    await session.recoveryCancelRetryButton().click();
    const cancelled = await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_disposition === "cancelled",
    );
    expect(cancelled.metadata?.attempts_started).toBe(0);
    await expect(session.recoveryResumeButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
    const trace = fs.readFileSync(fixture.tracePath, "utf8");
    expect(trace).not.toContain('"event":"session_load"');
    expect(trace).not.toContain(
      "Your previous turn was interrupted by a temporary connection failure.",
    );
  } finally {
    await fixture.dispose();
  }
});

for (const scenario of ["read-restore-hard", "read-ambiguous"]) {
  test(`desktop: ${scenario} stops automatic recovery without replay`, async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    const fixture = await createContinuationFixture(backend, apiClient, seedData, scenario);
    try {
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const recovery = await waitForContinuationMessage(
        apiClient,
        fixture.sessionId,
        (message) =>
          message.metadata?.recovery_mode === "continue" &&
          message.metadata?.recovery_actions === true,
      );
      expect(recovery.metadata?.attempts_started).toBe(1);
      expect(recovery.metadata?.recovery_disposition).toBe("manual");
      await expect(session.recoveryResumeButton()).toBeVisible();
      await expect(session.transientRetryCard()).toBeHidden();
      if (scenario === "read-ambiguous") {
        assertNativeContinuationTrace(fixture.tracePath, scenario);
      } else {
        const trace = fs.readFileSync(fixture.tracePath, "utf8");
        expect(trace).not.toContain(
          "Your previous turn was interrupted by a temporary connection failure.",
        );
        expect(trace.match(/"event":"session_new"/g)).toHaveLength(1);
      }
    } finally {
      await fixture.dispose();
    }
  });
}

test("desktop: queued human work takes priority over automatic continuation", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createContinuationFixture(backend, apiClient, seedData, "read-hold");
  try {
    await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_phase === "waiting",
    );
    const identity = await apiClient.getQueueSessionIdentity(fixture.taskId, fixture.sessionId);
    await apiClient.setQueueAutoRun(identity, false);
    await apiClient.queueMessage(identity, "new human request awaiting explicit dispatch");
    await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_actions === true,
    );
    const queue = await apiClient.getQueueStatus(identity);
    expect(queue.count).toBe(1);
    expect(queue.entries[0].content).toBe("new human request awaiting explicit dispatch");
    const trace = fs.readFileSync(fixture.tracePath, "utf8");
    expect(trace).not.toContain('"event":"session_load"');
    expect(trace).not.toContain(
      "Your previous turn was interrupted by a temporary connection failure.",
    );
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.recoveryResumeButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
  } finally {
    await fixture.dispose();
  }
});

for (const survives of [false, true]) {
  test(`desktop: backend restart, agent survival=${survives} never redispatches continuation`, async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(480_000);
    const overrides = {
      KANDEV_FEATURES_AGENT_SURVIVAL: String(survives),
      KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION: "true",
    };
    const fixture = await createContinuationFixture(backend, apiClient, seedData, "read-hold", {
      env: overrides,
      executorProfileId: seedData.worktreeExecutorProfileId,
    });
    try {
      await waitForContinuationMessage(
        apiClient,
        fixture.sessionId,
        (message) => message.metadata?.recovery_phase === "continuing",
      );
      assertNativeContinuationTrace(fixture.tracePath, "read-hold");
      await backend.restart(overrides);
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(session.transientRetryCard()).toBeHidden();
      if (survives) {
        await expect(session.cancelAgentButton()).toBeVisible();
        await expect(session.recoveryResumeButton()).toBeHidden();
        const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
        expect(sessions.find((candidate) => candidate.id === fixture.sessionId)?.state).toBe(
          "RUNNING",
        );
      } else {
        const recovery = await waitForContinuationMessage(
          apiClient,
          fixture.sessionId,
          (message) => message.metadata?.recovery_disposition === "restart_interrupted",
        );
        expect(recovery.metadata?.attempts_started).toBe(1);
        await expect(session.recoveryResumeButton()).toBeVisible();
      }
      assertNativeContinuationTrace(fixture.tracePath, "read-hold");
      await expect(session.activeChat()).toContainText("partial history preserved");
    } finally {
      await fixture.dispose();
    }
  });
}
