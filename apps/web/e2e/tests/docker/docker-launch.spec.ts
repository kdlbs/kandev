import { spawnSync } from "node:child_process";
import { test, expect } from "../../fixtures/docker-test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

function dockerInspectExists(containerID: string): boolean {
  const res = spawnSync("docker", ["inspect", containerID], { stdio: "ignore" });
  return res.status === 0;
}

function dockerRemove(containerID: string): void {
  spawnSync("docker", ["rm", "-f", containerID], { stdio: "ignore" });
}

function dockerStop(containerID: string): void {
  const res = spawnSync("docker", ["stop", containerID], { stdio: "ignore" });
  if (res.status !== 0) {
    throw new Error(`failed to stop Docker container ${containerID}`);
  }
}

function dockerState(containerID: string): string {
  const res = spawnSync("docker", ["inspect", "-f", "{{.State.Status}}", containerID], {
    encoding: "utf8",
  });
  if (res.status !== 0) return "missing";
  return res.stdout.trim();
}

async function waitForLatestSessionDone(
  apiClient: import("../../helpers/api-client").ApiClient,
  taskId: string,
  expectedCount: number,
  message: string,
  timeout = 120_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        if (sessions.length < expectedCount) return false;
        const latest = sessions[sessions.length - 1];
        return DONE_STATES.includes(latest?.state ?? "");
      },
      { timeout, message },
    )
    .toBe(true);
}

async function waitForSessionDone(
  apiClient: import("../../helpers/api-client").ApiClient,
  taskId: string,
  sessionId: string,
  message: string,
  timeout = 120_000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        const session = sessions.find((s) => s.id === sessionId);
        return DONE_STATES.includes(session?.state ?? "");
      },
      { timeout, message },
    )
    .toBe(true);
}

