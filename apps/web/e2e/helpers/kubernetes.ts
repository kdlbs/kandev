import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { KubernetesCluster, KubernetesPod, KubernetesPVC } from "../fixtures/kubernetes-tools";
import type { ApiClient } from "./api-client";
import { DatabaseSync } from "./node-sqlite";
import { pollUntil } from "./poll-until";

type KubernetesList<T> = { items: T[] };

type KubernetesResourceIdentity = { taskId: string; sessionId: string };

type LabelledKubernetesResource = {
  metadata: { labels?: Record<string, string>; name: string; uid: string };
};

type ServiceAccountKubeconfig = {
  "current-context"?: string;
  contexts?: Array<{ context?: { user?: string }; name?: string }>;
  users?: Array<{ name?: string; user?: { token?: string } }>;
};

export function readKubeconfigBearerToken(kubeconfigPath: string): string {
  const kubeconfig = JSON.parse(
    fs.readFileSync(kubeconfigPath, "utf8"),
  ) as ServiceAccountKubeconfig;
  const context = kubeconfig.contexts?.find(
    (candidate) => candidate.name === kubeconfig["current-context"],
  );
  const token = kubeconfig.users?.find((candidate) => candidate.name === context?.context?.user)
    ?.user?.token;
  if (!token) throw new Error("host service-account kubeconfig has no bearer credential");
  return token;
}

export function selectExactKubernetesResource<T extends LabelledKubernetesResource>(
  resources: T[],
  identity: KubernetesResourceIdentity,
  kind: "Pod" | "PVC",
): T | undefined {
  const mismatched = resources.find(
    (resource) =>
      resource.metadata.labels?.["kandev.ai/task-id"] !== identity.taskId ||
      resource.metadata.labels?.["kandev.ai/session-id"] !== identity.sessionId,
  );
  if (mismatched) {
    throw new Error(
      `${kind} ${mismatched.metadata.name} has unexpected identity while selecting task ${identity.taskId}, session ${identity.sessionId}`,
    );
  }
  if (resources.length > 1) {
    throw new Error(
      `found multiple ${kind}s for task ${identity.taskId}, session ${identity.sessionId}; one session must map to one resource`,
    );
  }
  return resources[0];
}

export async function waitForKubernetesPod(
  cluster: KubernetesCluster,
  taskId: string,
  sessionId: string,
  timeout = 120_000,
): Promise<KubernetesPod> {
  const result = await pollUntil(
    () => {
      const pods = cluster.json<KubernetesList<KubernetesPod>>([
        "-n",
        cluster.namespace,
        "get",
        "pods",
        "-l",
        `kandev.ai/task-id=${taskId},kandev.ai/session-id=${sessionId}`,
      ]).items;
      const found = selectExactKubernetesResource(pods, { taskId, sessionId }, "Pod");
      const main = found?.status?.containerStatuses?.find(
        (container) => container.name === "kandev-agent",
      );
      return {
        found,
        ready: found?.status?.phase === "Running" && main?.ready === true && main.state?.running,
      };
    },
    (value) => Boolean(value.ready),
    timeout,
    `Waiting for Kubernetes Pod for task ${taskId}, session ${sessionId}`,
  );
  return result.found!;
}

export async function waitForKubernetesPVC(
  cluster: KubernetesCluster,
  taskId: string,
  sessionId: string,
  timeout = 120_000,
): Promise<KubernetesPVC> {
  const result = await pollUntil(
    () => {
      const claims = cluster.json<KubernetesList<KubernetesPVC>>([
        "-n",
        cluster.namespace,
        "get",
        "persistentvolumeclaims",
        "-l",
        `kandev.ai/task-id=${taskId},kandev.ai/session-id=${sessionId}`,
      ]).items;
      const found = selectExactKubernetesResource(claims, { taskId, sessionId }, "PVC");
      return { found, ready: Boolean(found?.metadata.uid) };
    },
    (value) => value.ready,
    timeout,
    `Waiting for Kubernetes PVC for task ${taskId}, session ${sessionId}`,
  );
  return result.found!;
}

export async function waitForKubernetesResourceAbsent(
  cluster: KubernetesCluster,
  kind: "pod" | "persistentvolumeclaim",
  name: string,
  timeout = 120_000,
): Promise<void> {
  await pollUntil(
    () =>
      cluster.kubectl(
        ["-n", cluster.namespace, "get", kind, name, "--ignore-not-found", "-o", "name"],
        { quiet: true },
      ) === "",
    (absent) => absent,
    timeout,
    `Waiting for ${kind}/${name} deletion`,
  );
}

export async function waitForKubernetesRestart(
  cluster: KubernetesCluster,
  podName: string,
  previousRestarts: number,
  timeout = 120_000,
): Promise<KubernetesPod> {
  const result = await pollUntil(
    () => {
      const pod = cluster.json<KubernetesPod>(["-n", cluster.namespace, "get", "pod", podName]);
      const restarts = pod.status?.containerStatuses?.find(
        (row) => row.name === "kandev-agent",
      )?.restartCount;
      return { pod, ready: restarts !== undefined && restarts > previousRestarts };
    },
    (value) => value.ready,
    timeout,
    `Waiting for main-container restart in ${podName}`,
  );
  return result.pod;
}

