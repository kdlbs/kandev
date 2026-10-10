import { execFileSync, spawn } from "node:child_process";
import path from "node:path";
import { expect } from "@playwright/test";
import type { KubernetesCluster } from "../fixtures/kubernetes-tools";

const ROOT = path.resolve(__dirname, "../../../..");
const FIXTURE_DIGEST = `registry.invalid/fixture@sha256:${"a".repeat(64)}`;

export function isolatedWorkerTemplate(agentImage: string, validationImage: string): string {
  if (!/^sha256:[a-f0-9]{64}$/.test(validationImage))
    throw new Error("Missing verified validator image ID");
  const template = execFileSync(
    "bash",
    [path.join(ROOT, "k8s/worker-images/full/render-template.sh"), "--isolated", FIXTURE_DIGEST],
    { encoding: "utf8", timeout: 10_000 },
  );
  return template
    .replace(`image: ${FIXTURE_DIGEST}`, `image: ${agentImage}`)
    .replace("imagePullPolicy: IfNotPresent", "imagePullPolicy: Never")
    .replaceAll(FIXTURE_DIGEST, validationImage)
    .replace("image_deadline=$(( $(date +%s) + 180 ))", "image_deadline=$(( $(date +%s) + 600 ))");
}

function kubectlExec(cluster: KubernetesCluster, pod: string): string[] {
  return [
    "--kubeconfig",
    cluster.adminKubeconfig,
    "-n",
    cluster.namespace,
    "exec",
    "-i",
    pod,
    "-c",
    "kandev-agent",
    "--",
  ];
}

export async function podCommand(
  cluster: KubernetesCluster,
  pod: string,
  script: string,
): Promise<{ code: number; stdout: string; stderr: string }> {
  const child = spawn(cluster.kubectlBin, [...kubectlExec(cluster, pod), "bash", "-ceu", script], {
    stdio: ["ignore", "pipe", "pipe"],
    timeout: 240_000,
  });
  let stdout = "",
    stderr = "";
  child.stdout.on("data", (data) => {
    stdout += data.toString();
  });
  child.stderr.on("data", (data) => {
    stderr += data.toString();
  });
  return new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("close", (code, signal) => {
      if (signal) reject(new Error(`Pod command terminated by ${signal}: ${stderr}`));
      else resolve({ code: code ?? 1, stdout: stdout.trim(), stderr: stderr.trim() });
    });
  });
}

export async function loadValidationImage(
  cluster: KubernetesCluster,
  pod: string,
  image: string,
): Promise<void> {
  await expect
    .poll(async () => (await podCommand(cluster, pod, "timeout 3 docker info >/dev/null")).code, {
      timeout: 60_000,
    })
    .toBe(0);
  const exporter = spawn("docker", ["image", "save", image], {
    stdio: ["ignore", "pipe", "pipe"],
    timeout: 600_000,
  });
  const importer = spawn(
    cluster.kubectlBin,
    [...kubectlExec(cluster, pod), "docker", "image", "load", "--quiet"],
    { stdio: ["pipe", "ignore", "pipe"], timeout: 600_000 },
  );
  exporter.stdout.pipe(importer.stdin);
  let diagnostics = "";
  for (const child of [exporter, importer])
    child.stderr.on("data", (data) => {
      diagnostics = (diagnostics + data.toString()).slice(-2000);
    });
  importer.stdin.on("error", () => {
    exporter.kill("SIGTERM");
  });
  try {
    await Promise.all(
      [exporter, importer].map(
        (child) =>
          new Promise<void>((resolve, reject) => {
            child.once("error", reject);
            child.once("close", (code, signal) => {
              if (code === 0 && !signal) resolve();
              else
                reject(new Error(`Exact image stream failed (${code}/${signal}): ${diagnostics}`));
            });
          }),
      ),
    );
  } finally {
    exporter.kill("SIGTERM");
    importer.kill("SIGTERM");
  }
  const inspected = await podCommand(
    cluster,
    pod,
    `docker image inspect --format '{{.Id}}' ${image}`,
  );
  expect(inspected.code, inspected.stderr).toBe(0);
  expect(inspected.stdout).toBe(image);
}

export async function validationEvidence(
  cluster: KubernetesCluster,
  pod: string,
): Promise<Record<string, unknown>> {
  const result = await podCommand(
    cluster,
    pod,
    "python3 /opt/full-worker/check.py --preflight; cat /run/docker/validation-accounting.json",
  );
  expect(result.code, result.stderr).toBe(0);
  const receipt = JSON.parse(result.stdout) as Record<string, unknown>;
  expect(receipt.memory_max).toBe(3221225472);
  expect(receipt.probe_memory_max).toBe(67108864);
  expect(Number(receipt.parent_delta)).toBeGreaterThanOrEqual(8388608);
  return receipt;
}

export async function liveValidationCgroup(
  cluster: KubernetesCluster,
  podName: string,
): Promise<string> {
  const pod = cluster.json<{
    status: { containerStatuses: Array<{ name: string; containerID: string }> };
  }>(["-n", cluster.namespace, "get", "pod", podName]);
  const companion = pod.status.containerStatuses
    .find((row) => row.name === "docker-engine")!
    .containerID.replace("containerd://", "");
  const active = await podCommand(
    cluster,
    podName,
    "docker inspect --format '{{.Id}}' kandev-validation",
  );
  expect(active.code, active.stderr).toBe(0);
  if (!/^[a-f0-9]{64}$/.test(companion) || !/^[a-f0-9]{64}$/.test(active.stdout))
    throw new Error("Invalid cgroup workload identity");
  return execFileSync(
    "docker",
    [
      "exec",
      `${cluster.name}-control-plane`,
      "sh",
      "-ceu",
      `
parent=$(find /sys/fs/cgroup -type d -name '*${companion}*')
child=$(find /sys/fs/cgroup -type d -name '${active.stdout}')
test -n "$parent"; test -n "$child"
case "$child" in "$parent"/*) ;; *) echo 'Validator escaped companion ancestor'; exit 1;; esac
test "$(cat "$parent/memory.max")" = 3221225472
test "$(cat "$child/memory.max")" = 2147483648
test "$(cut -d ' ' -f1 "$child/cpu.max")" = 200000
test "$(cat "$child/pids.max")" = 512
printf 'parent=%s\\nchild=%s\\n' "$parent" "$child"
cat "$child/memory.current" "$parent/memory.current" "$child/cpu.stat"
`,
    ],
    { encoding: "utf8", timeout: 30_000 },
  );
}
