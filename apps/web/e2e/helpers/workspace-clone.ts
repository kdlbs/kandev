import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import type { ApiClient } from "./api-client";
import { MobileGitHubPage } from "../pages/mobile-github-page";

export async function seedCloneSource(api: ApiClient, path: string) {
  const source = await api.createWorkspace("Clone source");
  const repo = await api.rawRequest("POST", `/api/v1/workspaces/${source.id}/repositories`, {
    workspace_id: source.id,
    name: "Copied repository",
    source_type: "local",
    local_path: path,
  });
  expect(repo.ok, await repo.text()).toBe(true);
  await api.mockGitHubSetWorkspaceConnection(source.id, {
    source: "gh_cli",
    status: "active",
    login: "test-user",
  });
  const presets = [
    {
      id: "clone-pr",
      kind: "pr",
      label: "Cloned PR default",
      customQuery: "author:@me is:open",
      repoFilter: "",
      createdAt: "2026-10-07T00:00:00Z",
      isDefault: true,
    },
    {
      id: "clone-issue",
      kind: "issue",
      label: "Cloned issue default",
      customQuery: "assignee:@me is:open",
      repoFilter: "",
      createdAt: "2026-10-07T00:00:00Z",
      isDefault: true,
    },
  ];
  const settings = await api.rawRequest("PUT", "/api/v1/github/workspace-settings", {
    workspace_id: source.id,
    saved_presets: presets,
    default_query_presets: { pr: [], issue: [] },
  });
  expect(settings.ok).toBe(true);
  const workflows = await api.listWorkflows(source.id);
  await api.createTask(source.id, "Source history", { workflow_id: workflows.workflows[0].id });
  return source;
}

export async function proveClone(
  page: Page,
  api: ApiClient,
  sourceId: string,
  targetId: string,
  mobile = false,
) {
  const repositories = await api.listRepositories(targetId);
  expect(repositories.repositories).toHaveLength(1);
  expect(repositories.repositories[0].name).toBe("Copied repository");
  const sourceRepos = await api.listRepositories(sourceId);
  expect(repositories.repositories[0].id).not.toBe(sourceRepos.repositories[0].id);
  const workflows = await api.listWorkflows(targetId);
  const sourceWorkflows = await api.listWorkflows(sourceId);
  expect(workflows.workflows).toHaveLength(sourceWorkflows.workflows.length);
  expect(workflows.workflows[0].id).not.toBe(sourceWorkflows.workflows[0].id);
  const tasks = await api.listTasks(targetId);
  expect(tasks.tasks).toHaveLength(0);
  const settings = await api.rawRequest(
    "GET",
    `/api/v1/github/workspace-settings?workspace_id=${targetId}`,
  );
  const copied = await settings.json();
  expect(copied.default_query_presets).toEqual({ pr: [], issue: [] });
  expect(copied.saved_presets.map((preset: { label: string }) => preset.label)).toEqual([
    "Cloned PR default",
    "Cloned issue default",
  ]);
  await page.reload();
  await expect(page.getByTestId("workspace-settings-switcher")).toContainText(
    "My cloned workspace",
  );
  await page.goto(`/settings/workspaces/${targetId}/repositories`);
  await expect(page.getByText("Copied repository", { exact: true }).first()).toBeVisible();
  await page.goto(`/settings/workspaces/${targetId}/workflows`);
  await expect(page.locator('input[value="Kanban"]').first()).toBeVisible();
  await api.saveUserSettings({ workspace_id: targetId });
  await page.goto("/github");
  const title = page.getByTestId("github-list-toolbar-title");
  await expect(title).toContainText("Cloned PR default");
  if (mobile) {
    const github = new MobileGitHubPage(page);
    await github.mobileMenuButton.tap();
    await github.mobileSidebar.getByRole("button", { name: "Issues", exact: true }).tap();
    await expect(title).toContainText("Cloned issue default");
    await page.keyboard.press("Escape");
  } else {
    await page
      .getByTestId("github-presets-scope-bar")
      .getByRole("button", { name: "Issues", exact: true })
      .click();
    await expect(title).toContainText("Cloned issue default");
  }
  await page.reload();
  await expect(title).toContainText("Cloned PR default");
  if (mobile) {
    const github = new MobileGitHubPage(page);
    await github.mobileMenuButton.tap();
    await github.mobileSidebar.getByRole("button", { name: "Issues", exact: true }).tap();
  } else {
    await page
      .getByTestId("github-presets-scope-bar")
      .getByRole("button", { name: "Issues", exact: true })
      .click();
  }
  await expect(title).toContainText("Cloned issue default");
  const changed = await api.rawRequest("PUT", "/api/v1/github/workspace-settings", {
    workspace_id: targetId,
    saved_presets: [],
  });
  expect(changed.ok).toBe(true);
  const original = await api.rawRequest(
    "GET",
    `/api/v1/github/workspace-settings?workspace_id=${sourceId}`,
  );
  expect((await original.json()).saved_presets).toHaveLength(2);
}
