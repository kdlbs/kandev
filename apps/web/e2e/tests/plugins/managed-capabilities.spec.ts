import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  openInstallDialog,
  PACKAGE_PATH,
  PLUGIN_ID,
  uninstallPluginFixture,
  uploadPackage,
} from "./plugin-test-helpers";

type ApprovalContext = {
  workspace_id: string;
  manifest_digest: string;
  declared_capability_ids: string[];
  approval: { revision: number; capability_ids: string[]; state: string } | null;
  requires_review: boolean;
  audit_events: Array<{ type: string; before_revision: number; after_revision: number }>;
};

async function readApprovalContext(
  apiClient: ApiClient,
  workspaceId: string,
): Promise<ApprovalContext> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/plugins/${PLUGIN_ID}/capability-approvals?workspace_id=${encodeURIComponent(workspaceId)}`,
  );
  if (!response.ok) throw new Error(`GET capability approval failed: ${response.status}`);
  return (await response.json()) as ApprovalContext;
}

async function installFixture(testPage: Parameters<typeof openInstallDialog>[0]) {
  await openInstallDialog(testPage);
  await uploadPackage(testPage, PACKAGE_PATH);
  await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });
  await testPage.goto(`/settings/plugins/${PLUGIN_ID}`);
  await expect(testPage.getByTestId("plugin-capability-approval")).toBeVisible();
}

test.describe("managed plugin capabilities", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallPluginFixture(apiClient);
  });

  test("grant, reject a stale edit, preserve the newer grant, and revoke access", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installFixture(testPage);

    const panel = testPage.getByTestId("plugin-capability-approval");
    const readTasks = panel.getByTestId("plugin-capability-host.v2.read-messages");
    const writeTasks = panel.getByTestId("plugin-capability-host.v2.write-tasks");
    await readTasks.getByRole("checkbox").check();
    await writeTasks.getByRole("checkbox").check();
    await panel.getByTestId("plugin-approval-reason").fill("Allow the fixture to coordinate tasks");
    await panel.getByTestId("plugin-approval-save").click();
    await expect(panel.getByTestId("plugin-approval-status")).toContainText("saved");

    const granted = await readApprovalContext(apiClient, seedData.workspaceId);
    expect(granted.approval).toMatchObject({ revision: 1, state: "active" });
    expect(granted.approval?.capability_ids).toEqual([
      "host.v2.read:messages",
      "host.v2.write:tasks",
    ]);

    await writeTasks.getByRole("checkbox").uncheck();
    await panel.getByTestId("plugin-approval-reason").fill("Remove task writes");
    const competingUpdate = await apiClient.rawRequest(
      "PUT",
      `/api/plugins/${PLUGIN_ID}/capability-approvals`,
      {
        workspace_id: seedData.workspaceId,
        expected_revision: granted.approval!.revision,
        manifest_digest: granted.manifest_digest,
        capability_ids: ["host.v2.write:tasks"],
        reason: "Concurrent manager update",
        audit_id: "e2e-concurrent-update",
      },
    );
    expect(competingUpdate.ok).toBe(true);

    await panel.getByTestId("plugin-approval-save").click();
    await expect(panel.getByTestId("plugin-approval-error")).toBeVisible();
    const preserved = await readApprovalContext(apiClient, seedData.workspaceId);
    expect(preserved.approval).toMatchObject({ revision: 2, state: "active" });
    expect(preserved.approval?.capability_ids).toEqual(["host.v2.write:tasks"]);

    await panel.getByTestId("plugin-approval-revoke").click();
    await expect(panel.getByTestId("plugin-approval-status")).toContainText("revoked");
    const revoked = await readApprovalContext(apiClient, seedData.workspaceId);
    expect(revoked.approval).toMatchObject({ revision: 3, state: "revoked" });
    expect(revoked.audit_events.map((event) => event.type)).toEqual(["grant", "narrow", "revoke"]);
  });

  test("upgrade review asks for explicit reapproval of unchanged access", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installFixture(testPage);
    const before = await readApprovalContext(apiClient, seedData.workspaceId);
    const granted = await apiClient.rawRequest(
      "PUT",
      `/api/plugins/${PLUGIN_ID}/capability-approvals`,
      {
        workspace_id: seedData.workspaceId,
        expected_revision: 0,
        manifest_digest: before.manifest_digest,
        capability_ids: ["host.v2.read:messages"],
        reason: "Initial review",
        audit_id: "e2e-upgrade-initial",
      },
    );
    expect(granted.ok).toBe(true);

    await testPage.route(`**/api/plugins/${PLUGIN_ID}/capability-approvals?*`, async (route) => {
      if (route.request().method() !== "GET") {
        await route.continue();
        return;
      }
      const response = await route.fetch();
      const context = (await response.json()) as ApprovalContext;
      await route.fulfill({
        response,
        json: { ...context, requires_review: true },
      });
    });
    await testPage.reload();

    const panel = testPage.getByTestId("plugin-capability-approval");
    await expect(panel.getByTestId("plugin-approval-upgrade-review")).toBeVisible();
    await panel.getByTestId("plugin-approval-reason").fill("Reviewed the plugin upgrade");
    await expect(panel.getByTestId("plugin-approval-save")).toBeEnabled();
    await panel.getByTestId("plugin-approval-save").click();
    await expect(panel.getByTestId("plugin-approval-status")).toContainText("saved");

    const reapproved = await readApprovalContext(apiClient, seedData.workspaceId);
    expect(reapproved.approval).toMatchObject({ revision: 2, state: "active" });
  });
});
