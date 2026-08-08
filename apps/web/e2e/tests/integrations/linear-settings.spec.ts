import { test, expect } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { LinearSettingsPage } from "../../pages/linear-settings-page";
import { assertWatcherAgentProfileResetsToStepDefault } from "./watcher-profile-default-flow";
import { assertWatcherDispatchOrderPersists } from "./watcher-dispatch-order-flow";
import { makeGitEnv } from "../../helpers/git-helper";

test.describe("Linear settings", () => {
  test("empty workspace shows form with disabled save/test until secret is filled", async ({
    testPage,
  }) => {
    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.statusBanner).toHaveCount(0);
    await expect(settings.saveButton).toHaveCount(0);
    await expect(settings.testButton).toBeDisabled();

    await settings.secretInput.fill("lin_api_xxx");
    await expect(settings.saveButton).toBeEnabled();
    await expect(settings.testButton).toBeEnabled();
  });

  test("saving the config persists across reload and shows the auth banner", async ({
    testPage,
    apiClient,
  }) => {
    // Seed a single team so the dropdown post-save can populate without an
    // empty-list flash. The default-team field is optional, but the team
    // fetch fires when hasSecret flips true and we want it to succeed.
    await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);

    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await settings.secretInput.fill("lin_api_xxx");
    await settings.saveButton.click();
    await expect(settings.saveButton).toHaveCount(0);
    // Wait for the async post-save probe to write lastOk=true before reloading.
    await apiClient.waitForIntegrationAuthHealthy("linear");

    await testPage.reload();
    await settings.secretInput.waitFor();
    await expect(settings.statusBanner).toHaveAttribute("data-state", "ok");
    // Saved-secret hint indicates the row was loaded, not started fresh.
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toBeVisible();
  });

  test("workspace-scoped route scopes the saved credentials form", async ({
    testPage,
    apiClient,
  }) => {
    const other = await apiClient.createWorkspace("Linear Secondary Workspace");
    await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);

    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await settings.secretInput.fill("lin_api_default");
    await settings.saveButton.click();
    await expect(settings.deleteButton).toBeVisible();
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toBeVisible();

    await settings.gotoWorkspace(other.id);

    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.saveButton).toHaveCount(0);
    await expect(settings.deleteButton).toHaveCount(0);
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toHaveCount(0);
  });

  test("a workspace-scoped deep link adopts that workspace on load", async ({
    testPage,
    apiClient,
  }) => {
    const other = await apiClient.createWorkspace("Linear Deep Link Workspace");
    // Seed the secondary workspace so the deep link lands on a configured row,
    // proving the route path — not the user's global default — drove selection.
    await apiClient.setLinearConfig({ secret: "lin_api_deeplink", workspaceId: other.id });

    const settings = new LinearSettingsPage(testPage);
    await settings.gotoWorkspace(other.id);

    // The seeded row loads, confirming the deep link scoped the form to `other`.
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toBeVisible();
  });

  test("workspace-scoped integration route keeps the workspace in the path", async ({
    testPage,
    apiClient,
  }) => {
    const other = await apiClient.createWorkspace("Linear Path Workspace");

    const settings = new LinearSettingsPage(testPage);
    await settings.gotoWorkspace(other.id);

    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspace/${other.id}/integrations/linear$`),
    );
    await expect(testPage).not.toHaveURL(/[?&]workspace=/);
    await expect(settings.secretInput).toHaveValue("");
  });

  test("copy config duplicates the credentials to another workspace", async ({
    testPage,
    apiClient,
  }) => {
    const other = await apiClient.createWorkspace("Linear Copy Target Workspace");
    await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);

    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    // Configure the source (default) workspace.
    await settings.secretInput.fill("lin_api_source");
    await settings.saveButton.click();
    await expect(settings.deleteButton).toBeVisible();

    // Copy the config to the empty target workspace via the dialog.
    await settings.copyConfigTrigger.click();
    await settings.copyConfigTarget.click();
    await testPage.getByRole("option", { name: new RegExp(other.name) }).click();
    await settings.copyConfigConfirm.click();
    await expect(testPage.getByText(/Copied Linear config/i)).toBeVisible();

    // The target workspace should now report a saved secret via the API.
    const res = await apiClient.rawRequest("GET", `/api/v1/linear/config?workspace_id=${other.id}`);
    expect(res.status).toBe(200);
    const cfg = (await res.json()) as { hasSecret?: boolean };
    expect(cfg.hasSecret).toBe(true);

    // Opening the target workspace route shows the copied credentials loaded.
    await settings.gotoWorkspace(other.id);
    await expect(settings.deleteButton).toBeVisible();
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toBeVisible();
  });

  test("test connection surfaces inline success and failure", async ({ testPage, apiClient }) => {
    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await apiClient.mockLinearSetAuthResult({
      ok: true,
      displayName: "Alice from Linear",
      email: "alice@example.com",
      orgName: "Acme",
    });
    await settings.secretInput.fill("lin_api_xxx");
    await settings.testButton.click();
    await expect(testPage.getByText(/Connected as Alice from Linear/i)).toBeVisible();

    await apiClient.mockLinearSetAuthResult({ ok: false, error: "Bad token" });
    await settings.testButton.click();
    await expect(testPage.getByText(/Failed: Bad token/)).toBeVisible();
  });

  test("seeded auth-health failure renders the failed banner on load", async ({
    testPage,
    apiClient,
  }) => {
    const settings = new LinearSettingsPage(testPage);
    await settings.goto();
    await settings.secretInput.fill("lin_api_xxx");
    await settings.saveButton.click();
    // Wait for the post-save probe to land BEFORE forcing the failure: the
    // probe goroutine could otherwise overwrite our forced lastOk=false back
    // to true a few ms after the mockLinearSetAuthHealth call.
    await apiClient.waitForIntegrationAuthHealthy("linear");

    await apiClient.mockLinearSetAuthHealth({
      ok: false,
      error: "rate limited",
    });
    await testPage.reload();
    await settings.statusBanner.waitFor();
    await expect(settings.statusBanner).toHaveAttribute("data-state", "failed");
    await expect(settings.statusBanner).toContainText(/rate limited/i);
  });

  test("delete clears the saved configuration", async ({ testPage }) => {
    const settings = new LinearSettingsPage(testPage);
    await settings.goto();
    await settings.secretInput.fill("lin_api_xxx");
    await settings.saveButton.click();
    await expect(settings.deleteButton).toBeVisible();

    testPage.once("dialog", (d) => void d.accept());
    await settings.deleteButton.click();
    await expect(settings.deleteButton).toHaveCount(0);
    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.statusBanner).toHaveCount(0);
  });

  // Regression test for #1107: passthrough profiles were silently filtered
  // out of the watcher dialog after #805 — once #923 made auto-start viable,
  // this filter became stale and hid Claude Code / Codex / Copilot CLI from
  // the Agent Profile selector. The dropdown must now list them.
  test("watcher dialog lists CLI-passthrough agent profiles", async ({ testPage, apiClient }) => {
    await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
    await apiClient.waitForIntegrationAuthHealthy("linear");

    const { agents } = await apiClient.listAgents();
    if (agents.length === 0) throw new Error("no agents registered in this e2e profile");
    const passthroughName = "Watcher CLI Passthrough";
    await apiClient.createAgentProfile(agents[0].id, passthroughName, {
      model: "mock-fast",
      cli_passthrough: true,
    });

    await testPage.goto("/settings/integrations/linear");
    await testPage.getByRole("button", { name: /new watcher/i }).click();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();

    // Reach the Agent Profile combobox via its <label> parent so we don't
    // collide with the other comboboxes (workspace / workflow / executor)
    // rendered in the same dialog.
    const trigger = dialog
      .getByText("Agent Profile", { exact: true })
      .locator("xpath=..")
      .getByRole("combobox");
    await trigger.click();

    // Radix portals the listbox to the document root, so search the page (not
    // the dialog) for the option. Substring match on the profile name handles
    // the "<agent> • <profile>" label format.
    await expect(testPage.getByRole("option", { name: new RegExp(passthroughName) })).toBeVisible();
  });

  test("watcher dialog resets the agent profile back to the step default", async ({ testPage }) => {
    await assertWatcherAgentProfileResetsToStepDefault(testPage);
  });

  test("watcher dialog persists the dispatch order across save and reopen", async ({
    testPage,
    apiClient,
  }) => {
    await assertWatcherDispatchOrderPersists(testPage, apiClient);
  });

  test("watcher dialog selects and persists multiple repositories", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await apiClient.mockLinearReset();
    await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);
    await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
    await apiClient.waitForIntegrationAuthHealthy("linear");

    // The fixture seeds one workspace repository ("E2E Repo"); add a second so
    // the picker has two options. The backend requires registered local repos
    // to exist and be valid git repositories (canonicalRepositoryLocalPath
    // stats + git-validates the path), so create a real one under the
    // backend's tmp dir (also inside discoveryRoots, so branch listing works).
    const secondRepoDir = path.join(backend.tmpDir, "repos", "e2e-repo-b");
    fs.rmSync(secondRepoDir, { recursive: true, force: true });
    fs.mkdirSync(secondRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: secondRepoDir, env: gitEnv });
    fs.writeFileSync(path.join(secondRepoDir, "README.md"), "second repo\n");
    execSync("git add README.md", { cwd: secondRepoDir, env: gitEnv });
    execSync('git commit -m "init"', { cwd: secondRepoDir, env: gitEnv });
    const secondRepo = await apiClient.createRepository(
      seedData.workspaceId,
      secondRepoDir,
      "main",
      { name: "E2E Repo B" },
    );
    const firstRepoId = seedData.repositoryId;

    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await testPage.getByRole("button", { name: /new watcher/i }).click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();

    // Scopes a Radix Select trigger by its field label (same shape as the
    // dispatch-order flow helper).
    const comboboxByLabel = (root: Locator, label: string) =>
      root.getByText(label, { exact: true }).locator("xpath=..").getByRole("combobox");
    const pick = async (label: string, option: string | RegExp) => {
      await comboboxByLabel(dialog, label).click();
      await testPage.getByRole("listbox").getByRole("option", { name: option }).click();
    };

    // Minimum fields to enable Save: workspace, non-empty filter (team),
    // workflow, and workflow step. Prompt is pre-filled.
    await pick("Workspace", "E2E Workspace");
    await pick("Team", /ENG/);
    await pick("Workflow", "E2E Workflow");
    await comboboxByLabel(dialog, "Workflow Step").click();
    await testPage.getByRole("listbox").getByRole("option").first().click();

    // Add both repositories via the add control, and pin a branch on the first.
    await dialog.getByTestId("add-repository-trigger").click();
    await testPage
      .getByRole("listbox")
      .getByRole("option", { name: "E2E Repo", exact: true })
      .click();
    await dialog.getByTestId("add-repository-trigger").click();
    await testPage
      .getByRole("listbox")
      .getByRole("option", { name: "E2E Repo B", exact: true })
      .click();
    await dialog.getByTestId(`branch-trigger-${firstRepoId}`).click();
    await testPage.getByRole("listbox").getByRole("option", { name: "main" }).click();

    const createButton = dialog.getByRole("button", { name: "Create" });
    await expect(createButton).toBeEnabled();
    await createButton.click();
    await expect(dialog).toBeHidden();

    // The stored watch carries both bindings in the added order; the second
    // repo's empty branch was resolved to its default ("main") at save.
    const res = await apiClient.rawRequest(
      "GET",
      `/api/v1/linear/watches/issue?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
    );
    const { watches } = (await res.json()) as {
      watches: Array<{
        id: string;
        repositories?: Array<{ repositoryId: string; baseBranch: string }>;
      }>;
    };
    const saved = watches[watches.length - 1];
    expect(saved.repositories).toEqual([
      { repositoryId: firstRepoId, baseBranch: "main" },
      { repositoryId: secondRepo.id, baseBranch: "main" },
    ]);

    // Reopen the saved watcher: both rows render with the saved branches.
    await testPage.getByText("team:ENG").first().click();
    const editDialog = testPage.getByRole("dialog");
    await expect(editDialog.getByText("Edit Linear Watcher")).toBeVisible();
    await expect(editDialog.getByText("E2E Repo", { exact: true })).toBeVisible();
    await expect(editDialog.getByText("E2E Repo B", { exact: true })).toBeVisible();
    await expect(editDialog.getByTestId(`branch-trigger-${firstRepoId}`)).toContainText("main");
    // The second repo's branch was resolved to its default at save; it must
    // render even though its branch fetch returns nothing (no repo on disk).
    await expect(editDialog.getByTestId(`branch-trigger-${secondRepo.id}`)).toContainText("main");
  });

  test("watcher saved without touching the repository picker stays unbound", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockLinearReset();
    await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);
    await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
    await apiClient.waitForIntegrationAuthHealthy("linear");

    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await testPage.getByRole("button", { name: /new watcher/i }).click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();

    const comboboxByLabel = (root: Locator, label: string) =>
      root.getByText(label, { exact: true }).locator("xpath=..").getByRole("combobox");
    const pick = async (label: string, option: string | RegExp) => {
      await comboboxByLabel(dialog, label).click();
      await testPage.getByRole("listbox").getByRole("option", { name: option }).click();
    };

    await pick("Workspace", "E2E Workspace");
    await pick("Team", /ENG/);
    await pick("Workflow", "E2E Workflow");
    await comboboxByLabel(dialog, "Workflow Step").click();
    await testPage.getByRole("listbox").getByRole("option").first().click();

    await dialog.getByRole("button", { name: "Create" }).click();
    await expect(dialog).toBeHidden();

    // The stored watch must carry no repository binding — the historical
    // repo-less behaviour (unbound tasks are pinned at the source layer).
    const res = await apiClient.rawRequest(
      "GET",
      `/api/v1/linear/watches/issue?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
    );
    const { watches } = (await res.json()) as {
      watches: Array<{
        id: string;
        repositories?: Array<{ repositoryId: string; baseBranch: string }>;
      }>;
    };
    const saved = watches[watches.length - 1];
    expect(saved.repositories).toBeUndefined();
  });
});
