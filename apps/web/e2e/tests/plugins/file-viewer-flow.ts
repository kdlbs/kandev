import fs from "node:fs";
import path from "node:path";
import { expect, type Page, type Request } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { resetSeedRepositoryCheckout, type SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import { installFixturePlugin } from "../../helpers/plugin-fixture";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

export async function fileViewerFlow({
  testPage,
  apiClient,
  seedData,
  backend,
  mobile,
  viewer,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  mobile: boolean;
  viewer?: {
    archivePath: string;
    id: string;
    bytes: Uint8Array;
    verify: (page: Page, mobile: boolean) => Promise<void>;
  };
}) {
  if (viewer) {
    await testPage.goto("/settings/plugins");
    await testPage.getByTestId("install-plugin-trigger").click();
    await testPage.getByTestId("install-plugin-tab-upload").click();
    await testPage.getByTestId("install-plugin-file-input").setInputFiles(viewer.archivePath);
    await testPage.getByTestId("install-plugin-upload-submit").click();
    await expect(testPage.getByTestId("install-plugin-dialog")).toBeHidden({ timeout: 30000 });
    await expect(testPage.getByTestId(`plugin-row-${viewer.id}`)).toBeVisible();
  } else await installFixturePlugin(testPage);
  const repo = path.join(backend.tmpDir, "repos", "e2e-repo");
  const git = new GitHelper(repo, makeGitEnv(backend.tmpDir));
  const external = path.join(backend.tmpDir, "outside-model.stl");
  let readRequests = 0;
  const observeRead = (request: Request) => {
    if (/file-previews\/[^/]+\/read/.test(request.url())) readRequests++;
  };
  try {
    resetSeedRepositoryCheckout(seedData, backend.tmpDir);
    const model = viewer?.bytes ?? Buffer.alloc(11 * 1024 * 1024, 1);
    fs.writeFileSync(external, model);
    fs.writeFileSync(path.join(repo, "workspace-model.stl"), model);
    fs.symlinkSync(external, path.join(repo, "external-model.stl"));
    fs.writeFileSync(path.join(repo, "unsupported.pdf"), Buffer.from([0, 255, 0]));
    fs.writeFileSync(path.join(repo, "notes.txt"), "Readable workspace notes");
    git.stageAll();
    git.commit("add external model selection");
    const profile = await createStandardProfile(apiClient, "File viewer test");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "File preview approval",
      profile.id,
      {
        description: `e2e:mcp:kandev-rich-output:show_rich_output_kandev(${JSON.stringify({
          version: 2,
          catalog_version: 1,
          title: "Model preview",
          spec: {
            root: "model",
            elements: {
              model: { type: "FilePreview", props: { path: "workspace-model.stl" }, children: [] },
            },
          },
        })})`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(
        async () => {
          const env = await apiClient.getTaskEnvironment(task.id);
          const paths = [
            env?.workspace_path,
            env?.worktree_path,
            ...(env?.repos ?? []).map((r) => r.worktree_path),
          ];
          return paths.some((root) => root && fs.existsSync(path.join(root, "external-model.stl")));
        },
        { timeout: 30000 },
      )
      .toBe(true);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    const card = testPage.getByTestId("rich-output-file");
    await expect(card).toBeVisible();
    if (viewer) await viewer.verify(testPage, mobile);
    else await expect(card.getByTestId("fixture-file-preview")).toHaveText(String(model.length));
    await card.getByTestId("rich-output-file-preview-toggle").click();
    await expect(card.getByTestId("plugin-file-preview")).toHaveCount(0);
    if (mobile) await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    await (await session.fileTree.waitForFileTreeNode("workspace-model.stl")).click();
    const preview = testPage.getByTestId("plugin-file-preview");
    if (viewer) await expect(preview.locator(".stl-viewport canvas")).toBeVisible();
    else await expect(preview.getByTestId("fixture-file-preview")).toHaveText(String(model.length));
    await expect(preview.getByRole("button", { name: "Request preview", exact: true })).toHaveCount(
      0,
    );
    await expect(preview.getByRole("button", { name: "Cancel", exact: true })).toHaveCount(0);
    await expect(testPage.getByRole("dialog")).toHaveCount(0);
    if (mobile) await testPage.getByRole("button", { name: "Close", exact: true }).click();
    testPage.on("request", observeRead);
    await (await session.fileTree.waitForFileTreeNode("external-model.stl")).click();
    await expect(preview).toBeVisible();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog.getByText(external, { exact: true })).toBeVisible();
    await expect(testPage.getByTestId("fixture-file-preview")).toHaveCount(0);
    await dwell(
      testPage,
      250,
      "negative-assertion",
      "external preview must not read bytes before consent",
    );
    expect(readRequests).toBe(0);
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await preview.getByRole("button", { name: "Retry preview", exact: true }).click();
    await dialog.getByRole("button", { name: "Preview once", exact: true }).click();
    if (viewer) await viewer.verify(testPage, mobile);
    else
      await expect(testPage.getByTestId("fixture-file-preview")).toHaveText(
        String(11 * 1024 * 1024),
      );
    expect(readRequests).toBe(1);
    if (!viewer) {
      await assertExternalPolicies(testPage, apiClient, seedData.workspaceId);
      await assertDiscoveryReturn(testPage, session, mobile);
    }
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  } finally {
    testPage.off("request", observeRead);
    resetSeedRepositoryCheckout(seedData, backend.tmpDir);
    fs.rmSync(external, { force: true });
  }
}

async function assertExternalPolicies(page: Page, api: ApiClient, workspace: string) {
  const endpoint = `/api/plugins/kandev-plugin-e2e/file-preview-policy?workspace_id=${workspace}`;
  const change = async (mode: string) => {
    const policy = await (await api.rawRequest("GET", endpoint)).json();
    const response = await api.rawRequest("PUT", endpoint, {
      mode,
      expected_revision: policy.revision,
    });
    expect(response.ok).toBe(true);
  };
  const baseline = await (await api.rawRequest("GET", endpoint)).json();
  try {
    await change("deny");
    await expect(page.getByTestId("fixture-file-preview")).toHaveCount(0);
    await page.getByRole("button", { name: "Retry preview", exact: true }).click();
    await expect(
      page.getByText(
        "Preview access was denied. Check the external file setting and your file permissions.",
        { exact: true },
      ),
    ).toBeVisible();
    await change("allow");
    await page.getByRole("button", { name: "Retry preview", exact: true }).click();
    await expect(page.getByTestId("fixture-file-preview")).toHaveText(String(11 * 1024 * 1024));
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await change("ask");
    await expect(page.getByTestId("fixture-file-preview")).toHaveCount(0);
    await page.getByRole("button", { name: "Retry preview", exact: true }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.getByRole("dialog").getByRole("button", { name: "Cancel", exact: true }).click();
  } finally {
    await change(baseline.mode);
  }
}

async function assertDiscoveryReturn(page: Page, session: SessionPage, mobile: boolean) {
  if (mobile) await page.getByRole("button", { name: "Close", exact: true }).click();
  await (await session.fileTree.waitForFileTreeNode("notes.txt")).click();
  await expect(page.getByText("Readable workspace notes", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Find viewer plugin", exact: true })).toHaveCount(0);
  if (mobile) await page.getByRole("button", { name: "Close", exact: true }).click();
  await (await session.fileTree.waitForFileTreeNode("unsupported.pdf")).click();
  const preview = page.getByTestId("plugin-file-preview");
  await expect(preview.getByText("unsupported.pdf", { exact: true })).toBeVisible();
  const downloadButton = preview.getByRole("button", { name: "Download file", exact: true });
  await expect(downloadButton).toBeVisible();
  const [download] = await Promise.all([page.waitForEvent("download"), downloadButton.click()]);
  expect(download.suggestedFilename()).toBe("unsupported.pdf");
  expect(fs.readFileSync((await download.path())!)).toEqual(Buffer.from([0, 255, 0]));
  const origin = page.url();
  await page.evaluate(() => Reflect.set(window, "viewerNavigationMarker", true));
  await preview.getByRole("link", { name: "Find viewer plugin", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "File type: pdf (clear)", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => Reflect.get(window, "viewerNavigationMarker"))).toBe(true);
  if (mobile) await page.getByTestId("settings-close-button").click();
  else await page.goBack();
  await expect(page).toHaveURL(origin);
  await expect(preview.getByText("unsupported.pdf", { exact: true })).toBeVisible();
}
