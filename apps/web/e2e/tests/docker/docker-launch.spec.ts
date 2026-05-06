import { test, expect } from "../../fixtures/docker-test-base";
import { E2E_IMAGE_TAG } from "../../fixtures/docker-probe";
import { SessionPage } from "../../pages/session-page";
import {
  dockerCurrentBranch,
  dockerInspectExists,
  dockerRemove,
  dockerState,
  dockerStop,
  waitForDockerContainerRemoved,
} from "../../helpers/docker";
import {
  waitForLatestSessionDone,
  waitForSessionDone,
  waitForSessionEnvironment,
} from "../../helpers/session";

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

  // Regression: a stored prepare_script that pre-dates the worktree-branch
  // checkout block must STILL land the agent on the kandev-managed feature
  // branch. Until kandev moved the checkout into a non-overridable postlude,
  // any profile created in the UI snapshotted the then-current default into
  // its prepare_script field — and older snapshots clone main and stop. The
  // empty-script e2e fixture papered over this because it falls through to
  // the runtime default; this test forces a real stored script.
  test("custom prepare_script without worktree checkout still lands on feature branch", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const { executors } = await apiClient.listExecutors();
    const dockerExec = executors.find((e) => e.type === "local_docker");
    expect(dockerExec?.id).toBeTruthy();

    // Older Docker default kandev shipped: clone the chosen branch, run the
    // user's repo setup, no kandev-managed feature-branch checkout. Any user
    // who created a profile under that default has this stored verbatim. The
    // safe.directory line is added so this test runs cleanly against the
    // e2e file:// remote which has a UID-mismatched checkout; in production
    // the user's actual stale script may or may not include it.
    const staleScript = `#!/bin/sh
set -eu
{{git.identity_setup}}
git config --global --add safe.directory '*'
git config --global url."https://github.com/".insteadOf "git@github.com:"
git config --global url."https://github.com/".insteadOf "ssh://git@github.com/"
{{github.auth_setup}}
git clone --depth=1 --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}
cd {{workspace.path}}
git remote set-url origin "$(git remote get-url origin | sed 's|https://[^@]*@github.com/|https://github.com/|')" 2>/dev/null || true
{{repository.setup_script}}
`;

    const profile = await apiClient.createExecutorProfile(dockerExec!.id, {
      name: "E2E Docker Stale Script",
      config: { image_tag: E2E_IMAGE_TAG },
      prepare_script: staleScript,
      cleanup_script: "",
      env_vars: [],
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Docker Stale Prep",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profile.id,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for stale-prep session");

      const env = await apiClient.getTaskEnvironment(task.id);
      expect(env?.container_id).toBeTruthy();
      // Even with a stored script that doesn't include the checkout block,
      // kandev must end up on the feature branch — the kandev-managed
      // checkout is an invariant, not something the user can drop.
      expect(dockerCurrentBranch(env!.container_id!)).toMatch(
        /^feature\/docker-stale-prep-[0-9a-f]{6}$/,
      );
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });

  test("stops the agent then resumes onto the same container without re-cloning", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Docker Stop Resume",
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
    const containerID = before!.container_id!;
    const branchBefore = dockerCurrentBranch(containerID);
    expect(branchBefore).toMatch(/^feature\/docker-stop-resume-[0-9a-f]{6}$/);

    const { sessions: sessionsBefore } = await apiClient.listTaskSessions(task.id);
    const firstSession = sessionsBefore[0];
    expect(firstSession?.id).toBeTruthy();

    // Stop via the same WS path the UI uses.
    await apiClient.stopSession({
      session_id: firstSession!.id,
      reason: "e2e stop",
      force: false,
    });
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.find((s) => s.id === firstSession!.id)?.state;
        },
        { timeout: 30_000, message: "Waiting for first session to reach CANCELLED" },
      )
      .toBe("CANCELLED");

    // Resume by launching a new session on the same task — the UI Resume
    // button takes this path (it just creates a new session bound to the
    // same task environment).
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
      "Waiting for resumed Docker session",
    );

    const after = await apiClient.getTaskEnvironment(task.id);
    expect(after?.id, "task environment is reused on resume").toBe(before!.id);
    expect(after?.container_id, "container is reused on resume").toBe(containerID);

    // Container is still alive and on the same kandev feature branch — no
    // re-clone, no re-checkout to base.
    expect(dockerInspectExists(containerID)).toBe(true);
    expect(dockerCurrentBranch(containerID), "feature branch is preserved").toBe(branchBefore);
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
