import path from "node:path";
import { devices, type Browser, type Page } from "@playwright/test";
import { backendFixture as test, type BackendContext } from "../../fixtures/backend";
import { ApiClient } from "../../helpers/api-client";
import { acceptInvite, createInviteToken, setupAdmin } from "../../helpers/auth";
import { expect } from "@playwright/test";

const ADMIN = { email: "k8s-admin@e2e.dev", password: "adminpass123", displayName: "K8s Admin" };
const MEMBER = {
  email: "k8s-member@e2e.dev",
  password: "memberpass123",
  displayName: "K8s Member",
};
const POD_TEMPLATE = `apiVersion: v1
kind: PodTemplate
template:
  metadata:
    annotations:
      example.com/e2e: settings-persistence
  spec:
    containers:
      - name: kandev-agent
        image: example.test/kandev-agent:e2e
`;

async function initPage(page: Page, backend: BackendContext) {
  await page.addInitScript(
    ({ backendPort }: { backendPort: string }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_PORT = backendPort;
    },
    { backendPort: String(backend.port) },
  );
}

async function openContext(browser: Browser, backend: BackendContext) {
  const context = await browser.newContext({
    ...devices["Desktop Chrome"],
    baseURL: backend.frontendUrl,
  });
  const page = await context.newPage();
  await initPage(page, backend);
  return { context, page };
}

async function openCreateFlow(page: Page) {
  await page.goto("/settings/executors");
  const card = page.getByText("Kubernetes", { exact: true }).first();
  await expect(card).toBeVisible();
  await card.click();
  await expect(page).toHaveURL((url) => new URL(url).pathname === "/settings/executors/new/k8s");
}

async function fillCreateForm(page: Page, name: string) {
  await page.getByTestId("kubernetes-executor-name").fill(name);
  await page.getByTestId("kubernetes-namespace").fill("e2e-settings");
  await page.getByTestId("kubernetes-request-timeout").fill("45");
  await page.getByTestId("kubernetes-kubeconfig-path").fill("/tmp/e2e-settings.kubeconfig");
  await page.getByTestId("kubernetes-context").fill("e2e-context");
  await page.locator("#profile-name").fill("Settings profile");
  await page.getByTestId("kubernetes-main-container").fill("kandev-agent");
  await page.getByTestId("kubernetes-pod-template").fill(POD_TEMPLATE);
  await page.getByTestId("kubernetes-workspace-size").fill("2Gi");
  await page.getByTestId("kubernetes-storage-class").fill("standard");
}

async function stubSuccessfulDiagnostics(page: Page) {
  await page.route("**/api/v1/kubernetes/test", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        server_version: "v1.36.1",
        namespace: "e2e-settings",
        steps: [
          { key: "configuration", success: true, duration_ms: 1, detail: "Configuration valid" },
          { key: "discovery", success: true, duration_ms: 2, detail: "API discovered" },
          { key: "rbac", success: true, duration_ms: 3, detail: "Permissions granted" },
        ],
        warnings: [],
      }),
    });
  });
}

async function stubEmptySessions(page: Page) {
  await page.route("**/api/v1/kubernetes/executors/*/sessions", async (route) => {
    await route.fulfill({ status: 200, contentType: "application/json", body: "[]" });
  });
}

async function verifyLeadingProfileClusterExperience(page: Page, profileId: string) {
  await page.goto(`/settings/executors/${profileId}`);
  const clusterSection = page.getByTestId("kubernetes-profile-cluster-section");
  const workloadCard = page.getByTestId("kubernetes-workload-card");
  await expect(clusterSection).toBeVisible();
  await expect(clusterSection.getByTestId("kubernetes-connection-card")).toBeVisible();
  await expect(clusterSection.getByTestId("kubernetes-diagnostics-card")).toBeVisible();
  await expect(clusterSection.getByTestId("kubernetes-sessions-card")).toBeVisible();
  await expect(clusterSection.getByTestId("kubernetes-namespace")).toHaveValue("e2e-settings");
  await expect(
    clusterSection.getByRole("button", { name: /edit cluster connection/i }),
  ).toHaveCount(0);
  const clusterBox = await clusterSection.boundingBox();
  const workloadBox = await workloadCard.boundingBox();
  expect(clusterBox).not.toBeNull();
  expect(workloadBox).not.toBeNull();
  expect(clusterBox!.y).toBeLessThan(workloadBox!.y);

  await clusterSection.getByTestId("kubernetes-namespace").fill("draft-profile-namespace");
  await page.getByTestId("kubernetes-main-container").fill("draft-profile-agent");
  const diagnosticRequest = page.waitForRequest(
    (request) => request.url().endsWith("/api/v1/kubernetes/test") && request.method() === "POST",
  );
  await page
    .getByTestId("kubernetes-profile-cluster-section")
    .getByTestId("kubernetes-test-button")
    .click();
  expect((await diagnosticRequest).postDataJSON()).toMatchObject({
    config: {
      kubeconfig_path: "/tmp/e2e-settings.kubeconfig",
      namespace: "draft-profile-namespace",
    },
    profile_config: { main_container: "draft-profile-agent" },
  });
}

