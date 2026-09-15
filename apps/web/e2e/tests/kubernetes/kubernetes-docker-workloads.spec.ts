import { execFileSync } from "node:child_process";
import fs from "node:fs";
import { randomUUID } from "node:crypto";
import path from "node:path";
import {
  test,
  expect,
  fullWorkerTemplate,
  fullWorkerPrepare,
} from "../../fixtures/kubernetes-docker-test-base";
import { kubernetesProfileConfig } from "../../fixtures/kubernetes-test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { KubernetesCluster } from "../../fixtures/kubernetes-tools";
import {
  waitForKubernetesPod,
  waitForKubernetesPVC,
  waitForKubernetesResourceAbsent,
  waitForTaskSessionState,
  waitForFailedSession,
} from "../../helpers/kubernetes";
import { SessionPage } from "../../pages/session-page";

// A skipped opt-in test is never Docker acceptance evidence.
test.skip(
  !process.env.KANDEV_E2E_FULL_WORKER_IMAGE,
  "Requires an explicitly built full worker image ID",
);

function run(cluster: KubernetesCluster, pod: string, script: string, container = "kandev-agent") {
  return cluster
    .kubectl(["-n", cluster.namespace, "exec", pod, "-c", container, "--", "sh", "-ceu", script], {
      timeoutMs: 600_000,
    })
    .trim();
}

async function archive(
  api: ApiClient,
  cluster: KubernetesCluster,
  task: { id: string },
  pod?: string,
  claim?: string,
) {
  await api.archiveTask(task.id);
  if (pod) await waitForKubernetesResourceAbsent(cluster, "pod", pod);
  if (claim) await waitForKubernetesResourceAbsent(cluster, "persistentvolumeclaim", claim);
}

