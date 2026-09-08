import path from "node:path";
import { devices, type Browser, type Locator, type Page } from "@playwright/test";
import { backendFixture as test, type BackendContext } from "../../fixtures/backend";
import { ApiClient } from "../../helpers/api-client";
import { acceptInvite, createInviteToken, setupAdmin } from "../../helpers/auth";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { expect } from "@playwright/test";

const ADMIN = {
  email: "mobile-k8s-admin@e2e.dev",
  password: "adminpass123",
  displayName: "Mobile K8s Admin",
};
const MEMBER = {
  email: "mobile-k8s-member@e2e.dev",
  password: "memberpass123",
  displayName: "Mobile K8s Member",
};
const LONG_VALUE = "x".repeat(360);
const POD_TEMPLATE = `apiVersion: v1
kind: PodTemplate
template:
  metadata:
    annotations:
      example.com/long-value: "${LONG_VALUE}"
  spec:
    containers:
      - name: kandev-agent
        image: example.test/kandev-agent:e2e
`;
const SHORT_POD_TEMPLATE = "apiVersion: v1\nkind: PodTemplate\n";
const TALL_POD_TEMPLATE = `${POD_TEMPLATE}${Array.from(
  { length: 24 },
  (_, index) => `# autosize-line-${index + 1}`,
).join("\n")}`;

async function initPage(page: Page, backend: BackendContext) {
  await page.addInitScript(
    ({ backendPort }: { backendPort: string }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_PORT = backendPort;
    },
    { backendPort: String(backend.port) },
  );
}

async function openMobileContext(browser: Browser, backend: BackendContext) {
  const context = await browser.newContext({ ...devices["Pixel 5"], baseURL: backend.frontendUrl });
  const page = await context.newPage();
  await initPage(page, backend);
  expect((await page.viewportSize())?.width).toBe(393);
  return { context, page };
}

async function expectTouchTarget(page: Page, name: RegExp) {
  const button = page.getByRole("button", { name }).first();
  await expectTouchLocator(button, String(name));
  return button;
}

async function expectTouchLocator(locator: Locator, label: string) {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  expect(box, `${label} must have geometry`).not.toBeNull();
  expect(box!.height, `${label} must be at least 44px tall`).toBeGreaterThanOrEqual(44);
}

async function stubEmptySessions(page: Page) {
  await page.route("**/api/v1/kubernetes/executors/*/sessions", async (route) => {
    await route.fulfill({ status: 200, contentType: "application/json", body: "[]" });
  });
}

async function expectLeadingProfileClusterSection(page: Page, canManage: boolean) {
  const section = page.getByTestId("kubernetes-profile-cluster-section");
  const workload = page.getByTestId("kubernetes-workload-card");
  await expect(section).toBeVisible();
  await expect(section.getByTestId("kubernetes-connection-card")).toBeVisible();
  await expect(section.getByTestId("kubernetes-diagnostics-card")).toBeVisible();
  await expect(section.getByTestId("kubernetes-sessions-card")).toBeVisible();
  const sectionBox = await section.boundingBox();
  const workloadBox = await workload.boundingBox();
  expect(sectionBox).not.toBeNull();
  expect(workloadBox).not.toBeNull();
  expect(sectionBox!.y).toBeLessThan(workloadBox!.y);
  await expect(
    section.getByRole("button", {
      name: canManage ? /edit cluster connection/i : /view cluster connection/i,
    }),
  ).toHaveCount(0);
  await expectTouchLocator(
    section.getByTestId("kubernetes-namespace"),
    "Kubernetes profile namespace field",
  );
  await expectTouchLocator(
    section.getByTestId("kubernetes-test-button"),
    "Kubernetes profile test button",
  );
  await expectTouchLocator(
    section.getByTestId("kubernetes-sessions-card").getByRole("button", { name: /refresh/i }),
    "Kubernetes sessions refresh button",
  );
}

async function expectPodTemplateAutoSizes(page: Page, yaml: Locator) {
  await yaml.fill(SHORT_POD_TEMPLATE);
  await expect.poll(async () => (await yaml.boundingBox())?.height ?? 0).toBeLessThanOrEqual(140);
  const shortHeight = (await yaml.boundingBox())!.height;

  await yaml.fill(TALL_POD_TEMPLATE);
  await expect
    .poll(async () => (await yaml.boundingBox())?.height ?? 0)
    .toBeGreaterThan(shortHeight + 100);
  const tallHeight = (await yaml.boundingBox())!.height;
  expect(await yaml.evaluate((element) => getComputedStyle(element).overflowY)).toBe("hidden");
  expect(await yaml.evaluate((element) => element.scrollHeight <= element.clientHeight + 2)).toBe(
    true,
  );
  expect(await yaml.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
  await assertNoDocumentHorizontalOverflow(page, "mobile Kubernetes auto-sized YAML");

  await yaml.fill(SHORT_POD_TEMPLATE);
  await expect
    .poll(async () => (await yaml.boundingBox())?.height ?? Number.POSITIVE_INFINITY)
    .toBeLessThan(tallHeight);
}

async function seedMemberBoundary(apiClient: ApiClient) {
  const executor = await apiClient.createExecutor("Mobile member Kubernetes", "k8s", {
    auth_mode: "kubeconfig",
    kubeconfig_path: "/tmp/mobile-member.kubeconfig",
    namespace: "default",
    request_timeout_seconds: "30",
  });
  const profile = await apiClient.createExecutorProfile(executor.id, {
    name: "Mobile member profile",
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

test("administrator creates, tests, saves, and reloads contained Kubernetes YAML by touch", async ({
  browser,
  backend,
}) => {
  const apiClient = new ApiClient(backend.baseUrl);
  const { context, page } = await openMobileContext(browser, backend);
  const name = "Mobile persistent Kubernetes";
  await stubEmptySessions(page);
  await page.route("**/api/v1/kubernetes/test", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        success: true,
        server_version: "v1.36.1",
        namespace: "mobile-e2e",
        steps: [
          { key: "configuration", success: true, duration_ms: 1, detail: "Configuration valid" },
          { key: "streaming", success: true, duration_ms: 4, detail: "Exec and streaming valid" },
        ],
        warnings: [],
      }),
    });
  });
  try {
    await page.goto("/settings/executors");
    const card = page.getByText("Kubernetes", { exact: true }).first();
    await expect(card).toBeVisible();
    await card.tap();
    await expect(page).toHaveURL((url) => new URL(url).pathname === "/settings/executors/new/k8s");

    await page.getByTestId("kubernetes-executor-name").fill(name);
    await page.getByTestId("kubernetes-namespace").fill("mobile-e2e");
    await page.getByTestId("kubernetes-kubeconfig-path").fill("/tmp/mobile-e2e.kubeconfig");
    await page.getByTestId("kubernetes-context").fill("mobile-context");
    await page.locator("#profile-name").fill("Mobile profile");
    const yaml = page.getByTestId("kubernetes-pod-template");
    await yaml.fill(POD_TEMPLATE);
    await page.getByTestId("kubernetes-workspace-size").fill("3Gi");
    await page.getByTestId("kubernetes-storage-class").fill("standard");

    const yamlBox = await yaml.boundingBox();
    const cardBox = await page.getByTestId("kubernetes-workload-card").boundingBox();
    expect(yamlBox).not.toBeNull();
    expect(cardBox).not.toBeNull();
    expect(yamlBox!.x).toBeGreaterThanOrEqual(cardBox!.x);
    expect(yamlBox!.x + yamlBox!.width).toBeLessThanOrEqual(cardBox!.x + cardBox!.width + 1);
    expect(await yaml.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
    await assertNoDocumentHorizontalOverflow(page, "mobile Kubernetes create YAML");

    const testButton = page.getByTestId("kubernetes-test-button");
    await expectTouchLocator(testButton, "Kubernetes test button");
    const diagnosticRequest = page.waitForRequest(
      (request) => request.url().endsWith("/api/v1/kubernetes/test") && request.method() === "POST",
    );
    await testButton.tap();
    const request = await diagnosticRequest;
    expect(request.postDataJSON()).toMatchObject({
      config: { namespace: "mobile-e2e", kubeconfig_path: "/tmp/mobile-e2e.kubeconfig" },
      profile_config: { pod_template_yaml: POD_TEMPLATE, "workspace.size": "3Gi" },
    });
    await expect(page.getByTestId("kubernetes-test-step-streaming")).toBeVisible();

    await expectTouchTarget(page, /^reset$/i);
    const saveButton = await expectTouchTarget(page, /save changes/i);
    await saveButton.tap();

    await expect(page).toHaveURL(/\/settings\/executors\/[^/]+$/);
    const profileId = decodeURIComponent(new URL(page.url()).pathname.split("/").at(-1)!);
    const executor = (await apiClient.listExecutors()).executors.find((row) => row.name === name);
    expect(executor).toBeTruthy();
    await expect
      .poll(async () => (await apiClient.getExecutorProfile(executor!.id, profileId)).name)
      .toBe("Mobile profile");
    await expect(page).toHaveURL(`/settings/executors/${profileId}`);
    await expectTouchTarget(page, /delete profile/i);
    await assertNoDocumentHorizontalOverflow(page, "mobile Kubernetes saved profile");
    await page.reload();
    await expect(page.getByTestId("kubernetes-executor-name")).toHaveValue(name);
    await expect(page.getByTestId("kubernetes-pod-template")).toHaveValue(POD_TEMPLATE);
    await expect(page.getByTestId("kubernetes-workspace-size")).toHaveValue("3Gi");
    await expectLeadingProfileClusterSection(page, true);
    await assertNoDocumentHorizontalOverflow(page, "mobile Kubernetes reloaded profile");

    const clusterSection = page.getByTestId("kubernetes-profile-cluster-section");
    await clusterSection.getByTestId("kubernetes-namespace").fill("mobile-draft-namespace");
    await page.getByTestId("kubernetes-main-container").fill("mobile-draft-container");
    const profileDiagnosticRequest = page.waitForRequest(
      (request) => request.url().endsWith("/api/v1/kubernetes/test") && request.method() === "POST",
    );
    await page
      .getByTestId("kubernetes-profile-cluster-section")
      .getByTestId("kubernetes-test-button")
      .tap();
    expect((await profileDiagnosticRequest).postDataJSON()).toMatchObject({
      config: {
        namespace: "mobile-draft-namespace",
        kubeconfig_path: "/tmp/mobile-e2e.kubeconfig",
      },
      profile_config: { main_container: "mobile-draft-container" },
    });
    await page.reload();
    await expect(page.getByTestId("kubernetes-pod-template")).toHaveValue(POD_TEMPLATE);
    await expectPodTemplateAutoSizes(page, page.getByTestId("kubernetes-pod-template"));
    await assertNoDocumentHorizontalOverflow(page, "mobile Kubernetes profile after reload");
  } finally {
    const executor = (await apiClient.listExecutors()).executors.find((row) => row.name === name);
    if (executor) await apiClient.deleteExecutor(executor.id).catch(() => undefined);
    await context.close();
  }
});

test("member boundaries stay read-only and touch-visible on mobile", async ({
  browser,
  backend,
}) => {
  const database = path.join(backend.tmpDir, "kubernetes-member-mobile.db");
  await backend.restart({ KANDEV_DATABASE_PATH: database });
  const apiClient = new ApiClient(backend.baseUrl);
  const seeded = await seedMemberBoundary(apiClient);
  await backend.restart({ KANDEV_DATABASE_PATH: database, KANDEV_FEATURES_AUTH: "true" });

  const adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
  const memberContext = await browser.newContext({
    ...devices["Pixel 5"],
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
    expect((await page.viewportSize())?.width).toBe(393);
    await page.goto(`/settings/executors/k8s/${seeded.executor.id}`);
    await expect(page).toHaveURL(`/settings/executors/${seeded.profile.id}`);

    await expect(page.getByTestId("kubernetes-read-only-notice")).toBeVisible();
    await expect(page.getByTestId("kubernetes-executor-name")).toBeDisabled();
    const testButton = page.getByTestId("kubernetes-test-button");
    await expect(testButton).toBeDisabled();
    await expectTouchLocator(testButton, "Disabled Kubernetes test button");
    const deleteButton = page.getByRole("button", { name: /delete profile/i });
    await expect(deleteButton).toBeDisabled();
    await expectTouchLocator(deleteButton, "Disabled delete executor button");
    await expect(page.getByTestId("settings-floating-save")).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(page, "mobile Kubernetes member profile");

    const patchResponse = await memberContext.request.patch(
      `${backend.baseUrl}/api/v1/executors/${seeded.executor.id}`,
      { data: { name: "Forbidden mobile rename" } },
    );
    expect(patchResponse.status()).toBe(403);
    const testResponse = await memberContext.request.post(
      `${backend.baseUrl}/api/v1/kubernetes/test`,
      {
        data: {
          config: {
            auth_mode: "kubeconfig",
            kubeconfig_path: "/tmp/mobile-member.kubeconfig",
            namespace: "default",
            request_timeout_seconds: "30",
          },
        },
      },
    );
    expect(testResponse.status()).toBe(403);

    await expectLeadingProfileClusterSection(page, false);
    const yaml = page.getByTestId("kubernetes-pod-template");
    await expect(yaml).toBeDisabled();
    const yamlBox = await yaml.boundingBox();
    expect(yamlBox).not.toBeNull();
    expect(yamlBox!.x + yamlBox!.width).toBeLessThanOrEqual(393);
    await assertNoDocumentHorizontalOverflow(page, "mobile Kubernetes member profile");
    await expect(
      page
        .getByTestId("kubernetes-profile-cluster-section")
        .getByRole("button", { name: /view cluster connection/i }),
    ).toHaveCount(0);
  } finally {
    await adminContext.close();
    await memberContext.close();
    await backend.restart();
  }
});

test("active session cards expose task and session identities without a cluster", async ({
  browser,
  backend,
}) => {
  const apiClient = new ApiClient(backend.baseUrl);
  const executor = await apiClient.createExecutor("Mobile session projection", "k8s", {
    auth_mode: "kubeconfig",
    kubeconfig_path: "/tmp/mobile-sessions.kubeconfig",
    namespace: "default",
    request_timeout_seconds: "30",
  });
  const taskId = "task-mobile-123456789";
  const sessionId = "session-mobile-987654321";
  const { context, page } = await openMobileContext(browser, backend);
  await page.route(`**/api/v1/kubernetes/executors/${executor.id}/sessions`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify([
        {
          task_id: taskId,
          session_id: sessionId,
          pod_name: "kandev-mobile-session",
          pod_phase: "Running",
          container_state: "running",
          restarts: 0,
          workspace_kind: "empty_dir",
          created_at: "2026-08-24T10:00:00Z",
        },
      ]),
    });
  });
  try {
    await page.goto(`/settings/executors/k8s/${executor.id}`);

    const sessions = page.getByTestId("kubernetes-mobile-session-list");
    await expect(sessions).toContainText(taskId);
    await expect(sessions).toContainText(sessionId);
    await assertNoDocumentHorizontalOverflow(page, "mobile Kubernetes active sessions");
  } finally {
    await apiClient.deleteExecutor(executor.id).catch(() => undefined);
    await context.close();
  }
});