test.describe("Docker executor — launch + reuse + recovery", () => {
  test("launches a session in a real container and exposes container_id", async ({
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker Launch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );

    await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for first Docker session");

    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.executor_type).toBe("local_docker");
    expect(env!.container_id, "task environment must record container_id").toBeTruthy();
    expect(dockerInspectExists(env!.container_id!), "container should exist on host").toBe(true);
  });

  test("restarts an externally stopped container and reuses the task environment", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker External Stop Reuse",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for first Docker session");
    const before = await apiClient.getTaskEnvironment(task.id);
    expect(before?.container_id).toBeTruthy();

    dockerStop(before!.container_id!);
    await expect
      .poll(() => dockerState(before!.container_id!), {
        timeout: 10_000,
        message: "Waiting for external Docker stop",
      })
      .toBe("exited");

    const launched = await apiClient.launchSession({
      task_id: task.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.dockerExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: "/e2e:simple-message",
    });

    await waitForSessionDone(
      apiClient,
      task.id,
      launched.session_id,
      "Waiting for second Docker session",
    );
    const after = await apiClient.getTaskEnvironment(task.id);
    expect(after?.id).toBe(before!.id);
    expect(after?.container_id).toBe(before!.container_id);
    await expect
      .poll(() => dockerState(before!.container_id!), {
        timeout: 30_000,
        message: "Waiting for Docker container to restart",
      })
      .toBe("running");
  });

  test("blocks chat input after an externally stopped container disconnects the executor", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(180_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker External Stop Chat Gate",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for first Docker session");
    const before = await apiClient.getTaskEnvironment(task.id);
    expect(before?.container_id).toBeTruthy();

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await expect(editor).toHaveAttribute("contenteditable", "true");

    dockerStop(before!.container_id!);
    await expect
      .poll(() => dockerState(before!.container_id!), {
        timeout: 10_000,
        message: "Waiting for external Docker stop",
      })
      .toBe("exited");
    await expect
      .poll(
        async () => {
          const res = await apiClient.rawRequest(
            "GET",
            `/api/v1/tasks/${task.id}/environment/live`,
          );
          const live = (await res.json()) as { container?: { state?: string } };
          return live.container?.state;
        },
        {
          timeout: 10_000,
          message: "Waiting for Kandev live environment status to observe the stop",
        },
      )
      .toBe("exited");

    await expect(editor).toBeHidden({ timeout: 15_000 });
    await expect(session.failedSessionResumeButton()).toBeVisible();
    await session.failedSessionResumeButton().click();

    await expect
      .poll(
        async () => {
          const res = await apiClient.rawRequest(
            "GET",
            `/api/v1/tasks/${task.id}/environment/live`,
          );
          const live = (await res.json()) as { container?: { state?: string } };
          return live.container?.state;
        },
        {
          timeout: 30_000,
          message: "Waiting for explicit restart to bring the container back",
        },
      )
      .toBe("running");

    await session.clickTab("Terminal");
    await session.expectTerminalConnected(30_000);
    await session.typeInTerminal("printf terminal-after-restart");
    await session.expectTerminalHasText("terminal-after-restart");
  });

  // FIXME: blocked on backend gap — workflow-prepared sessions (the "Review" step
  // auto-creates a session without invoking persistTaskEnvironment, so
  // session.task_environment_id stays empty for those rows). Re-enable once
  // the prepare-only path also stamps task_environment_id from the existing env.
  test.fixme("multiple sessions on the same task share one task environment", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker Multi-Session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for first session");
    const before = await apiClient.getTaskEnvironment(task.id);
    expect(before!.container_id).toBeTruthy();

    await apiClient.launchSession({
      task_id: task.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.dockerExecutorProfileId,
      prompt: "/e2e:simple-message",
      auto_start: true,
    });

    // Wait for the session count to grow (workflow may also create sessions),
    // then for the latest one to settle.
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 30_000,
        message: "Waiting for an additional session to be created",
      })
      .toBeGreaterThanOrEqual(2);
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[sessions.length - 1]?.state ?? "");
        },
        { timeout: 60_000, message: "Waiting for latest session to settle" },
      )
      .toBe(true);

    // The durable contract: every session on this task is bound to the same
    // task_environment_id. The container itself may be recreated under the
    // hood, but the env row is the stable handle.
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.length).toBeGreaterThanOrEqual(2);
    for (const s of sessions) {
      expect(s.task_environment_id, `session ${s.id} should reuse the env`).toBe(before!.id);
    }
  });

  // FIXME: blocked on backend gap — DockerExecutor.reconnectToContainer constructs
  // its agentctl ControlClient with no auth token (the original handshake token
  // is held only in memory and lost across launches). Reconnect 401s, the
  // executor falls back to creating a fresh container on each launch, and the
  // task environment row never updates its container_id. Once the executor
  // persists/looks up the auth token (e.g. via the SecretStore secret already
  // stamped on the previous execution's metadata), this test can verify the
  // recovery path picks a brand-new container_id.
  test.fixme("recovers from an externally removed container by launching a fresh one", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker External Stop",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for first session");
    const before = await apiClient.getTaskEnvironment(task.id);
    expect(before!.container_id).toBeTruthy();

    // Simulate operator action: remove the container outside of kandev.
    dockerRemove(before!.container_id!);
    await expect
      .poll(() => dockerInspectExists(before!.container_id!), { timeout: 10_000 })
      .toBe(false);

    await apiClient.launchSession({
      task_id: task.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.dockerExecutorProfileId,
      prompt: "/e2e:simple-message",
      auto_start: true,
    });

    await waitForLatestSessionDone(apiClient, task.id, 2, "Waiting for recovery session");
    const after = await apiClient.getTaskEnvironment(task.id);
    expect(after!.container_id, "must record a container_id on recovery").toBeTruthy();
    expect(after!.container_id).not.toBe(before!.container_id);
    expect(dockerInspectExists(after!.container_id!), "recovery container should exist").toBe(true);
  });
});