export async function waitForFailedSession(
  apiClient: ApiClient,
  taskId: string,
  expected: RegExp,
  timeout = 120_000,
): Promise<string> {
  const result = await pollUntil(
    async () => {
      const { sessions } = await apiClient.listTaskSessions(taskId);
      const failed = sessions.find((session) => session.state === "FAILED");
      const error = failed?.error_message ?? "";
      return { error, ready: expected.test(error) };
    },
    (value) => value.ready,
    timeout,
    `Waiting for causal Kubernetes failure matching ${expected}`,
  );
  return result.error;
}

export async function waitForTaskSessionState(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  expectedState: string,
  timeout = 120_000,
): Promise<void> {
  await pollUntil(
    async () => {
      const { sessions } = await apiClient.listTaskSessions(taskId);
      return sessions.find((session) => session.id === sessionId)?.state ?? "";
    },
    (state) => state === expectedState,
    timeout,
    `Waiting for session ${sessionId} to reach ${expectedState}`,
  );
}

export async function waitForTaskResourceCleanupAttempt(
  backendTmpDir: string,
  taskId: string,
  expectedError: RegExp,
  timeout = 60_000,
): Promise<{ attempts: number; last_error: string; state: string }> {
  const database = path.join(backendTmpDir, "kandev.db");
  const result = await pollUntil(
    () => {
      const sqlite = new DatabaseSync(database, { readOnly: true });
      let result = { attempts: 0, last_error: "", state: "" };
      try {
        result =
          (sqlite
            .prepare(
              `SELECT state, attempts, last_error
                 FROM task_resource_cleanup_jobs
                WHERE task_id = ?
                  AND trigger IN ('archive', 'cascade_archive')
                ORDER BY created_at DESC
                LIMIT 1`,
            )
            .get(taskId) as typeof result | undefined) ?? result;
      } finally {
        sqlite.close();
      }
      return {
        result,
        ready:
          result.attempts > 0 &&
          ["retry_wait", "failed"].includes(result.state) &&
          expectedError.test(result.last_error),
      };
    },
    (value) => value.ready,
    timeout,
    `Waiting for archive cleanup attempt for task ${taskId}`,
  );
  return result.result;
}

export function execInKubernetesPod(
  cluster: KubernetesCluster,
  podName: string,
  command: string[],
): string {
  return cluster.kubectl([
    "-n",
    cluster.namespace,
    "exec",
    podName,
    "-c",
    "kandev-agent",
    "--",
    ...command,
  ]);
}

export function kubernetesContainerLogsContainCredential(
  cluster: KubernetesCluster,
  pod: KubernetesPod,
  credential: string,
): boolean {
  const containers = pod.spec?.containers ?? [];
  if (containers.length === 0) {
    throw new Error(`agent Pod ${pod.metadata.name} has no containers to inspect`);
  }
  let containsCredential = false;
  for (const container of containers) {
    for (const previous of [false, true]) {
      const args = [
        "--kubeconfig",
        cluster.adminKubeconfig,
        "-n",
        cluster.namespace,
        "logs",
        pod.metadata.name,
        "-c",
        container.name,
        "--timestamps=true",
      ];
      if (previous) args.push("--previous");
      const result = spawnSync(cluster.kubectlBin, args, {
        encoding: "utf8",
        timeout: 30_000,
      });
      if (result.error || (!previous && result.status !== 0)) {
        const history = previous ? "previous" : "current";
        throw new Error(
          `failed to fetch ${history} logs for agent Pod ${pod.metadata.name}/${container.name}`,
        );
      }
      containsCredential ||= `${result.stdout}\n${result.stderr}`.includes(credential);
    }
  }
  return containsCredential;
}

export function recreateForeignPod(
  cluster: KubernetesCluster,
  original: KubernetesPod,
): KubernetesPod {
  const labels = Object.entries(original.metadata.labels ?? {})
    .map(([key, value]) => `    ${key}: ${JSON.stringify(value)}`)
    .join("\n");
  cluster.kubectl([
    "-n",
    cluster.namespace,
    "delete",
    "pod",
    original.metadata.name,
    "--wait=true",
    "--timeout=60s",
  ]);
  cluster.kubectl(["apply", "-f", "-"], {
    input: `apiVersion: v1
kind: Pod
metadata:
  name: ${original.metadata.name}
  namespace: ${cluster.namespace}
  labels:
${labels}
spec:
  restartPolicy: Always
  containers:
    - name: kandev-agent
      image: ${cluster.image}
      imagePullPolicy: Never
      command: ["/bin/sh", "-c", "while true; do sleep 60; done"]
`,
  });
  return cluster.json<KubernetesPod>([
    "-n",
    cluster.namespace,
    "get",
    "pod",
    original.metadata.name,
  ]);
}
