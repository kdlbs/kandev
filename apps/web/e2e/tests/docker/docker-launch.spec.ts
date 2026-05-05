import { spawnSync } from "node:child_process";
import { test, expect } from "../../fixtures/docker-test-base";
import { E2E_IMAGE_TAG } from "../../fixtures/docker-probe";
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

function dockerCurrentBranch(containerID: string): string {
  const res = spawnSync(
    "docker",
    ["exec", containerID, "git", "-C", "/workspace", "branch", "--show-current"],
    {
      encoding: "utf8",
    },
  );
  if (res.status !== 0) {
    const diag = spawnSync(
      "docker",
      [
        "exec",
        containerID,
        "sh",
        "-lc",
        "ls -la /workspace; git -C /workspace status --short --branch",
      ],
      { encoding: "utf8" },
    );
    const logs = spawnSync("docker", ["logs", "--tail", "40", containerID], { encoding: "utf8" });
    return [
      `ERR status=${res.status} state=${dockerState(containerID)}`,
      `stderr=${res.stderr.trim()}`,
      `diag=${diag.stdout.trim()} ${diag.stderr.trim()}`,
      `logs=${logs.stdout.trim()} ${logs.stderr.trim()}`,
    ].join("\n");
  }
  return res.stdout.trim();
}

async function waitForDockerContainerRemoved(containerID: string, message: string): Promise<void> {
  await expect
    .poll(() => dockerInspectExists(containerID), {
      timeout: 60_000,
      message,
    })
    .toBe(false);
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
        // API returns sessions newest-first.
        const latest = sessions[0];
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

async function waitForSessionEnvironment(
  apiClient: import("../../helpers/api-client").ApiClient,
  options: {
    taskId: string;
    sessionId: string;
    expectedEnvironmentId: string;
    message: string;
    timeout?: number;
  },
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(options.taskId);
        const session = sessions.find((s) => s.id === options.sessionId);
        return session?.task_environment_id ?? "";
      },
      { timeout: options.timeout ?? 60_000, message: options.message },
    )
    .toBe(options.expectedEnvironmentId);
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
    // Suffix is the first 6 chars of the task UUID (deterministic so resume
    // lands on the same branch even when the env row predates the suffix
    // change), so the pattern is hex chars rather than a 3-char random tail.
    expect(dockerCurrentBranch(env!.container_id!)).toMatch(/^feature\/docker-launch-[0-9a-f]{6}$/);
  });

  test("shows Docker container wait progress during slow bootstrap", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(180_000);
    const { executors } = await apiClient.listExecutors();
    const dockerExec = executors.find((e) => e.type === "local_docker");
    expect(dockerExec?.id).toBeTruthy();
    const profile = await apiClient.createExecutorProfile(dockerExec!.id, {
      name: "E2E Docker Slow",
      config: { image_tag: E2E_IMAGE_TAG },
      prepare_script: "sleep 20",
      cleanup_script: "",
      env_vars: [],
    });
    const persistedProfile = await apiClient.getExecutorProfile(dockerExec!.id, profile.id);
    expect(persistedProfile.prepare_script).toBe("sleep 20");

    try {
      const task = await apiClient.createTask(seedData.workspaceId, "Docker Slow Progress", {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const launchPromise = apiClient.launchSession({
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: profile.id,
        workflow_step_id: seedData.startStepId,
        prompt: "/e2e:simple-message",
      });

      const panel = testPage.getByTestId("prepare-progress-panel");
      await expect(panel).toBeVisible({ timeout: 15_000 });
      await expect(panel).toHaveAttribute("data-status", "preparing");
      await expect(panel.getByTestId("prepare-progress-header-spinner")).toBeVisible();
      await expect(panel).toContainText("Waiting for Docker container");
      await expect(testPage.getByTestId("submit-message-button")).toBeDisabled({
        timeout: 15_000,
      });

      const launched = await launchPromise;
      await waitForSessionDone(
        apiClient,
        task.id,
        launched.session_id,
        "Waiting for slow Docker session",
      );
      await expect(panel).toHaveAttribute("data-status", "completed", { timeout: 30_000 });
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });

  test("archives a task and removes its Docker container", async ({ apiClient, seedData }) => {
    test.setTimeout(180_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker Archive Cleanup",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for archive cleanup session");
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env?.container_id).toBeTruthy();
    const containerID = env!.container_id!;

    try {
      await apiClient.archiveTask(task.id);
      await waitForDockerContainerRemoved(containerID, "Archived task should remove container");
    } finally {
      dockerRemove(containerID);
    }
  });

  test("deletes a task and removes its Docker container", async ({ apiClient, seedData }) => {
    test.setTimeout(180_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker Delete Cleanup",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for delete cleanup session");
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env?.container_id).toBeTruthy();
    const containerID = env!.container_id!;

    try {
      await apiClient.deleteTask(task.id);
      await waitForDockerContainerRemoved(containerID, "Deleted task should remove container");
    } finally {
      dockerRemove(containerID);
    }
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

  test("page refresh after an external stop resumes the same Docker container", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(180_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker Refresh Reuse",
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

    dockerStop(before!.container_id!);
    await expect
      .poll(() => dockerState(before!.container_id!), {
        timeout: 10_000,
        message: "Waiting for external Docker stop",
      })
      .toBe("exited");

    await testPage.reload();
    await session.waitForLoad();

    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.container_id, {
        timeout: 30_000,
        message: "Refresh resume must keep the original task container",
      })
      .toBe(before!.container_id);
    await expect
      .poll(() => dockerState(before!.container_id!), {
        timeout: 60_000,
        message: "Waiting for refresh resume to restart the original container",
      })
      .toBe("running");

    await session.clickTab("Terminal");
    await session.expectTerminalConnected(30_000);
    await session.typeInTerminal("printf terminal-after-refresh");
    await session.expectTerminalHasText("terminal-after-refresh");
  });

  test("reset environment from executor settings popover removes the Docker container", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(180_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker Popover Reset",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for reset session");
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env?.container_id).toBeTruthy();
    const containerID = env!.container_id!;

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await testPage.getByTestId("executor-settings-button").click();
    const popover = testPage.getByTestId("executor-settings-popover");
    await expect(popover).toBeVisible({ timeout: 5_000 });
    await testPage.getByTestId("executor-settings-reset").click();
    await testPage.getByLabel("I understand any uncommitted changes will be lost.").click();
    await testPage.getByTestId("reset-env-confirm").click();

    await waitForDockerContainerRemoved(containerID, "Reset should remove Docker container");
    await expect
      .poll(async () => await apiClient.getTaskEnvironment(task.id), {
        timeout: 15_000,
        message: "Waiting for Docker environment row to be reset",
      })
      .toBeNull();
  });

  test("multiple sessions on the same task share one task environment", async ({
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

    const launched = await apiClient.launchSession({
      task_id: task.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.dockerExecutorProfileId,
      prompt: "/e2e:simple-message",
      auto_start: true,
    });

    await waitForSessionEnvironment(apiClient, {
      taskId: task.id,
      sessionId: launched.session_id,
      expectedEnvironmentId: before!.id,
      message: "Waiting for second Docker session to reuse the task environment",
    });

    const after = await apiClient.getTaskEnvironment(task.id);
    expect(after?.id).toBe(before!.id);
    expect(after?.container_id).toBe(before!.container_id);

    // The durable contract for a launched session: it is bound to the same
    // task_environment_id rather than creating its own container-scoped env.
    // The list can also include unlaunched CREATED sessions from workflow
    // automation; those legitimately have no task_environment_id yet.
    const { sessions } = await apiClient.listTaskSessions(task.id);
    const launchedSession = sessions.find((s) => s.id === launched.session_id);
    expect(launchedSession?.task_environment_id).toBe(before!.id);
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
