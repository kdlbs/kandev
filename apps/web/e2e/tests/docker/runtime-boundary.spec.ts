import fs from "node:fs";
import { spawnSync } from "node:child_process";

import { test, expect } from "../../fixtures/docker-test-base";
import {
  dockerContainerLabels,
  dockerFindContainerByTaskID,
  dockerInspectExists,
  dockerRemove,
  dockerState,
  dockerTopCommandLines,
} from "../../helpers/docker";
import { waitForLatestSessionDone } from "../../helpers/session";

function pidNamespace(pid: number): string | null {
  try {
    return fs.readlinkSync(`/proc/${pid}/ns/pid`);
  } catch {
    return null;
  }
}

test.describe("Docker runtime boundary", () => {
  test("records a daemon-verifiable worker and runs the mock agent inside it", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Docker runtime boundary ${Date.now()}`,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );

    let containerID = "";
    try {
      await waitForLatestSessionDone(
        apiClient,
        task.id,
        1,
        "Waiting for Docker runtime boundary session",
      );

      const environment = await apiClient.getTaskEnvironment(task.id);
      expect(environment?.executor_type).toBe("local_docker");
      expect(
        environment?.container_id,
        "environment must record the runner container",
      ).toBeTruthy();

      const { sessions } = await apiClient.listTaskSessions(task.id);
      expect(sessions).toHaveLength(1);
      const { session } = await apiClient.getTaskSession(sessions[0].id);
      expect(session.execution_runtime).toBe("docker");
      expect(session.container_id).toBe(environment!.container_id);
      expect(session.agent_execution_id).toBeTruthy();

      containerID = session.container_id!;
      expect(dockerInspectExists(containerID), "worker container must exist on the daemon").toBe(
        true,
      );
      expect(dockerState(containerID)).not.toBe("missing");

      const labels = dockerContainerLabels(containerID);
      expect(labels?.kandev_task_id ?? labels?.["kandev.task_id"]).toBe(task.id);
      expect(labels?.kandev_session_id ?? labels?.["kandev.session_id"]).toBe(session.id);

      const processes = dockerTopCommandLines(containerID);
      expect(
        processes.some((line) => /mock-agent/.test(line)),
        `worker container must own the bounded mock agent; processes: ${processes.join(" | ")}`,
      ).toBe(true);

      // The bounded mock agent must live in the worker's PID namespace, not
      // the backend process's namespace. A hostname or filesystem marker is
      // not accepted as identity evidence.
      const backendPid = backend.pid();
      expect(backendPid, "the backend process must be observable").toBeTruthy();
      const inspectPid = spawnSync("docker", ["inspect", "-f", "{{.State.Pid}}", containerID], {
        encoding: "utf8",
      });
      const containerPid = Number(inspectPid.stdout.trim());
      expect(containerPid, "worker container init PID must be observable").toBeGreaterThan(0);
      expect(processes.map((line) => line.split(/\s+/, 1)[0])).not.toContain(String(backendPid));
      // Docker Desktop runs the daemon in a VM, so the inspected container PID
      // is a VM process that has no host /proc entry. Compare namespaces only
      // when both processes are visible in this kernel.
      const backendNamespace = pidNamespace(backendPid);
      const workerNamespace = pidNamespace(containerPid);
      if (backendNamespace && workerNamespace) {
        expect(workerNamespace).not.toBe(backendNamespace);
      }

      // The runner identity is not the backend's own service identity: the
      // session/container pair belongs to exactly this task and exposes the
      // daemon's full container ID.
      expect(containerID.length).toBeGreaterThan(12);
    } finally {
      if (containerID) {
        dockerRemove(containerID);
      }
    }
  });

  test("an unreachable registered daemon fails without a host worker", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    // Keep the Docker backend registered but point it at a closed loopback
    // port. This exercises a provider failure after registration, not a
    // missing-backend refusal.
    await backend.restart({ KANDEV_DOCKER_HOST: "tcp://127.0.0.1:1" });

    let profileID = "";
    try {
      const { executors } = await apiClient.listExecutors();
      const docker = executors.find((executor) => executor.type === "local_docker");
      expect(docker, "the Local Docker executor row must exist").toBeTruthy();
      const profile = await apiClient.createExecutorProfile(docker!.id, {
        name: `Unreachable Docker ${Date.now()}`,
        config: { image_tag: "kandev/e2e-unreachable-docker:latest" },
        prepare_script: "",
        cleanup_script: "",
        env_vars: [],
      });
      profileID = profile.id;

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        `Unreachable Docker ${Date.now()}`,
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          executor_profile_id: profile.id,
        },
      );

      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            return sessions[0]?.state ?? "none";
          },
          { timeout: 60_000, message: "waiting for the unreachable Docker launch to fail" },
        )
        .toBe("FAILED");

      const { sessions } = await apiClient.listTaskSessions(task.id);
      const { session } = await apiClient.getTaskSession(sessions[0].id);
      expect(
        session.execution_runtime ?? null,
        "no runtime is observed for a failed create",
      ).toBeNull();
      expect(session.agent_execution_id ?? "").toBe("");
      expect(session.container_id ?? "").toBe("");
      expect(dockerFindContainerByTaskID(task.id), "no daemon container may be created").toBeNull();
    } finally {
      if (profileID) {
        await apiClient.deleteExecutorProfile(profileID).catch(() => {});
      }
      await backend.restart();
    }
  });
});
