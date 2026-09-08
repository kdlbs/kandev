import { type ChildProcess, execFileSync, spawn, spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { dwell } from "../helpers/causal-waits";
import { waitForHealth } from "./backend";
import {
  assertRuntimeImageTagAvailable,
  FixtureResourceOwnership,
  redactKubernetesDiagnosticText,
  renderWorkloadRBACRules,
  WORKLOAD_RBAC_PROBES,
} from "./kubernetes-fixture-policy";
import { buildServiceAccountKubeconfig } from "./kubernetes-kubeconfig";
import {
  KIND_SHA256_AMD64,
  KIND_VERSION,
  KUBERNETES_E2E_BASE_IMAGE,
  type KubernetesFixturePin,
  resolveKubernetesFixturePin,
} from "./kubernetes-pins";

export {
  KIND_NODE_IMAGE,
  KIND_SHA256_AMD64,
  KIND_VERSION,
  KUBECTL_SHA256_AMD64,
  KUBERNETES_FIXTURE_PINS,
  KUBERNETES_E2E_BASE_IMAGE,
  KUBERNETES_VERSION,
} from "./kubernetes-pins";

const REPO_ROOT = path.resolve(__dirname, "../../../..");
const BACKEND_DIR = path.join(REPO_ROOT, "apps/backend");
const WEB_DIST_DIR = path.join(REPO_ROOT, "apps/web/dist");
const CONTROL_NAMESPACE = "kandev-e2e-control";
const WORKLOAD_NAMESPACE = "kandev-e2e-workloads";
const HOST_SERVICE_ACCOUNT = "kandev-host";
const IN_CLUSTER_SERVICE_ACCOUNT = "kandev-in-cluster";
const RESTRICTED_SERVICE_ACCOUNT = "kandev-restricted";
const TOOL_DOWNLOAD_TIMEOUT_MS = 120_000;

export type KubernetesPod = {
  metadata: {
    name: string;
    uid: string;
    labels?: Record<string, string>;
  };
  spec?: {
    automountServiceAccountToken?: boolean;
    containers?: Array<{ name: string }>;
  };
  status?: {
    phase?: string;
    containerStatuses?: Array<{
      name: string;
      ready?: boolean;
      restartCount: number;
      state?: Record<string, unknown>;
    }>;
  };
};

export type KubernetesPVC = {
  metadata: { name: string; uid: string; labels?: Record<string, string> };
  status?: { phase?: string };
};

export type InClusterBackend = {
  baseUrl: string;
  frontendUrl: string;
  stop: () => Promise<void>;
};

export type KubernetesCluster = {
  name: string;
  kubernetesVersion: string;
  nodeImage: string;
  kindBin: string;
  kubectlBin: string;
  adminKubeconfig: string;
  hostKubeconfig: string;
  restrictedKubeconfig: string;
  namespace: string;
  controlNamespace: string;
  image: string;
  kubectl: (args: string[], options?: KubectlOptions) => string;
  json: <T>(args: string[], options?: KubectlOptions) => T;
  podTemplate: (overrides?: PodTemplateOverrides) => string;
  startInClusterBackend: () => Promise<InClusterBackend>;
  cleanupWorkloads: () => Promise<void>;
  diagnostics: () => string;
  dispose: () => Promise<void>;
};

type KubectlOptions = {
  kubeconfig?: string;
  input?: string;
  quiet?: boolean;
  timeoutMs?: number;
};

type PodTemplateOverrides = {
  image?: string;
  imagePullPolicy?: "Always" | "IfNotPresent" | "Never";
  nodeSelector?: Record<string, string>;
};

type ToolPaths = { kind: string; kubectl: string };

function requireSupportedHost(): void {
  if (process.platform !== "linux") {
    throw new Error(`Kind-backed Kubernetes E2E currently requires Linux, not ${process.platform}`);
  }
  if (process.arch === "x64") return;
  throw new Error(
    `Kubernetes E2E currently requires x64 because CI builds linux/amd64 agent helpers, not ${process.arch}`,
  );
}

function verifyTool(pathname: string, expectedChecksum: string): string {
  const resolved = path.resolve(pathname);
  const actual = createHash("sha256").update(fs.readFileSync(resolved)).digest("hex");
  if (actual !== expectedChecksum) {
    throw new Error(`pinned tool checksum mismatch at ${resolved}: got ${actual}`);
  }
  return resolved;
}

function downloadPinnedTool(destination: string, url: string, expectedChecksum: string): string {
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  if (!fs.existsSync(destination)) {
    const partial = `${destination}.${process.pid}.partial`;
    try {
      execFileSync(
        "curl",
        [
          "--fail",
          "--location",
          "--silent",
          "--show-error",
          "--retry",
          "3",
          "--retry-delay",
          "2",
          "--retry-all-errors",
          url,
          "--output",
          partial,
        ],
        {
          timeout: TOOL_DOWNLOAD_TIMEOUT_MS,
          stdio: ["ignore", "inherit", "inherit"],
        },
      );
      const actual = createHash("sha256").update(fs.readFileSync(partial)).digest("hex");
      if (actual !== expectedChecksum) {
        throw new Error(`checksum mismatch for ${url}: got ${actual}, want ${expectedChecksum}`);
      }
      fs.chmodSync(partial, 0o755);
      fs.renameSync(partial, destination);
    } finally {
      fs.rmSync(partial, { force: true });
    }
  }
  const actual = createHash("sha256").update(fs.readFileSync(destination)).digest("hex");
  if (actual !== expectedChecksum) {
    throw new Error(`cached tool checksum mismatch at ${destination}`);
  }
  return destination;
}

function resolveTools(root: string, fixturePin: KubernetesFixturePin): ToolPaths {
  requireSupportedHost();
  const kind =
    process.env.KANDEV_E2E_KIND_BIN !== undefined
      ? verifyTool(process.env.KANDEV_E2E_KIND_BIN, KIND_SHA256_AMD64)
      : downloadPinnedTool(
          path.join(root, `kind-${KIND_VERSION}-amd64`),
          `https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64`,
          KIND_SHA256_AMD64,
        );
  const kubectl =
    process.env.KANDEV_E2E_KUBECTL_BIN !== undefined
      ? verifyTool(process.env.KANDEV_E2E_KUBECTL_BIN, fixturePin.kubectlSha256Amd64)
      : downloadPinnedTool(
          path.join(root, `kubectl-${fixturePin.version}-amd64`),
          `https://dl.k8s.io/release/${fixturePin.version}/bin/linux/amd64/kubectl`,
          fixturePin.kubectlSha256Amd64,
        );
  return { kind: path.resolve(kind), kubectl: path.resolve(kubectl) };
}

function requireBuildArtifacts(): void {
  const artifacts = [
    path.join(BACKEND_DIR, "bin/kandev"),
    path.join(BACKEND_DIR, "bin/agentctl-linux-amd64"),
    path.join(BACKEND_DIR, "bin/mock-agent-linux-amd64"),
    WEB_DIST_DIR,
  ];
  const missing = artifacts.filter((item) => !fs.existsSync(item));
  if (missing.length > 0) {
    throw new Error(
      `Kubernetes E2E build artifacts are missing: ${missing.join(", ")}. Run make build-backend build-backend-linux-helpers build-web-e2e.`,
    );
  }
}

function buildRuntimeImage(tag: string): void {
  requireBuildArtifacts();
  const context = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-kubernetes-e2e-image-"));
  const dockerfile = `FROM ${KUBERNETES_E2E_BASE_IMAGE}
RUN apt-get update \\
 && apt-get install -y --no-install-recommends ca-certificates curl git \\
 && rm -rf /var/lib/apt/lists/*
COPY kandev /usr/local/bin/kandev
COPY agentctl-linux-amd64 /usr/local/bin/agentctl-linux-amd64
COPY mock-agent-linux-amd64 /usr/local/bin/mock-agent
COPY web-dist /opt/kandev/web
RUN chmod 0755 /usr/local/bin/kandev /usr/local/bin/agentctl-linux-amd64 /usr/local/bin/mock-agent \
 && ln -s /usr/local/bin/agentctl-linux-amd64 /usr/local/bin/agentctl
ENV PATH=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
WORKDIR /workspace
`;
  try {
    fs.copyFileSync(path.join(BACKEND_DIR, "bin/kandev"), path.join(context, "kandev"));
    fs.copyFileSync(
      path.join(BACKEND_DIR, "bin/agentctl-linux-amd64"),
      path.join(context, "agentctl-linux-amd64"),
    );
    fs.copyFileSync(
      path.join(BACKEND_DIR, "bin/mock-agent-linux-amd64"),
      path.join(context, "mock-agent-linux-amd64"),
    );
    fs.cpSync(WEB_DIST_DIR, path.join(context, "web-dist"), { recursive: true });
    execFileSync("docker", ["build", "--tag", tag, "--file", "-", context], {
      input: dockerfile,
      timeout: 300_000,
      stdio: process.env.E2E_DEBUG ? ["pipe", "inherit", "inherit"] : ["pipe", "ignore", "inherit"],
    });
  } finally {
    fs.rmSync(context, { recursive: true, force: true });
  }
}

function dockerImageTagExists(tag: string): boolean {
  const result = spawnSync("docker", ["image", "inspect", "--format", "{{.Id}}", tag], {
    encoding: "utf8",
    timeout: 30_000,
  });
  if (result.error) {
    throw new Error(`failed to inspect Docker image ${tag}`, { cause: result.error });
  }
  if (result.status === 0) return true;
  if (result.status === 1 && /no such image/i.test(result.stderr)) return false;
  throw new Error(
    `failed to inspect Docker image ${tag}: ${result.stderr.trim() || `exit ${result.status}`}`,
  );
}

function runKubectl(
  kubectlBin: string,
  defaultKubeconfig: string,
  args: string[],
  options: KubectlOptions = {},
): string {
  return execFileSync(
    kubectlBin,
    ["--kubeconfig", options.kubeconfig ?? defaultKubeconfig, ...args],
    {
      encoding: "utf8",
      input: options.input,
      timeout: options.timeoutMs ?? 120_000,
      stdio: options.quiet ? ["pipe", "pipe", "pipe"] : ["pipe", "pipe", "inherit"],
    },
  ).trim();
}

function workloadRBAC(): string {
  return `apiVersion: v1
kind: Namespace
metadata:
  name: ${CONTROL_NAMESPACE}
---
apiVersion: v1
kind: Namespace
metadata:
  name: ${WORKLOAD_NAMESPACE}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${HOST_SERVICE_ACCOUNT}
  namespace: ${CONTROL_NAMESPACE}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${IN_CLUSTER_SERVICE_ACCOUNT}
  namespace: ${CONTROL_NAMESPACE}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${RESTRICTED_SERVICE_ACCOUNT}
  namespace: ${CONTROL_NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kandev-executor
  namespace: ${WORKLOAD_NAMESPACE}
rules:
${renderWorkloadRBACRules()}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kandev-executor
  namespace: ${WORKLOAD_NAMESPACE}
subjects:
  - kind: ServiceAccount
    name: ${HOST_SERVICE_ACCOUNT}
    namespace: ${CONTROL_NAMESPACE}
  - kind: ServiceAccount
    name: ${IN_CLUSTER_SERVICE_ACCOUNT}
    namespace: ${CONTROL_NAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: kandev-executor
`;
}

function assertWorkloadServiceAccountRBAC(kubectlBin: string, kubeconfig: string): void {
  for (const probe of WORKLOAD_RBAC_PROBES) {
    const result = spawnSync(
      kubectlBin,
      [
        "--kubeconfig",
        kubeconfig,
        "auth",
        "can-i",
        probe.verb,
        probe.resource,
        "--namespace",
        WORKLOAD_NAMESPACE,
        "--request-timeout=15s",
      ],
      { encoding: "utf8", timeout: 30_000 },
    );
    if (result.error) {
      throw new Error(`Kubernetes RBAC probe failed for ${probe.verb} ${probe.resource}`, {
        cause: result.error,
      });
    }
    const answer = result.stdout.trim();
    const expected = probe.allowed ? "yes" : "no";
    if (answer !== expected || (probe.allowed && result.status !== 0)) {
      throw new Error(
        `Kubernetes RBAC probe ${probe.verb} ${probe.resource} returned ${JSON.stringify(answer)} (exit ${result.status}), expected ${expected}: ${result.stderr.trim()}`,
      );
    }
  }
}

function ownershipMarkerPath(): string | undefined {
  const configured = process.env.KANDEV_E2E_KIND_OWNERSHIP_MARKER?.trim();
  return configured ? path.resolve(configured) : undefined;
}

function writeClusterOwnershipMarker(marker: string | undefined, clusterName: string): void {
  if (!marker) return;
  if (fs.existsSync(marker)) {
    throw new Error(`refusing to overwrite existing Kind ownership marker ${marker}`);
  }
  fs.writeFileSync(marker, `${clusterName}\n`, { flag: "wx", mode: 0o600 });
}

function removeClusterOwnershipMarker(marker: string | undefined, clusterName: string): void {
  if (!marker || !fs.existsSync(marker)) return;
  const recordedName = fs.readFileSync(marker, "utf8").trim();
  if (recordedName !== clusterName) {
    throw new Error(
      `refusing to remove Kind ownership marker ${marker}: recorded ${JSON.stringify(recordedName)}, expected ${clusterName}`,
    );
  }
  fs.rmSync(marker);
}

function serviceAccountKubeconfig(
  kubectlBin: string,
  adminKubeconfig: string,
  serviceAccount: string,
  destination: string,
): void {
  const flattened = JSON.parse(
    runKubectl(kubectlBin, adminKubeconfig, ["config", "view", "--raw", "--flatten", "-o", "json"]),
  ) as {
    clusters: Array<{ cluster: { server: string; "certificate-authority-data": string } }>;
  };
  const token = runKubectl(kubectlBin, adminKubeconfig, [
    "-n",
    CONTROL_NAMESPACE,
    "create",
    "token",
    serviceAccount,
    "--duration=2h",
  ]);
  fs.writeFileSync(
    destination,
    JSON.stringify(
      buildServiceAccountKubeconfig(flattened.clusters[0]!.cluster, serviceAccount, token),
    ),
    { mode: 0o600 },
  );
}

function podTemplate(image: string, overrides: PodTemplateOverrides = {}): string {
  const nodeSelector = overrides.nodeSelector
    ? `\n    nodeSelector:\n${Object.entries(overrides.nodeSelector)
        .map(([key, value]) => `      ${key}: ${JSON.stringify(value)}`)
        .join("\n")}`
    : "";
  return `apiVersion: v1
kind: PodTemplate
template:
  spec:${nodeSelector}
    containers:
      - name: kandev-agent
        image: ${overrides.image ?? image}
        imagePullPolicy: ${overrides.imagePullPolicy ?? "Never"}
`;
}

function uniqueClusterName(workerIndex: number): string {
  const configured = process.env.KANDEV_E2E_KIND_CLUSTER_NAME;
  if (configured) {
    const normalized = configured
      .toLowerCase()
      .replace(/[^a-z0-9-]/g, "-")
      .slice(0, 63)
      .replace(/^-+|-+$/g, "");
    if (!normalized) {
      throw new Error("KANDEV_E2E_KIND_CLUSTER_NAME must contain a letter or digit");
    }
    return normalized;
  }
  return `kandev-e2e-${process.pid}-${workerIndex}-${randomUUID().slice(0, 6)}`;
}

async function waitForPortForward(proc: ChildProcess, timeoutMs = 30_000): Promise<number> {
  return new Promise<number>((resolve, reject) => {
    const timer = setTimeout(() => {
      dispose();
      reject(new Error(`kubectl port-forward did not report a local port within ${timeoutMs}ms`));
    }, timeoutMs);
    let stderr = "";
    const onData = (chunk: Buffer) => {
      const text = chunk.toString();
      stderr += text;
      const match = text.match(/Forwarding from 127\.0\.0\.1:(\d+) -> 8080/);
      if (!match) return;
      dispose();
      resolve(Number(match[1]));
    };
    const onExit = (code: number | null) => {
      dispose();
      reject(new Error(`kubectl port-forward exited with ${code}: ${stderr}`));
    };
    const dispose = () => {
      clearTimeout(timer);
      proc.stdout?.off("data", onData);
      proc.stderr?.off("data", onData);
      proc.off("exit", onExit);
    };
    proc.stdout?.on("data", onData);
    proc.stderr?.on("data", onData);
    proc.once("exit", onExit);
  });
}

async function stopChild(proc: ChildProcess): Promise<void> {
  if (proc.exitCode !== null) return;
  proc.kill("SIGTERM");
  await new Promise<void>((resolve) => {
    const timer = setTimeout(() => {
      proc.kill("SIGKILL");
      resolve();
    }, 5_000);
    proc.once("exit", () => {
      clearTimeout(timer);
      resolve();
    });
  });
}

function inClusterBackendPod(image: string): string {
  return `apiVersion: v1
kind: Pod
metadata:
  name: kandev-in-cluster
  namespace: ${CONTROL_NAMESPACE}
  labels:
    app.kubernetes.io/name: kandev-in-cluster-e2e
spec:
  serviceAccountName: ${IN_CLUSTER_SERVICE_ACCOUNT}
  restartPolicy: Never
  containers:
    - name: backend
      image: ${image}
      imagePullPolicy: Never
      command: ["/bin/sh", "-c"]
      args:
        - mkdir -p /data/home /data/worktrees /data/repos && exec /usr/local/bin/kandev __backend
      ports:
        - name: http
          containerPort: 8080
      readinessProbe:
        httpGet:
          path: /health
          port: http
        periodSeconds: 1
        failureThreshold: 60
      env:
        - {name: HOME, value: /data}
        - {name: KANDEV_HOME_DIR, value: /data/home}
        - {name: KANDEV_SERVER_PORT, value: "8080"}
        - {name: KANDEV_SERVER_HOST, value: "0.0.0.0"}
        - {name: KANDEV_WEB_DIST_DIR, value: /opt/kandev/web}
        - {name: KANDEV_DATABASE_PATH, value: /data/kandev.db}
        - {name: KANDEV_WORKTREE_ENABLED, value: "true"}
        - {name: KANDEV_WORKTREE_BASEPATH, value: /data/worktrees}
        - {name: KANDEV_REPOCLONE_BASEPATH, value: /data/repos}
        - {name: KANDEV_E2E_MOCK, value: "true"}
        - {name: KANDEV_DOCKER_ENABLED, value: "false"}
        - {name: KANDEV_AGENTCTL_LINUX_BINARY, value: /usr/local/bin/agentctl-linux-amd64}
        - {name: KANDEV_MOCK_AGENT_LINUX_BINARY, value: /usr/local/bin/mock-agent}
        - {name: KANDEV_LOG_LEVEL, value: warn}
      volumeMounts:
        - {name: data, mountPath: /data}
  volumes:
    - name: data
      emptyDir: {}
`;
}

function diagnosticCommand(
  kubectlBin: string,
  adminKubeconfig: string,
  credential: string,
  args: string[],
): { output: string; succeeded: boolean } {
  const result = spawnSync(kubectlBin, ["--kubeconfig", adminKubeconfig, ...args], {
    encoding: "utf8",
    timeout: 30_000,
  });
  const command = `kubectl ${args.join(" ")}`;
  if (result.error) {
    return {
      output: redactKubernetesDiagnosticText(
        `${command}\nERROR: ${String(result.error)}`,
        credential,
      ),
      succeeded: false,
    };
  }
  const streams = redactKubernetesDiagnosticText(
    [result.stdout.trim(), result.stderr.trim()].filter(Boolean).join("\n"),
    credential,
  );
  return {
    output: `${command}\n${streams || "<no output>"}${result.status === 0 ? "" : `\n(exit ${result.status})`}`,
    succeeded: result.status === 0,
  };
}

function kubeconfigBearerToken(kubeconfig: string): string {
  const document = JSON.parse(fs.readFileSync(kubeconfig, "utf8")) as {
    "current-context"?: string;
    contexts?: Array<{ context?: { user?: string }; name?: string }>;
    users?: Array<{ name?: string; user?: { token?: string } }>;
  };
  const context = document.contexts?.find(
    (candidate) => candidate.name === document["current-context"],
  );
  const credential = document.users?.find((candidate) => candidate.name === context?.context?.user)
    ?.user?.token;
  if (!credential) {
    throw new Error("cannot safely collect Kubernetes diagnostics without a host credential");
  }
  return credential;
}

function kubernetesDiagnostics(
  kubectlBin: string,
  adminKubeconfig: string,
  hostKubeconfig: string,
): string {
  const credential = kubeconfigBearerToken(hostKubeconfig);
  const sections = [
    diagnosticCommand(kubectlBin, adminKubeconfig, credential, [
      "get",
      "pods,persistentvolumeclaims,events",
      "--all-namespaces",
      "-o",
      "wide",
    ]).output,
  ];
  for (const namespace of [WORKLOAD_NAMESPACE, CONTROL_NAMESPACE]) {
    const listing = diagnosticCommand(kubectlBin, adminKubeconfig, credential, [
      "-n",
      namespace,
      "get",
      "pods",
      "-o",
      "json",
    ]);
    sections.push(listing.output);
    if (!listing.succeeded) continue;
    const pods = JSON.parse(listing.output.slice(listing.output.indexOf("\n") + 1)) as {
      items: Array<{
        metadata: { name: string };
        spec?: { containers?: Array<{ name: string }> };
      }>;
    };
    for (const pod of pods.items) {
      const podName = pod.metadata.name;
      sections.push(
        diagnosticCommand(kubectlBin, adminKubeconfig, credential, [
          "-n",
          namespace,
          "get",
          "pod",
          podName,
          "-o",
          "json",
        ]).output,
        diagnosticCommand(kubectlBin, adminKubeconfig, credential, [
          "-n",
          namespace,
          "describe",
          "pod",
          podName,
        ]).output,
      );
      for (const container of pod.spec?.containers ?? []) {
        sections.push(
          diagnosticCommand(kubectlBin, adminKubeconfig, credential, [
            "-n",
            namespace,
            "logs",
            podName,
            "-c",
            container.name,
            "--timestamps",
          ]).output,
          diagnosticCommand(kubectlBin, adminKubeconfig, credential, [
            "-n",
            namespace,
            "logs",
            podName,
            "-c",
            container.name,
            "--previous",
            "--timestamps",
          ]).output,
        );
      }
    }
  }
  return redactKubernetesDiagnosticText(sections.join("\n\n"), credential);
}

export async function provisionKubernetesCluster(
  root: string,
  workerIndex: number,
): Promise<KubernetesCluster> {
  const fixturePin = resolveKubernetesFixturePin();
  const tools = resolveTools(path.join(root, "kubernetes-tools"), fixturePin);
  const name = uniqueClusterName(workerIndex);
  const adminKubeconfig = path.join(root, `${name}.kubeconfig`);
  const hostKubeconfig = path.join(root, `${name}.host.kubeconfig`);
  const restrictedKubeconfig = path.join(root, `${name}.restricted.kubeconfig`);
  const image = `kandev-kubernetes-e2e:${name}`;
  const marker = ownershipMarkerPath();
  const ownership = new FixtureResourceOwnership();
  let inCluster: { context: InClusterBackend; proc: ChildProcess } | undefined;

  const kubectl = (args: string[], options?: KubectlOptions) =>
    runKubectl(tools.kubectl, adminKubeconfig, args, options);
  const json = <T>(args: string[], options?: KubectlOptions): T =>
    JSON.parse(kubectl([...args, "-o", "json"], options)) as T;

  const deleteCluster = () => {
    if (!ownership.owns("cluster")) return;
    execFileSync(tools.kind, ["delete", "cluster", "--name", name], {
      timeout: 120_000,
      stdio: process.env.E2E_DEBUG ? "inherit" : "ignore",
    });
    removeClusterOwnershipMarker(marker, name);
    ownership.release("cluster");
  };

  const removeRuntimeImage = () => {
    if (!ownership.owns("image")) return;
    if (!dockerImageTagExists(image)) {
      ownership.release("image");
      return;
    }
    execFileSync("docker", ["image", "rm", "--force", image], {
      timeout: 60_000,
      stdio: process.env.E2E_DEBUG ? "inherit" : "ignore",
    });
    ownership.release("image");
  };

  try {
    if (marker && fs.existsSync(marker)) {
      throw new Error(`refusing to reuse existing Kind ownership marker ${marker}`);
    }
    const existingClusters = execFileSync(tools.kind, ["get", "clusters", "--quiet"], {
      encoding: "utf8",
      timeout: 30_000,
    })
      .trim()
      .split(/\s+/)
      .filter(Boolean);
    if (existingClusters.includes(name)) {
      throw new Error(`refusing to reuse existing Kind cluster ${name}`);
    }
    assertRuntimeImageTagAvailable(image, dockerImageTagExists(image));
    ownership.acquire("image", () => buildRuntimeImage(image));
    writeClusterOwnershipMarker(marker, name);
    ownership.acquire("cluster", () =>
      execFileSync(
        tools.kind,
        [
          "create",
          "cluster",
          "--name",
          name,
          "--image",
          fixturePin.nodeImage,
          "--kubeconfig",
          adminKubeconfig,
          "--wait",
          "180s",
        ],
        { timeout: 300_000, stdio: process.env.E2E_DEBUG ? "inherit" : "ignore" },
      ),
    );
    execFileSync(tools.kind, ["load", "docker-image", image, "--name", name], {
      timeout: 180_000,
      stdio: process.env.E2E_DEBUG ? "inherit" : "ignore",
    });
    kubectl(["apply", "-f", "-"], { input: workloadRBAC() });
    serviceAccountKubeconfig(tools.kubectl, adminKubeconfig, HOST_SERVICE_ACCOUNT, hostKubeconfig);
    assertWorkloadServiceAccountRBAC(tools.kubectl, hostKubeconfig);
    serviceAccountKubeconfig(
      tools.kubectl,
      adminKubeconfig,
      RESTRICTED_SERVICE_ACCOUNT,
      restrictedKubeconfig,
    );
  } catch (error) {
    try {
      deleteCluster();
    } finally {
      removeRuntimeImage();
    }
    throw error;
  }

  const cleanupWorkloads = async () => {
    kubectl(
      [
        "-n",
        WORKLOAD_NAMESPACE,
        "delete",
        "pods,persistentvolumeclaims",
        "--all",
        "--ignore-not-found",
        "--wait=true",
        "--timeout=90s",
      ],
      { quiet: true },
    );
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      const resources = json<{ items: unknown[] }>([
        "-n",
        WORKLOAD_NAMESPACE,
        "get",
        "pods,persistentvolumeclaims",
      ]);
      if (resources.items.length === 0) return;
      await dwell(
        250,
        "poll-interval",
        "Kubernetes deletion completion has no cross-resource watch in the E2E fixture",
      );
    }
    throw new Error(`Kubernetes E2E cleanup left resources in ${WORKLOAD_NAMESPACE}`);
  };

  const startInClusterBackend = async (): Promise<InClusterBackend> => {
    if (inCluster) return inCluster.context;
    kubectl(["apply", "-f", "-"], { input: inClusterBackendPod(image) });
    kubectl([
      "-n",
      CONTROL_NAMESPACE,
      "wait",
      "--for=condition=Ready",
      "pod/kandev-in-cluster",
      "--timeout=120s",
    ]);
    const proc = spawn(
      tools.kubectl,
      [
        "--kubeconfig",
        adminKubeconfig,
        "-n",
        CONTROL_NAMESPACE,
        "port-forward",
        "pod/kandev-in-cluster",
        ":8080",
        "--address=127.0.0.1",
      ],
      { stdio: ["ignore", "pipe", "pipe"] },
    );
    try {
      const port = await waitForPortForward(proc);
      const baseUrl = `http://127.0.0.1:${port}`;
      await waitForHealth(`${baseUrl}/health`, 30_000, proc);
      const context: InClusterBackend = {
        baseUrl,
        frontendUrl: baseUrl,
        stop: async () => {
          await stopChild(proc);
          kubectl(
            [
              "-n",
              CONTROL_NAMESPACE,
              "delete",
              "pod/kandev-in-cluster",
              "--ignore-not-found",
              "--wait=true",
            ],
            { quiet: true },
          );
        },
      };
      inCluster = { context, proc };
      return context;
    } catch (error) {
      await stopChild(proc);
      kubectl(
        [
          "-n",
          CONTROL_NAMESPACE,
          "delete",
          "pod/kandev-in-cluster",
          "--ignore-not-found",
          "--wait=true",
        ],
        { quiet: true },
      );
      throw error;
    }
  };

  return {
    name,
    kubernetesVersion: fixturePin.version,
    nodeImage: fixturePin.nodeImage,
    kindBin: tools.kind,
    kubectlBin: tools.kubectl,
    adminKubeconfig,
    hostKubeconfig,
    restrictedKubeconfig,
    namespace: WORKLOAD_NAMESPACE,
    controlNamespace: CONTROL_NAMESPACE,
    image,
    kubectl,
    json,
    podTemplate: (overrides) => podTemplate(image, overrides),
    startInClusterBackend,
    cleanupWorkloads,
    diagnostics: () => kubernetesDiagnostics(tools.kubectl, adminKubeconfig, hostKubeconfig),
    dispose: async () => {
      if (inCluster) await inCluster.context.stop().catch(() => undefined);
      try {
        deleteCluster();
      } finally {
        removeRuntimeImage();
      }
    },
  };
}

