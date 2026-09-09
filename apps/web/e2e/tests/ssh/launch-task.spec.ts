import { test, expect } from "../../fixtures/ssh-test-base";
import {
  execInContainer,
  listRemoteDir,
  readRemoteFile,
  remotePathExists,
} from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";

/**
 * Full end-to-end task launch on the real sshd container. The smoke test
 * for the SSH executor: upload agentctl, mkdir the per-task dir, clone the
 * repo, launch the per-session agentctl, port-forward, run the agent to
 * completion, observe the on-remote filesystem layout.
 *
 * Covers e2e-plan.md group H (H1–H8).
 */
test.describe("ssh executor — task launch", () => {
  test("launches a session and records ssh runtime on the task environment", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H1 launch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );

    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for first SSH session");

    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.executor_type).toBe("ssh");

    const row = (await apiClient.listSSHSessions(seedData.sshExecutorId)).find(
      (session) => session.task_id === task.id,
    );
    expect(row?.remote_task_dir).toBeTruthy();
    const workspace = row!.remote_task_dir!;
    expect(
      execInContainer(seedData.sshTarget, [
        "git",
        "-c",
        "safe.directory=*",
        "-C",
        workspace,
        "rev-parse",
        "--show-toplevel",
      ]).trim(),
    ).toBe(workspace);
    expect(
      execInContainer(seedData.sshTarget, [
        "git",
        "-c",
        "safe.directory=*",
        "-C",
        workspace,
        "branch",
        "--show-current",
      ]).trim(),
    ).not.toBe("main");
    expect(readRemoteFile(seedData.sshTarget, `${workspace}/remote-source.txt`)).toBe(
      "e2e-ssh fixture\n",
    );
  });

  test("runs custom prepare and terminal cleanup hooks on the remote workspace", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const profile = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
      name: "E2E SSH hooks",
      config: {},
      prepare_script: `#!/bin/bash
set -euo pipefail
workspace={{workspace.path}}
repository_url={{repository.clone_url}}
repository_branch={{repository.branch}}
git init -q "$workspace"
git -C "$workspace" remote add origin "$repository_url"
git -C "$workspace" fetch --no-tags origin "+refs/heads/$repository_branch:refs/remotes/origin/$repository_branch"
git -C "$workspace" checkout -b "$repository_branch" "origin/$repository_branch"
printf 'prepared\\n' > "$workspace/custom-prepare-marker"
`,
      cleanup_script: "printf 'cleaned\\n' > {{workspace.path}}/custom-cleanup-marker",
      env_vars: [],
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "SSH custom hooks",
        seedData.agentProfileId,
        {
          description: 'e2e:delay(30000)\ne2e:message("still running")',
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profile.id,
        },
      );
      let workspace = "";
      await expect
        .poll(
          async () => {
            const row = (await apiClient.listSSHSessions(seedData.sshExecutorId)).find(
              (session) => session.task_id === task.id,
            );
            workspace = row?.remote_task_dir ?? "";
            return (
              workspace !== "" &&
              (row?.remote_agentctl_port ?? 0) > 0 &&
              (await apiClient.getTask(task.id)).state === "IN_PROGRESS" &&
              readRemoteFile(seedData.sshTarget, `${workspace}/custom-prepare-marker`) ===
                "prepared\n"
            );
          },
          {
            timeout: 60_000,
            message: "Wait for custom SSH prepare hook",
          },
        )
        .toBe(true);
      expect(remotePathExists(seedData.sshTarget, `${workspace}/custom-cleanup-marker`)).toBe(
        false,
      );

      await apiClient.archiveTask(task.id);
      await expect
        .poll(() => remotePathExists(seedData.sshTarget, `${workspace}/custom-cleanup-marker`), {
          timeout: 30_000,
          message: "Wait for terminal SSH cleanup hook",
        })
        .toBe(true);
      expect(remotePathExists(seedData.sshTarget, workspace)).toBe(true);
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => undefined);
    }
  });

  test("agentctl is uploaded on first launch and sha256 sidecar lands", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H2 upload agentctl",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for upload");

    expect(remotePathExists(seedData.sshTarget, "/home/kandev/.kandev/bin/agentctl")).toBe(true);
    expect(remotePathExists(seedData.sshTarget, "/home/kandev/.kandev/bin/agentctl.sha256")).toBe(
      true,
    );
    const sha = readRemoteFile(seedData.sshTarget, "/home/kandev/.kandev/bin/agentctl.sha256");
    expect(sha.trim()).toMatch(/^[0-9a-f]{64}$/);
  });

  test("second launch on the same host skips the upload (sha matches)", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    // First launch primes the cache.
    const first = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H3 first launch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, first.id, 1, "First launch");

    // Inspect mtime BEFORE the second launch.
    const beforeMtime = readRemoteFile(
      seedData.sshTarget,
      "/home/kandev/.kandev/bin/agentctl.sha256",
    );

    const second = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H3 second launch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, second.id, 1, "Second launch");

    // The sha256 file should be byte-identical (same hash, content reused).
    const afterMtime = readRemoteFile(
      seedData.sshTarget,
      "/home/kandev/.kandev/bin/agentctl.sha256",
    );
    expect(afterMtime).toBe(beforeMtime);
  });

  test("per-task workdir lives at <workdir>/tasks/<task-dir>/", async ({ apiClient, seedData }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H4 task workdir",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for workdir");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row?.remote_task_dir).toMatch(/\/tasks\/[^/]+$/);
    expect(remotePathExists(seedData.sshTarget, row!.remote_task_dir!)).toBe(true);
  });

  test("per-session runtime dir holds the agentctl pid and port files", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H5 session runtime",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for session runtime");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row).toBeDefined();

    const sessionDir = `${row!.remote_task_dir}/.kandev/sessions/${row!.session_id}`;
    expect(remotePathExists(seedData.sshTarget, sessionDir)).toBe(true);
    const entries = listRemoteDir(seedData.sshTarget, sessionDir);
    // The wrapper writes the pid file and the log; the port travels via the
    // AGENTCTL_PORT env var and the ExecutorRunning metadata, not a file.
    expect(entries).toEqual(expect.arrayContaining(["agentctl.pid", "agentctl.log"]));
    expect(row!.remote_agentctl_port).toBeGreaterThan(0);
  });

  test("uploads path-mode attachments into the remote executor workspace", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const content = "remote attachment bytes";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H6 attachment upload",
      seedData.agentProfileId,
      {
        description: "Inspect the attached file.",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
        attachments: [
          {
            type: "resource",
            data: Buffer.from(content, "utf8").toString("base64"),
            mime_type: "text/plain",
            name: "remote-note.txt",
            delivery_mode: "path",
          },
        ],
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for attachment upload");

    await expect
      .poll(
        async () => {
          const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
          return sessions.find((s) => s.task_id === task.id)?.remote_task_dir ?? "";
        },
        { timeout: 60_000, message: "Wait for SSH session row" },
      )
      .not.toBe("");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row?.remote_task_dir).toBeTruthy();
    const savedPath = execInContainer(seedData.sshTarget, [
      "sh",
      "-c",
      `find ${JSON.stringify(row!.remote_task_dir)}/.kandev/attachments -name remote-note.txt -type f -print -quit`,
    ]).trim();
    expect(savedPath).toContain("/.kandev/attachments/");
    expect(readRemoteFile(seedData.sshTarget, savedPath)).toBe(content);
  });

  test("uploads path-mode attachments queued while the remote task is running", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const content = "remote follow-up attachment bytes";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H6 running attachment upload",
      seedData.agentProfileId,
      {
        description: "/slow 20s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await apiClient.queueMessage(task.id, task.session_id, "Inspect the follow-up file.", [
      {
        type: "resource",
        data: Buffer.from(content, "utf8").toString("base64"),
        mime_type: "text/plain",
        name: "remote-followup.txt",
        delivery_mode: "path",
      },
    ]);

    await expect
      .poll(
        async () => {
          const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
          return sessions.find((s) => s.task_id === task.id)?.remote_task_dir ?? "";
        },
        { timeout: 60_000, message: "Wait for SSH session row" },
      )
      .not.toBe("");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    const taskDir = row?.remote_task_dir;
    expect(taskDir).toBeTruthy();

    const findAttachment = () =>
      execInContainer(seedData.sshTarget, [
        "sh",
        "-c",
        `find ${JSON.stringify(taskDir)}/.kandev/attachments -name remote-followup.txt -type f -print -quit 2>/dev/null || true`,
      ]).trim();

    await expect
      .poll(findAttachment, {
        timeout: 120_000,
        message: "Wait for queued attachment to upload remotely",
      })
      .not.toBe("");

    const attachmentPath = findAttachment();
    expect(attachmentPath).toContain("/.kandev/attachments/");
    expect(readRemoteFile(seedData.sshTarget, attachmentPath)).toBe(content);
  });

  test("stopping the session cleans up the session runtime dir but leaves the task dir", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H7 stop cleanup",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait before stop");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row).toBeDefined();
    const sessionDir = `${row!.remote_task_dir}/.kandev/sessions/${row!.session_id}`;

    // Archive (or otherwise end) the task to trigger StopInstance.
    await apiClient.archiveTask(task.id);

    await expect
      .poll(() => remotePathExists(seedData.sshTarget, sessionDir), { timeout: 30_000 })
      .toBe(false);
    // Task dir intact — v1 spec, no auto-clean on last session stop.
    expect(remotePathExists(seedData.sshTarget, row!.remote_task_dir!)).toBe(true);
  });
});
