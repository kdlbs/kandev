import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { enableCoordinatorFeature } from "./coordinator-fixture";

function settingsSidebar(page: import("@playwright/test").Page) {
  return page.getByTestId("app-sidebar-settings-mode");
}

async function createDedicatedAgentProfile(
  apiClient: import("../../helpers/api-client").ApiClient,
  name: string,
  cliPassthrough: boolean,
): Promise<string> {
  const { agents } = await apiClient.listAgents();
  const agent = agents[0];
  if (!agent) throw new Error("No E2E agent registered");
  const profile = await apiClient.createAgentProfile(agent.id, name, {
    model: "mock-fast",
    cli_passthrough: cliPassthrough,
  });
  return profile.id;
}

test.describe("Coordinators settings tab", () => {
  test("shows the tab after Secrets and is found by settings search (AC-004.1)", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const releaseFeature = await enableCoordinatorFeature(backend, apiClient, seedData.workspaceId);

    try {
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/secrets`);
      const tabs = testPage.getByTestId("workspace-settings-tabs");
      await expect(tabs).toBeVisible({ timeout: 15_000 });
      const tabLabels = await tabs.getByRole("link").allTextContents();
      const secretsIndex = tabLabels.findIndex((label) => label.includes("Secrets"));
      const coordinatorsIndex = tabLabels.findIndex((label) => label.includes("Coordinators"));
      expect(secretsIndex).toBeGreaterThanOrEqual(0);
      expect(coordinatorsIndex).toBe(tabLabels.length - 1);
      expect(coordinatorsIndex).toBeGreaterThan(secretsIndex);

      await tabs.getByRole("link", { name: "Coordinators", exact: true }).click();
      await expect(testPage).toHaveURL(
        new RegExp(`/settings/workspaces/${seedData.workspaceId}/coordinators$`),
      );
      await expect(testPage.getByTestId("coordinators-list-page")).toBeVisible();

      await testPage.goto("/settings/preferences/appearance");
      const sidebar = settingsSidebar(testPage);
      const search = sidebar.getByRole("searchbox", { name: "Search settings" });
      await search.fill("Coordinators");
      const result = sidebar
        .getByRole("link")
        .filter({ hasText: /^Coordinators/ })
        .first();
      await expect(result).toBeVisible();
      await result.click();
      await expect(testPage).toHaveURL(
        new RegExp(`/settings/workspaces/${seedData.workspaceId}/coordinators$`),
      );
    } finally {
      await releaseFeature();
    }
  });

  test("adds, edits and deletes a coordinator through the settings tab (AC-004.2-004.5, AC-005.1)", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    const releaseFeature = await enableCoordinatorFeature(backend, apiClient, seedData.workspaceId);

    const dedicatedAgentProfileId = await createDedicatedAgentProfile(
      apiClient,
      "Coordinator E2E Agent",
      false,
    );
    const passthroughAgentProfileId = await createDedicatedAgentProfile(
      apiClient,
      "Coordinator E2E Passthrough Agent",
      true,
    );

    const { executors } = await apiClient.listExecutors();
    let executorProfileName: string | undefined;
    for (const executor of executors) {
      const match = executor.profiles?.find((p) => p.id === seedData.worktreeExecutorProfileId);
      if (match) {
        executorProfileName = match.name;
        break;
      }
    }
    if (!executorProfileName) {
      throw new Error("Could not resolve seeded worktree executor profile name");
    }

    try {
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/coordinators`);
      await expect(testPage.getByTestId("coordinators-empty-state")).toBeVisible({
        timeout: 15_000,
      });
      await expect(testPage.getByTestId("coordinators-later-phase-note")).toBeVisible();

      await testPage.getByTestId("add-coordinator-button").click();
      await expect(testPage).toHaveURL(
        new RegExp(`/settings/workspaces/${seedData.workspaceId}/coordinators/new$`),
      );
      const submit = testPage.getByTestId("add-coordinator-submit");
      await expect(submit).toBeDisabled();

      // Passthrough profile is disabled with a reason tooltip (AC-004.3).
      await testPage.getByTestId("coordinator-agent-profile-picker").click();
      const passthroughOption = testPage.getByRole("option", {
        name: /Coordinator E2E Passthrough Agent/,
      });
      await expect(passthroughOption).toHaveAttribute("aria-disabled", "true");
      // The disabled cmdk item has `pointer-events: none`, so hovering it
      // directly would hit its wrapping Radix tooltip trigger anyway (and
      // Playwright's actionability check flags that as "intercepted").
      // Hover the wrapper itself, which is the actual tooltip trigger.
      const passthroughTrigger = testPage
        .locator('[data-slot="tooltip-trigger"]')
        .filter({ has: passthroughOption });
      await passthroughTrigger.hover();
      await expect(
        testPage.getByText("Uses CLI passthrough, which a coordinator can't use", { exact: true }),
      ).toBeVisible();

      // Select the enabled profile from the same, still-open dropdown.
      await testPage.getByRole("option", { name: /Coordinator E2E Agent$/ }).click();
      await expect(submit).toBeDisabled();

      await testPage.getByTestId("executor-profile-selector").click();
      await testPage.getByRole("option", { name: executorProfileName, exact: true }).click();
      await expect(submit).toBeDisabled();

      await testPage.getByLabel("Name").fill("Release Coordinator");
      await expect(submit).toBeEnabled();

      const created = waitForHttp(
        testPage,
        "POST",
        new RegExp(`/api/v1/workspaces/${seedData.workspaceId}/coordinators$`),
      );
      await submit.click();
      const createdResponse = await created;
      const createdBody = (await createdResponse.json()) as { id: string };

      await expect(testPage).toHaveURL(
        new RegExp(`/settings/workspaces/${seedData.workspaceId}/coordinators/${createdBody.id}$`),
      );
      await expect(testPage.getByTestId("coordinator-editor-page")).toBeVisible();
      await expect(testPage.getByTestId("all-coordinators-link")).toBeVisible();
      await expect(
        testPage.getByText(
          "Changing the context, agent profile or executor starts the next conversation fresh.",
          { exact: true },
        ),
      ).toBeVisible();
      await expect(
        testPage.getByText("The profile's auto-approve setting is ignored for coordinators.", {
          exact: true,
        }),
      ).toBeVisible();

      // Edit the context and save via the shared settings save bar (AC-004.4).
      const contextField = testPage.getByLabel("Context");
      await contextField.fill("Watch the release board for blocked cards.");
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await expect(floatingSave).toBeVisible();
      const patched = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/workspaces/${seedData.workspaceId}/coordinators/${createdBody.id}$`),
      );
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await patched;
      await expect(floatingSave).not.toBeVisible();

      // Flip the dedicated agent profile to passthrough behind the coordinator's
      // back, then reload to confirm the profile-status warning (AC-005.1).
      await apiClient.updateAgentProfile(dedicatedAgentProfileId, { cli_passthrough: true });
      await testPage.reload();
      await expect(testPage.getByTestId("coordinator-agent-profile-status")).toHaveText(
        "The agent profile uses CLI passthrough, which a coordinator can't use. Choose another.",
      );

      // Delete confirmation: cancel first, then confirm (AC-004.5).
      await testPage.getByTestId("delete-coordinator-button").click();
      const dialog = testPage.getByTestId("coordinator-delete-confirm-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog).toContainText("Release Coordinator");
      await dialog.getByRole("button", { name: "Cancel" }).click();
      await expect(dialog).not.toBeVisible();
      await expect(testPage.getByTestId("coordinator-editor-page")).toBeVisible();

      await testPage.getByTestId("delete-coordinator-button").click();
      const deleted = waitForHttp(
        testPage,
        "DELETE",
        new RegExp(`/api/v1/workspaces/${seedData.workspaceId}/coordinators/${createdBody.id}$`),
      );
      await testPage.getByTestId("coordinator-delete-confirm").click();
      await deleted;

      await expect(testPage).toHaveURL(
        new RegExp(`/settings/workspaces/${seedData.workspaceId}/coordinators$`),
      );
      await expect(testPage.getByTestId("coordinators-empty-state")).toBeVisible();
    } finally {
      await apiClient.deleteAgentProfile(dedicatedAgentProfileId, true).catch(() => undefined);
      await apiClient.deleteAgentProfile(passthroughAgentProfileId, true).catch(() => undefined);
      await releaseFeature();
    }
  });
});
