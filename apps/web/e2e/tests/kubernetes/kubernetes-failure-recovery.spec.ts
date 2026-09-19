import fs from "node:fs";
import { test, expect, kubernetesProfileConfig } from "../../fixtures/kubernetes-test-base";
import {
  execInKubernetesPod,
  waitForKubernetesPod,
  waitForKubernetesPVC,
  waitForKubernetesResourceAbsent,
  waitForTaskSessionState,
} from "../../helpers/kubernetes";
import { waitForAgentMessage, waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.1 through .4
for (const restart of [false, true]) {
  const title = restart
    ? "preserves Kubernetes workspace after recoverable agent error and backend restart"
    : "preserves Kubernetes workspace after recoverable agent error";
  test(title, async ({ apiClient, seedData, cluster, backend, testPage }) => {
    test.setTimeout(360_000);
    const profile = await apiClient.createExecutorProfile(seedData.executorId, {
      name: "Recoverable failure managed workspace",
      config: kubernetesProfileConfig(cluster, {
        "workspace.mode": "managed_pvc",
        "workspace.size": "1Gi",
        "workspace.access_modes": JSON.stringify(["ReadWriteOnce"]),
      }),
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });
    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Kubernetes failure recovery",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          executor_id: seedData.executorId,
          executor_profile_id: profile.id,
        },
      );
      const sessionId = task.session_id!;
      expect(sessionId).toBeTruthy();
      await waitForSessionDone(apiClient, task.id, sessionId, "Initial Kubernetes turn");
      const pod = await waitForKubernetesPod(cluster, task.id, sessionId);
      const claim = await waitForKubernetesPVC(cluster, task.id, sessionId);
      execInKubernetesPod(cluster, pod.metadata.name, [
        "/bin/sh",
        "-c",
        "printf retained > /workspace/recovery-sentinel",
      ]);
      const session = new SessionPage(testPage);
      const logOffset = fs.readFileSync(backend.logPath, "utf8").length;
      await apiClient.addUserMessage(task.id, sessionId, "/transport-lost");
      await waitForTaskSessionState(apiClient, task.id, sessionId, "WAITING_FOR_INPUT");
      await expect
        .poll(
          () =>
            fs
              .readFileSync(backend.logPath, "utf8")
              .slice(logOffset)
              .split("\n")
              .some(
                (line) =>
                  line.includes(task.id) &&
                  line.includes("agent stopped and removed from tracking"),
              ),
          {
            timeout: 30_000,
            message: "Waiting for failed execution cleanup before checking retention",
          },
        )
        .toBe(true);
      expect((await waitForKubernetesPod(cluster, task.id, sessionId)).metadata.uid).toBe(
        pod.metadata.uid,
      );
      expect((await waitForKubernetesPVC(cluster, task.id, sessionId)).metadata.uid).toBe(
        claim.metadata.uid,
      );
      if (restart) await backend.restart();
      await testPage.goto(`/t/${task.id}`);
      await session.waitForLoad();
      await session.recoveryResumeButton().click();
      await waitForTaskSessionState(apiClient, task.id, sessionId, "WAITING_FOR_INPUT", 90_000);
      const reply = restart ? "recovered-after-restart" : "recovered-after-failure";
      await apiClient.addUserMessage(task.id, sessionId, `e2e:message("${reply}")`);
      await waitForAgentMessage(apiClient, sessionId, reply);
      await waitForSessionDone(apiClient, task.id, sessionId, "Recovered Kubernetes turn");
      expect((await waitForKubernetesPod(cluster, task.id, sessionId)).metadata.uid).toBe(
        pod.metadata.uid,
      );
      expect(
        execInKubernetesPod(cluster, pod.metadata.name, ["cat", "/workspace/recovery-sentinel"]),
      ).toBe("retained");
      await apiClient.archiveTask(task.id);
      await waitForKubernetesResourceAbsent(cluster, "pod", pod.metadata.name);
      await waitForKubernetesResourceAbsent(cluster, "persistentvolumeclaim", claim.metadata.name);
    } finally {
      await apiClient.deleteExecutorProfile(profile.id);
    }
  });
}