/** Decode the listening TCP sockets owned by one Linux process. */
export function processTcpListeners(pid: number): Array<{ address: string; port: number }> {
  const socketInodes = new Set<string>();
  for (const entry of fs.readdirSync(`/proc/${pid}/fd`)) {
    try {
      const target = fs.readlinkSync(`/proc/${pid}/fd/${entry}`);
      const match = target.match(/^socket:\[(\d+)]$/);
      if (match) socketInodes.add(match[1]!);
    } catch {
      // A descriptor can close between readdir and readlink.
    }
  }
  const listeners: Array<{ address: string; port: number }> = [];
  for (const table of ["tcp", "tcp6"]) {
    const rows = fs.readFileSync(`/proc/${pid}/net/${table}`, "utf8").trim().split("\n").slice(1);
    for (const row of rows) {
      const columns = row.trim().split(/\s+/);
      if (columns[3] !== "0A" || !socketInodes.has(columns[9]!)) continue;
      const [encodedAddress, encodedPort] = columns[1]!.split(":");
      const port = Number.parseInt(encodedPort!, 16);
      if (table === "tcp") {
        const bytes = encodedAddress!
          .match(/../g)!
          .reverse()
          .map((part) => Number.parseInt(part, 16));
        listeners.push({ address: bytes.join("."), port });
      } else {
        listeners.push({
          address: encodedAddress === "00000000000000000000000001000000" ? "::1" : encodedAddress!,
          port,
        });
      }
    }
  }
  return listeners;
}
