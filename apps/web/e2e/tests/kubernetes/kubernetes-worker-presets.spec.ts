import {
  expect,
  kubernetesExecutorConfig,
  kubernetesProfileConfig,
  test,
} from "../../fixtures/kubernetes-test-base";
import {
  kubernetesWorkerPreset,
  type KubernetesWorkerTarget,
} from "../../fixtures/kubernetes-worker-images";
import {
  execInKubernetesPod,
  waitForKubernetesPod,
  waitForKubernetesPVC,
  waitForKubernetesResourceAbsent,
  waitForTaskSessionState,
} from "../../helpers/kubernetes";
import { waitForAgentMessage } from "../../helpers/session";

const WORKER_TARGETS: Array<{
  target: KubernetesWorkerTarget;
  imageKey: "minimal" | "nodePnpm" | "python";
}> = [
  { target: "minimal", imageKey: "minimal" },
  { target: "node-pnpm", imageKey: "nodePnpm" },
  { target: "python", imageKey: "python" },
];

test("runs every prepared worker image through the Kubernetes lifecycle", async ({
  apiClient,
  cluster,
  seedData,
  workerImages,
}) => {
  test.setTimeout(1_200_000);

  for (const { target, imageKey } of WORKER_TARGETS) {
    await test.step(target, async () => {
      const profileConfig = kubernetesProfileConfig(cluster, {
        pod_template_yaml: kubernetesWorkerPreset(target, workerImages[imageKey]),
        "workspace.mode": "managed_pvc",
        "workspace.size": "1Gi",
        "workspace.access_modes": JSON.stringify(["ReadWriteOnce"]),
      });
      const diagnostic = await apiClient.testKubernetesConnection({
        config: kubernetesExecutorConfig(cluster),
        profile_config: profileConfig,
      });
      expect(diagnostic.success, JSON.stringify(diagnostic)).toBe(true);

      const profile = await apiClient.createExecutorProfile(seedData.executorId, {
        name: "E2E worker preset " + target,
        config: profileConfig,
        prepare_script: "",
        cleanup_script: "",
        env_vars: [],
      });
      let task: { id: string; session_id?: string } | undefined;
      let archived = false;
      try {
        task = await apiClient.createTaskWithAgent(
          seedData.workspaceId,
          "Kubernetes worker preset " + target,
          seedData.agentProfileId,
          {
            description: 'e2e:message("started")\ne2e:delay(60000)',
            workflow_id: seedData.workflowId,
            workflow_step_id: seedData.startStepId,
            executor_id: seedData.executorId,
            executor_profile_id: profile.id,
          },
        );
        expect(task.session_id).toBeTruthy();
        const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
        const claim = await waitForKubernetesPVC(cluster, task.id, task.session_id!);
        expect(execInKubernetesPod(cluster, pod.metadata.name, ["id", "-u"])).toBe("1000");
        const smokeCommands = [
          "mkdir -p /workspace/" + target + "-git",
          "git -C /workspace/" + target + "-git init",
          "git -C /workspace/" + target + "-git config user.email smoke@example.invalid",
          "git -C /workspace/" + target + "-git config user.name kandev-smoke",
          "printf '%s\\n' '" + target + "' > /workspace/" + target + "-git/README.md",
          "git -C /workspace/" + target + "-git add README.md",
          "git -C /workspace/" + target + "-git commit -m smoke",
          "printf retained > /workspace/kandev-worker-" + target + "-retained",
        ].join("\n");
        execInKubernetesPod(cluster, pod.metadata.name, ["/bin/sh", "-ceu", smokeCommands]);
        await waitForAgentMessage(apiClient, task.session_id!, "started");

        if (target === "node-pnpm") {
          expect(execInKubernetesPod(cluster, pod.metadata.name, ["pnpm", "--version"])).toMatch(
            /^9\.15\.9/,
          );
          expect(execInKubernetesPod(cluster, pod.metadata.name, ["node", "--version"])).toMatch(
            /^v\d+\./,
          );
          const nodePnpmSmoke = [
            "set -eu",
            "package=/workspace/kandev-npm-global-smoke",
            'mkdir -p "$package" /workspace/.npm-global /workspace/.pnpm /workspace/.pnpm-store',
            'printf \'%s\\n\' \'{"name":"kandev-npm-global-smoke","version":"1.0.0","bin":{"kandev-npm-global-smoke":"cli.js"}}\' > "$package/package.json"',
            "printf '%s\\n' '#!/bin/sh' 'printf npm-global-ok' > \"$package/cli.js\"",
            'chmod +x "$package/cli.js"',
            'npm install --global --offline --ignore-scripts "$package" >/dev/null',
            "test -x /workspace/.npm-global/bin/kandev-npm-global-smoke",
            'test "$(/workspace/.npm-global/bin/kandev-npm-global-smoke)" = npm-global-ok',
            "test -w /workspace/.pnpm",
            "test -w /workspace/.pnpm-store",
            "touch /workspace/.pnpm/.write-test /workspace/.pnpm-store/.write-test",
            'rm -rf "$package" /workspace/.npm-global/lib/node_modules/kandev-npm-global-smoke /workspace/.npm-global/bin/kandev-npm-global-smoke /workspace/.pnpm/.write-test /workspace/.pnpm-store/.write-test',
          ].join(" && ");
          expect(
            execInKubernetesPod(cluster, pod.metadata.name, ["/bin/sh", "-ceu", nodePnpmSmoke]),
          ).toBe("");
        } else if (target === "python") {
          const pythonSmoke = [
            "python3 -m venv /workspace/kandev-python-venv",
            "/workspace/kandev-python-venv/bin/python -c 'import sys; assert sys.version_info.major == 3'",
          ].join(" && ");
          expect(
            execInKubernetesPod(cluster, pod.metadata.name, ["/bin/sh", "-ceu", pythonSmoke]),
          ).toBe("");
        } else {
          expect(execInKubernetesPod(cluster, pod.metadata.name, ["git", "--version"])).toMatch(
            /^git version /,
          );
        }

        expect(
          (await apiClient.listKubernetesSessions(seedData.executorId)).find(
            (row) => row.task_id === task!.id,
          ),
        ).toMatchObject({ retention_state: "active" });
        await apiClient.stopSession({ session_id: task.session_id! });
        await waitForTaskSessionState(apiClient, task.id, task.session_id!, "CANCELLED");
        await expect
          .poll(
            async () =>
              (await apiClient.listKubernetesSessions(seedData.executorId)).find(
                (row) => row.task_id === task!.id,
              ),
            { timeout: 60_000, message: "Waiting for " + target + " retention projection" },
          )
          .toMatchObject({ session_state: "CANCELLED", retention_state: "retained" });

        const resumed = await apiClient.launchSession(
          { task_id: task.id, intent: "resume", session_id: task.session_id! },
          90_000,
        );
        expect(resumed.session_id).toBe(task.session_id);
        await waitForTaskSessionState(
          apiClient,
          task.id,
          task.session_id!,
          "WAITING_FOR_INPUT",
          90_000,
        );
        const after = await waitForKubernetesPod(cluster, task.id, task.session_id!);
        const afterClaim = await waitForKubernetesPVC(cluster, task.id, task.session_id!);
        expect(after.metadata.uid).toBe(pod.metadata.uid);
        expect(afterClaim.metadata.uid).toBe(claim.metadata.uid);
        expect(
          execInKubernetesPod(cluster, after.metadata.name, [
            "cat",
            "/workspace/kandev-worker-" + target + "-retained",
          ]),
        ).toBe("retained");
        await expect
          .poll(
            async () =>
              (await apiClient.listKubernetesSessions(seedData.executorId)).find(
                (row) => row.task_id === task!.id,
              ),
            { timeout: 90_000, message: "Waiting for " + target + " active projection" },
          )
          .toMatchObject({ session_state: "WAITING_FOR_INPUT", retention_state: "active" });

        await apiClient.archiveTask(task.id);
        archived = true;
        await waitForKubernetesResourceAbsent(cluster, "pod", pod.metadata.name);
        await waitForKubernetesResourceAbsent(
          cluster,
          "persistentvolumeclaim",
          claim.metadata.name,
        );
      } finally {
        if (task && !archived) await apiClient.archiveTask(task.id).catch(() => undefined);
        await apiClient.deleteExecutorProfile(profile.id).catch(() => undefined);
      }
    });
  }
});