test("full worker returns source and Docker results", async ({
  apiClient,
  cluster,
  seedData,
  fullWorkerImage,
  testPage,
}, testInfo) => {
  test.setTimeout(900_000);
  const profile = await apiClient.createExecutorProfile(seedData.executorId, {
    name: "Full worker source acceptance",
    config: kubernetesProfileConfig(cluster, {
      pod_template_yaml: fullWorkerTemplate(fullWorkerImage),
      "workspace.mode": "managed_pvc",
      "workspace.size": "2Gi",
      "workspace.access_modes": '["ReadWriteOnce"]',
    }),
    prepare_script: fullWorkerPrepare(),
    cleanup_script: "",
    env_vars: [],
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Full worker source results",
    seedData.agentProfileId,
    {
      description: 'e2e:message("ready")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      executor_id: seedData.executorId,
      executor_profile_id: profile.id,
    },
  );
  let podName: string | undefined;
  let claimName: string | undefined;
  try {
    const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    podName = pod.metadata.name;
    const claim = await waitForKubernetesPVC(cluster, task.id, task.session_id!);
    claimName = claim.metadata.name;
    await waitForTaskSessionState(apiClient, task.id, task.session_id!, "WAITING_FOR_INPUT");
    const output = run(cluster, podName, "bash /opt/full-worker/smoke.sh --all");
    expect(output).toContain("source-browser-docker-compose-ok");
    expect(run(cluster, podName, "id -u")).toBe("1000");
    expect(
      run(
        cluster,
        podName,
        "test -S /run/docker/docker.sock; cat /workspace/full-worker-result.txt",
      ),
    ).toBe("source-browser-docker-compose-ok");
    expect(run(cluster, podName, "cat /proc/1/cmdline", "docker-engine")).not.toContain("tcp://");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad(30_000);
    await session.clickTab("Terminal");
    await session.typeInTerminal("cat /workspace/full-worker-result.txt");
    await session.expectTerminalHasText("source-browser-docker-compose-ok");
    await testInfo.attach("full-worker-source-results", {
      body: output,
      contentType: "text/plain",
    });
  } finally {
    await archive(apiClient, cluster, task, podName, claimName);
  }
});

test("unavailable daemon fails preparation", async ({
  apiClient,
  cluster,
  seedData,
  fullWorkerImage,
  testPage,
}) => {
  test.setTimeout(240_000);
  const profile = await apiClient.createExecutorProfile(seedData.executorId, {
    name: "Unavailable Docker daemon",
    config: kubernetesProfileConfig(cluster, {
      pod_template_yaml: fullWorkerTemplate(fullWorkerImage, true),
    }),
    prepare_script: "export DOCKER_READY_TIMEOUT_SECONDS=2\n" + fullWorkerPrepare(),
    cleanup_script: "",
    env_vars: [],
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Docker readiness failure",
    seedData.agentProfileId,
    {
      description: 'e2e:message("must not start")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      executor_id: seedData.executorId,
      executor_profile_id: profile.id,
    },
  );
  try {
    await waitForFailedSession(apiClient, task.id, /Docker daemon did not become ready/, 90_000);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad(30_000);
    const error = session.activeChat().getByTestId("task-launch-error-entry");
    await expect(error).toBeVisible();
    await error.getByRole("button", { name: "Show details" }).click();
    await expect(error.getByTestId("task-launch-error-details")).toContainText(
      "Docker daemon did not become ready",
    );
  } finally {
    await archive(apiClient, cluster, task);
  }
});

test("resume and replacement retain workspace", async ({
  apiClient,
  cluster,
  seedData,
  fullWorkerImage,
}) => {
  test.setTimeout(480_000);
  const config = kubernetesProfileConfig(cluster, {
    pod_template_yaml: fullWorkerTemplate(fullWorkerImage),
    "workspace.mode": "managed_pvc",
    "workspace.size": "2Gi",
    "workspace.access_modes": '["ReadWriteOnce"]',
  });
  const profile = await apiClient.createExecutorProfile(seedData.executorId, {
    name: "Full worker retention",
    config,
    prepare_script: fullWorkerPrepare(),
    cleanup_script: "",
    env_vars: [],
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Docker workspace retention",
    seedData.agentProfileId,
    {
      description: 'e2e:message("ready")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      executor_id: seedData.executorId,
      executor_profile_id: profile.id,
    },
  );
  let podName: string | undefined;
  let claimName: string | undefined;
  try {
    const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    podName = pod.metadata.name;
    const claim = await waitForKubernetesPVC(cluster, task.id, task.session_id!);
    claimName = claim.metadata.name;
    await waitForTaskSessionState(apiClient, task.id, task.session_id!, "WAITING_FOR_INPUT");
    const daemonID = run(cluster, podName, "docker info --format '{{.ID}}'");
    run(
      cluster,
      podName,
      "printf uncommitted-result > /workspace/retained-result.txt; docker volume create retained-daemon-state",
    );
    await apiClient.stopSession({ session_id: task.session_id! });
    await waitForTaskSessionState(apiClient, task.id, task.session_id!, "CANCELLED");
    expect((await waitForKubernetesPod(cluster, task.id, task.session_id!)).metadata.uid).toBe(
      pod.metadata.uid,
    );
    expect(run(cluster, podName, "docker info --format '{{.ID}}'")).toBe(daemonID);
    await apiClient.launchSession(
      { task_id: task.id, intent: "resume", session_id: task.session_id! },
      90_000,
    );
    await waitForTaskSessionState(apiClient, task.id, task.session_id!, "WAITING_FOR_INPUT");
    expect(run(cluster, podName, "cat /workspace/retained-result.txt")).toBe("uncommitted-result");
    await apiClient.stopSession({ session_id: task.session_id! });
    await waitForTaskSessionState(apiClient, task.id, task.session_id!, "CANCELLED");
    await apiClient.updateExecutorProfile(seedData.executorId, profile.id, {
      config: { ...config, pod_template_yaml: cluster.podTemplate() },
    });
    cluster.kubectl(["-n", cluster.namespace, "delete", "pod", podName, "--wait=true"]);
    await apiClient.launchSession(
      { task_id: task.id, intent: "resume", session_id: task.session_id! },
      90_000,
    );
    await waitForTaskSessionState(apiClient, task.id, task.session_id!, "WAITING_FOR_INPUT");
    const replacement = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    podName = replacement.metadata.name;
    expect(replacement.metadata.uid).not.toBe(pod.metadata.uid);
    expect(replacement.spec?.containers?.map((c) => c.name)).toContain("docker-engine");
    expect((await waitForKubernetesPVC(cluster, task.id, task.session_id!)).metadata.uid).toBe(
      claim.metadata.uid,
    );
    expect(run(cluster, podName, "cat /workspace/retained-result.txt")).toBe("uncommitted-result");
    expect(run(cluster, podName, "docker info --format '{{.ID}}'")).not.toBe(daemonID);
    expect(run(cluster, podName, "docker volume ls --format '{{.Name}}'")).not.toContain(
      "retained-daemon-state",
    );
  } finally {
    await archive(apiClient, cluster, task, podName, claimName);
  }
});

test("isolated daemons clean up with owned Pods", async ({
  apiClient,
  cluster,
  seedData,
  fullWorkerImage,
}, testInfo) => {
  test.setTimeout(600_000);
  const tasks: Array<{ id: string; session_id?: string }> = [];
  const pods: string[] = [];
  const claims: Array<string | undefined> = [];
  const existingClaim = `full-existing-${randomUUID()}`;
  let existingCreated = false;
  const profile = await apiClient.createExecutorProfile(seedData.executorId, {
    name: "Independent Docker daemons",
    config: kubernetesProfileConfig(cluster, {
      pod_template_yaml: fullWorkerTemplate(fullWorkerImage),
      "workspace.mode": "managed_pvc",
      "workspace.size": "2Gi",
      "workspace.access_modes": '["ReadWriteOnce"]',
    }),
    prepare_script: fullWorkerPrepare(),
    cleanup_script: "",
    env_vars: [],
  });
  try {
    cluster.kubectl(["-n", cluster.namespace, "create", "-f", "-"], {
      input: JSON.stringify({
        apiVersion: "v1",
        kind: "PersistentVolumeClaim",
        metadata: { name: existingClaim },
        spec: { accessModes: ["ReadWriteOnce"], resources: { requests: { storage: "2Gi" } } },
      }),
    });
    existingCreated = true;
    const existingProfile = await apiClient.createExecutorProfile(seedData.executorId, {
      name: "Existing Docker workspace",
      config: kubernetesProfileConfig(cluster, {
        pod_template_yaml: fullWorkerTemplate(fullWorkerImage),
        "workspace.mode": "existing_claim",
        "workspace.claim_name": existingClaim,
      }),
      prepare_script: fullWorkerPrepare(),
      cleanup_script: "",
      env_vars: [],
    });
    for (const suffix of ["one", "two"]) {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        `Docker isolation ${suffix}`,
        seedData.agentProfileId,
        {
          description: 'e2e:message("ready")',
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          executor_id: seedData.executorId,
          executor_profile_id: suffix === "one" ? profile.id : existingProfile.id,
        },
      );
      tasks.push(task);
      pods.push((await waitForKubernetesPod(cluster, task.id, task.session_id!)).metadata.name);
      claims.push(
        suffix === "one"
          ? (await waitForKubernetesPVC(cluster, task.id, task.session_id!)).metadata.name
          : undefined,
      );
      await waitForTaskSessionState(apiClient, task.id, task.session_id!, "WAITING_FOR_INPUT");
    }
    const existingUID = cluster.json<{ metadata: { uid: string } }>([
      "-n",
      cluster.namespace,
      "get",
      "pvc",
      existingClaim,
    ]).metadata.uid;
    const daemonIDs = pods.map((pod) => run(cluster, pod, "docker info --format '{{.ID}}'"));
    expect(daemonIDs[0]).not.toBe(daemonIDs[1]);
    run(cluster, pods[0], "docker volume create isolated-volume");
    expect(run(cluster, pods[1], "docker volume ls --format '{{.Name}}'")).not.toContain(
      "isolated-volume",
    );
    const inventory = await apiClient.listKubernetesSessions(seedData.executorId);
    for (const task of tasks)
      expect(inventory.filter((row) => row.task_id === task.id)).toHaveLength(1);
    const accounting = fs.readFileSync(
      path.resolve(__dirname, "../../../../../k8s/worker-images/full/accounting.sh"),
      "utf8",
    );
    const nestedID = run(cluster, pods[0], accounting).split("\n").at(-1)!;
    expect(nestedID).toMatch(/^[a-f0-9]{64}$/);
    const pod = cluster.json<{ metadata: { uid: string } }>([
      "-n",
      cluster.namespace,
      "get",
      "pod",
      pods[0],
    ]);
    const evidence = execFileSync(
      "docker",
      [
        "exec",
        `${cluster.name}-control-plane`,
        "sh",
        "-ceu",
        `
      nested=$(find /sys/fs/cgroup -type d \\( -name '${nestedID}' -o -name 'docker-${nestedID}.scope' \\) | head -1)
      test -n "$nested"
      pod=$(find /sys/fs/cgroup -type d -name '*pod${pod.metadata.uid.replaceAll("-", "_")}*' | head -1)
      test -n "$pod"
      case "$nested" in "$pod"/*) ;; *) echo 'Nested process escaped Pod cgroup' >&2; exit 1;; esac
      test "$(cat "$pod/memory.max")" != max
      test "$(cut -d ' ' -f1 "$pod/cpu.max")" != max
      echo pod-memory-limit; cat "$pod/memory.max"
      echo nested-memory-current; cat "$nested/memory.current"
      echo nested-cpu-stat; cat "$nested/cpu.stat"
      test "$(cat "$nested/memory.current")" -ge 16777216
    `,
      ],
      { encoding: "utf8", timeout: 30_000 },
    );
    await testInfo.attach("nested-cgroup-accounting", {
      body: `${cluster.kubernetesVersion}\n${cluster.nodeImage}\n${evidence}`,
      contentType: "text/plain",
    });
    await archive(apiClient, cluster, tasks[0], pods[0], claims[0]);
    expect(run(cluster, pods[1], "docker info --format '{{.ID}}'")).toBe(daemonIDs[1]);
    const remaining = execFileSync(
      "docker",
      [
        "exec",
        `${cluster.name}-control-plane`,
        "find",
        "/sys/fs/cgroup",
        "-type",
        "d",
        "-name",
        `*${nestedID}*`,
      ],
      { encoding: "utf8", timeout: 30_000 },
    );
    expect(remaining.trim()).toBe("");
    await archive(apiClient, cluster, tasks[1], pods[1]);
    expect(
      cluster.json<{ metadata: { uid: string } }>([
        "-n",
        cluster.namespace,
        "get",
        "pvc",
        existingClaim,
      ]).metadata.uid,
    ).toBe(existingUID);
  } finally {
    try {
      for (let i = 0; i < tasks.length; i++)
        await archive(apiClient, cluster, tasks[i], pods[i], claims[i]);
    } finally {
      if (existingCreated)
        cluster.kubectl(["-n", cluster.namespace, "delete", "pvc", existingClaim, "--wait=true"]);
    }
  }
});
