import { test, expect } from "../../fixtures/test-base";
import {
  createContinuationFixture,
  waitForContinuationMessage,
  assertNativeContinuationTrace,
  assertNativeNoContinuationTrace,
} from "../../helpers/provider-interruption-continuation";
import { SessionPage } from "../../pages/session-page";
import fs from "node:fs";
import path from "node:path";

test.setTimeout(480_000);

for (const scenario of [
  "read",
  "write",
  "shell",
  "output",
  "read-restore-transient",
  "read-restore-hard",
]) {
  test(`integration: ${scenario} continues in the same live native conversation without original prompt replay`, async ({
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
      assertNativeContinuationTrace(fixture.tracePath, scenario);
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
      expect(
        messages.filter(
          (message) => message.author_type === "user" && message.content === "continue",
        ),
      ).toHaveLength(0);
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
    await expect(session.activeChat().getByText("continue", { exact: true })).toHaveCount(0);
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
    await expect(session.activeChat().getByText("continue", { exact: true })).toHaveCount(0);
    const { messages: reloadedMessages } = await apiClient.listSessionMessages(fixture.sessionId);
    expect(
      reloadedMessages.filter(
        (message) => message.author_type === "user" && message.content === "continue",
      ),
    ).toHaveLength(0);
    const viewer = await testPage.context().newPage();
    try {
      await viewer.goto(`/t/${fixture.taskId}`);
      const second = new SessionPage(viewer);
      await second.waitForLoad();
      await expect(second.transientRetryCard()).toHaveCount(1);
      await expect(second.transientRetryCard()).toContainText("Continuing");
      await expect(second.activeChat().getByText("continue", { exact: true })).toHaveCount(0);
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
    expect(cancelled.metadata?.runtime_retained).toBe(true);
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);
    await expect(session.transientRetryCard()).toBeHidden();
    await expect(testPage.getByTestId("session-recovery-card")).toHaveCount(0);
    await expect
      .poll(
        async () =>
          (await apiClient.listTaskSessions(fixture.taskId)).sessions.find(
            (candidate) => candidate.id === fixture.sessionId,
          )?.state,
      )
      .toBe("WAITING_FOR_INPUT");
    assertNativeContinuationTrace(fixture.tracePath, "read-hold");
    await session.sendMessage("continue");
    await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.author_type === "user" && message.content === "continue",
    );
    await expect(session.activeChat().getByText("continue", { exact: true })).toBeVisible();
    await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) =>
        message.author_type === "agent" &&
        message.content?.includes('completed the analysis of your request: "continue"') === true,
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

for (const scenario of ["pending", "unknown"]) {
  test(`desktop: ${scenario} refuses replay and keeps the live runtime`, async ({
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
      const failure = await waitForContinuationMessage(
        apiClient,
        fixture.sessionId,
        (message) => message.metadata?.runtime_retained === true,
      );
      expect(failure.metadata?.attempts_started ?? 0).toBe(0);
      await expect(session.recoveryResumeButton()).toHaveCount(0);
      await expect(session.recoveryFreshButton()).toHaveCount(0);
      await expect(session.transientRetryCard()).toBeHidden();
      await expect(testPage.getByTestId("session-recovery-card")).toHaveCount(0);
      await expect
        .poll(
          async () =>
            (await apiClient.listTaskSessions(fixture.taskId)).sessions.find(
              (candidate) => candidate.id === fixture.sessionId,
            )?.state,
        )
        .toBe("WAITING_FOR_INPUT");
      assertNativeNoContinuationTrace(fixture.tracePath, scenario);
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

test("desktop: disabled continuation preserves manual recovery without native replay", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  const fixture = await createContinuationFixture(backend, apiClient, seedData, "read", {
    enabled: false,
  });
  try {
    const recovery = await waitForContinuationMessage(
      apiClient,
      fixture.sessionId,
      (message) => message.metadata?.recovery_reason === "disabled",
    );
    await expect
      .poll(async () => {
        const status = await apiClient.wsRequest<{
          state: string;
          is_agent_running: boolean;
          needs_resume: boolean;
          resume_reason?: string;
        }>("task.session.status", { task_id: fixture.taskId, session_id: fixture.sessionId });
        return {
          state: status.state,
          running: status.is_agent_running,
          needsResume: status.needs_resume,
          reason: status.resume_reason,
        };
      })
      .toEqual({
        state: "WAITING_FOR_INPUT",
        running: false,
        needsResume: false,
        reason: "error_recovery",
      });
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    expect(recovery.metadata?.recovery_actions).toBe(true);
    expect(recovery.metadata?.runtime_retained).not.toBe(true);
    expect(recovery.metadata?.attempts_started ?? 0).toBe(0);
    assertNativeNoContinuationTrace(fixture.tracePath, "read");

    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.recoveryResumeButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
    assertNativeNoContinuationTrace(fixture.tracePath, "read");
  } finally {
    await fixture.dispose();
  }
});

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
    expect(cancelled.metadata?.runtime_retained).toBe(true);
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);
    await expect(session.transientRetryCard()).toBeHidden();
    await expect
      .poll(
        async () =>
          (await apiClient.listTaskSessions(fixture.taskId)).sessions.find(
            (candidate) => candidate.id === fixture.sessionId,
          )?.state,
      )
      .toBe("WAITING_FOR_INPUT");
    await expect(
      session.activeChat().locator('.tiptap.ProseMirror[contenteditable="true"]'),
    ).toBeVisible();
    assertNativeNoContinuationTrace(fixture.tracePath, "read-hold");
  } finally {
    await fixture.dispose();
  }
});

for (const scenario of ["read-ambiguous"]) {
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
      (message) => message.metadata?.runtime_retained === true,
    );
    const queue = await apiClient.getQueueStatus(identity);
    expect(queue.count).toBe(1);
    expect(queue.entries[0].content).toBe("new human request awaiting explicit dispatch");
    assertNativeNoContinuationTrace(fixture.tracePath, "read-hold");
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);
    await expect(session.transientRetryCard()).toBeHidden();
    await expect
      .poll(
        async () =>
          (await apiClient.listTaskSessions(fixture.taskId)).sessions.find(
            (candidate) => candidate.id === fixture.sessionId,
          )?.state,
      )
      .toBe("WAITING_FOR_INPUT");
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
