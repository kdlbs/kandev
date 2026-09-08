import { type Page, test as base } from "@playwright/test";
import { backendFixture, type BackendContext } from "./backend";
import { hasDocker } from "./docker-probe";
import { type KubernetesCluster, provisionKubernetesCluster } from "./kubernetes-tools";
import { ApiClient } from "../helpers/api-client";
import type { WorkflowStep } from "../../lib/types/http";

export type KubernetesSeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  steps: WorkflowStep[];
  agentProfileId: string;
  executorId: string;
  executorProfileId: string;
};

export function kubernetesExecutorConfig(
  cluster: KubernetesCluster,
  overrides: Partial<
    Record<
      "auth_mode" | "kubeconfig_path" | "kube_context" | "namespace" | "request_timeout_seconds",
      string
    >
  > = {},
): Record<string, string> {
  return {
    auth_mode: "kubeconfig",
    kubeconfig_path: cluster.hostKubeconfig,
    namespace: cluster.namespace,
    request_timeout_seconds: "30",
    ...overrides,
  };
}

export function kubernetesProfileConfig(
  cluster: KubernetesCluster,
  overrides: Record<string, string> = {},
): Record<string, string> {
  return {
    platform: "linux/amd64",
    main_container: "kandev-agent",
    pod_template_yaml: cluster.podTemplate(),
    "workspace.mode": "empty_dir",
    ...overrides,
  };
}

/** Seed only the resources needed to launch real Kubernetes-backed tasks. */
export async function seedKubernetesBackend(
  apiClient: ApiClient,
  cluster: KubernetesCluster,
  options: {
    label: string;
    executorConfig?: Record<string, string>;
    profileConfig?: Record<string, string>;
  },
): Promise<KubernetesSeedData> {
  const workspace = await apiClient.createWorkspace(`E2E Kubernetes ${options.label}`);
  const workflow = await apiClient.createWorkflow(
    workspace.id,
    `E2E Kubernetes ${options.label}`,
    "simple",
  );
  const { steps } = await apiClient.listWorkflowSteps(workflow.id);
  const sorted = steps.sort((a, b) => a.position - b.position);
  const startStep = sorted.find((step) => step.is_start_step) ?? sorted[0];
  if (!startStep) throw new Error("Kubernetes E2E seed has no workflow start step");

  const { agents } = await apiClient.listAgents();
  const mock = agents.find((agent) => agent.name === "mock-agent");
  const agentProfileId = mock?.profiles[0]?.id;
  if (!agentProfileId) throw new Error("Kubernetes E2E seed has no mock-agent profile");

  const executor = await apiClient.createExecutor(
    `E2E Kubernetes ${options.label}`,
    "k8s",
    options.executorConfig ?? kubernetesExecutorConfig(cluster),
  );
  const profile = await apiClient.createExecutorProfile(executor.id, {
    name: `E2E Kubernetes ${options.label}`,
    config: options.profileConfig ?? kubernetesProfileConfig(cluster),
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });

  return {
    workspaceId: workspace.id,
    workflowId: workflow.id,
    startStepId: startStep.id,
    steps: sorted,
    agentProfileId,
    executorId: executor.id,
    executorProfileId: profile.id,
  };
}

export const kubernetesTest = backendFixture.extend<
  { testPage: Page; kubernetesCleanup: void },
  { apiClient: ApiClient; cluster: KubernetesCluster; seedData: KubernetesSeedData }
>({
  apiClient: [
    async ({ backend }, use) => {
      await use(new ApiClient(backend.baseUrl));
    },
    { scope: "worker" },
  ],

  cluster: [
    async ({ backend }, use, workerInfo) => {
      if (!hasDocker()) {
        base.skip(true, "Docker daemon not reachable; skipping Kind-backed Kubernetes E2E worker");
        return;
      }
      const cluster = await provisionKubernetesCluster(backend.tmpDir, workerInfo.workerIndex);
      try {
        await use(cluster);
      } catch (error) {
        let diagnostics = "Kubernetes diagnostics unavailable";
        try {
          diagnostics = cluster.diagnostics();
        } catch (diagnosticError) {
          diagnostics += `: ${String(diagnosticError)}`;
        }
        throw new Error(`Kind-backed Kubernetes E2E failed.\n${diagnostics}`, { cause: error });
      } finally {
        await cluster.dispose();
      }
    },
    { scope: "worker", timeout: 600_000 },
  ],

  seedData: [
    async ({ apiClient, cluster }, use) => {
      const seed = await seedKubernetesBackend(apiClient, cluster, { label: "kubeconfig" });
      try {
        await use(seed);
      } finally {
        await apiClient.deleteExecutorProfile(seed.executorProfileId).catch(() => undefined);
        await apiClient.deleteExecutor(seed.executorId).catch(() => undefined);
      }
    },
    { scope: "worker", timeout: 120_000 },
  ],

  kubernetesCleanup: [
    async ({ apiClient, cluster, seedData }, use, testInfo) => {
      const reset = async () => {
        let resetError: unknown;
        try {
          await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
        } catch (error) {
          resetError = error;
        } finally {
          await cluster.cleanupWorkloads();
        }
        if (resetError) {
          // Foreign-resource safety tests intentionally make the backend refuse
          // an owned-name deletion. Remove the isolated test namespace's
          // workloads first, then finish resetting backend state.
          await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
        }
      };
      await reset();
      try {
        await use();
      } finally {
        try {
          if (testInfo.status !== testInfo.expectedStatus) {
            let diagnostics = "Kubernetes diagnostics unavailable";
            try {
              diagnostics = cluster.diagnostics();
            } catch (diagnosticError) {
              diagnostics += `: ${String(diagnosticError)}`;
            }
            await testInfo.attach("kubernetes-cluster-inventory", {
              body: Buffer.from(diagnostics),
              contentType: "text/plain",
            });
          }
        } finally {
          await reset();
        }
      }
    },
    { auto: true, timeout: 180_000 },
  ],

  testPage: async ({ browser, backend }, use) => {
    await backend.ensureReady();
    const context = await browser.newContext({ baseURL: backend.frontendUrl });
    const page = await context.newPage();
    await page.addInitScript(
      ({ backendPort }: { backendPort: string }) => {
        localStorage.setItem("kandev.onboarding.completed", "true");
        window.__KANDEV_API_PORT = backendPort;
      },
      { backendPort: String((backend as BackendContext).port) },
    );
    await use(page);
    await context.close();
  },
});

export { expect } from "@playwright/test";
export const test = kubernetesTest;
export { base };