async function seedMemberBoundary(apiClient: ApiClient) {
  const executor = await apiClient.createExecutor("Member-visible Kubernetes", "k8s", {
    auth_mode: "kubeconfig",
    kubeconfig_path: "/tmp/member.kubeconfig",
    namespace: "default",
    request_timeout_seconds: "30",
  });
  const profile = await apiClient.createExecutorProfile(executor.id, {
    name: "Member-visible profile",
    config: {
      platform: "linux/amd64",
      main_container: "kandev-agent",
      pod_template_yaml: POD_TEMPLATE,
      "workspace.mode": "empty_dir",
    },
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
  return { executor, profile };
}

test("administrator tests, saves, and reloads Kubernetes connection and profile settings", async ({
  browser,
  backend,
}) => {
  const apiClient = new ApiClient(backend.baseUrl);
  const { context, page } = await openContext(browser, backend);
  const name = "Persistent Kubernetes settings";
  await stubSuccessfulDiagnostics(page);
  await stubEmptySessions(page);
  try {
    await openCreateFlow(page);
    await fillCreateForm(page, name);

    const diagnosticRequest = page.waitForRequest(
      (request) => request.url().endsWith("/api/v1/kubernetes/test") && request.method() === "POST",
    );
    await page.getByTestId("kubernetes-test-button").click();
    const request = await diagnosticRequest;
    expect(request.postDataJSON()).toMatchObject({
      config: {
        auth_mode: "kubeconfig",
        kubeconfig_path: "/tmp/e2e-settings.kubeconfig",
        kube_context: "e2e-context",
        namespace: "e2e-settings",
        request_timeout_seconds: "45",
      },
      profile_config: {
        main_container: "kandev-agent",
        pod_template_yaml: POD_TEMPLATE,
        "workspace.mode": "managed_pvc",
        "workspace.size": "2Gi",
        "workspace.storage_class": "standard",
      },
    });
    await expect(page.getByTestId("kubernetes-test-result")).toContainText("v1.36.1");
    await expect(page.getByTestId("kubernetes-test-step-rbac")).toBeVisible();

    const saveBar = page.getByTestId("settings-floating-save");
    await expect(saveBar).toHaveAttribute("data-status", "dirty");
    await saveBar.getByRole("button", { name: /save changes/i }).click();
    await expect(page).toHaveURL(/\/settings\/executors\/[^/]+$/);
    const profileId = decodeURIComponent(new URL(page.url()).pathname.split("/").at(-1)!);
    const executor = (await apiClient.listExecutors()).executors.find((row) => row.name === name);
    expect(executor).toBeTruthy();
    await expect
      .poll(async () => (await apiClient.getExecutorProfile(executor!.id, profileId)).name)
      .toBe("Settings profile");
    await expect(page).toHaveURL(`/settings/executors/${profileId}`);

    await page.reload();
    await expect(page.getByTestId("kubernetes-executor-name")).toHaveValue(name);
    await expect(page.getByTestId("kubernetes-namespace")).toHaveValue("e2e-settings");
    await expect(page.getByTestId("kubernetes-kubeconfig-path")).toHaveValue(
      "/tmp/e2e-settings.kubeconfig",
    );
    await expect(page.getByTestId("kubernetes-pod-template")).toHaveValue(POD_TEMPLATE);
    await expect(page.getByTestId("kubernetes-workspace-size")).toHaveValue("2Gi");
    await expect(page.getByTestId("kubernetes-storage-class")).toHaveValue("standard");
    await page.reload();
    await expect(page.getByTestId("kubernetes-pod-template")).toHaveValue(POD_TEMPLATE);
    await verifyLeadingProfileClusterExperience(page, profileId);

    await page.goto(`/settings/executors/k8s/${executor!.id}`);
    await expect(page).toHaveURL(`/settings/executors/${profileId}`);
  } finally {
    const executor = (await apiClient.listExecutors()).executors.find((row) => row.name === name);
    if (executor) await apiClient.deleteExecutor(executor.id).catch(() => undefined);
    await context.close();
  }
});

test("member sees Kubernetes settings but cannot test, mutate, save, or delete them", async ({
  browser,
  backend,
}) => {
  const database = path.join(backend.tmpDir, "kubernetes-member-desktop.db");
  await backend.restart({ KANDEV_DATABASE_PATH: database });
  const apiClient = new ApiClient(backend.baseUrl);
  const seeded = await seedMemberBoundary(apiClient);
  await backend.restart({ KANDEV_DATABASE_PATH: database, KANDEV_FEATURES_AUTH: "true" });

  const adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
  const memberContext = await browser.newContext({
    ...devices["Desktop Chrome"],
    baseURL: backend.frontendUrl,
  });
  try {
    await setupAdmin(adminContext, backend.baseUrl, ADMIN);
    const token = await createInviteToken(adminContext, backend.baseUrl, {
      email: MEMBER.email,
      role: "member",
    });
    await acceptInvite(memberContext, backend.baseUrl, token, MEMBER);
    const page = await memberContext.newPage();
    await initPage(page, backend);
    await stubEmptySessions(page);
    await page.goto(`/settings/executors/k8s/${seeded.executor.id}`);
    await expect(page).toHaveURL(`/settings/executors/${seeded.profile.id}`);

    await expect(page.getByTestId("kubernetes-read-only-notice")).toBeVisible();
    await expect(page.getByTestId("kubernetes-executor-name")).toBeDisabled();
    await expect(page.getByTestId("kubernetes-test-button")).toBeDisabled();
    await expect(page.getByRole("button", { name: /delete profile/i })).toBeDisabled();
    await expect(page.getByTestId("settings-floating-save")).toHaveCount(0);

    const patchResponse = await memberContext.request.patch(
      `${backend.baseUrl}/api/v1/executors/${seeded.executor.id}`,
      { data: { name: "Forbidden rename" } },
    );
    expect(patchResponse.status()).toBe(403);
    const testResponse = await memberContext.request.post(
      `${backend.baseUrl}/api/v1/kubernetes/test`,
      {
        data: {
          config: {
            auth_mode: "kubeconfig",
            kubeconfig_path: "/tmp/member.kubeconfig",
            namespace: "default",
            request_timeout_seconds: "30",
          },
        },
      },
    );
    expect(testResponse.status()).toBe(403);

    await expect(page.getByTestId("kubernetes-pod-template")).toBeDisabled();
    const clusterSection = page.getByTestId("kubernetes-profile-cluster-section");
    await expect(clusterSection.getByTestId("kubernetes-diagnostics-card")).toBeVisible();
    await expect(clusterSection.getByTestId("kubernetes-sessions-card")).toBeVisible();
    await expect(clusterSection.getByTestId("kubernetes-test-button")).toBeDisabled();
    await expect(
      clusterSection.getByRole("button", { name: /view cluster connection/i }),
    ).toHaveCount(0);
  } finally {
    await adminContext.close();
    await memberContext.close();
    await backend.restart();
  }
});

test("profile-less Kubernetes executors retain the standalone recovery page", async ({
  browser,
  backend,
}) => {
  const apiClient = new ApiClient(backend.baseUrl);
  const executor = await apiClient.createExecutor("Orphan Kubernetes recovery", "k8s", {
    auth_mode: "kubeconfig",
    kubeconfig_path: "/tmp/orphan.kubeconfig",
    namespace: "orphan-workloads",
    request_timeout_seconds: "30",
  });
  const { context, page } = await openContext(browser, backend);
  await stubEmptySessions(page);
  try {
    await page.goto(`/settings/executors/k8s/${executor.id}`);

    await expect(page).toHaveURL(`/settings/executors/k8s/${executor.id}`);
    await expect(page.getByTestId("kubernetes-executor-name")).toHaveValue(
      "Orphan Kubernetes recovery",
    );
    await expect(page.getByTestId("kubernetes-namespace")).toHaveValue("orphan-workloads");
    await expect(page.getByRole("button", { name: /delete executor/i })).toBeVisible();
  } finally {
    await apiClient.deleteExecutor(executor.id).catch(() => undefined);
    await context.close();
  }
});
