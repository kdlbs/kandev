import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, type Locator, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { openMCPComposerCustomization } from "./mcp-composer";
import { waitForSessionDone } from "./session";

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.20, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.21
export async function checkMCPOriginSections({
  page,
  apiClient,
  seedData,
  mobile,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  mobile: boolean;
}) {
  const previous = await Promise.all([
    apiClient.getMCPSelections("profile", seedData.agentProfileId, seedData.workspaceId),
    apiClient.getMCPSelections("repository", seedData.repositoryId, seedData.workspaceId),
  ]);
  const root = mkdtempSync(join(tmpdir(), "kandev-mcp-origin-e2e-"));
  const repositoryPath = join(root, "Atlas-operations");
  execFileSync("git", ["clone", seedData.repositoryPath, repositoryPath]);
  const repository = await apiClient.createRepository(seedData.workspaceId, repositoryPath);
  const definitions = await createOriginDefinitions(apiClient, seedData.workspaceId);
  const [taskOnly, repositoryOnly, profileOnly, shared, multiRepo, extra, disabled] = definitions;
  let taskId: string | undefined;
  try {
    await apiClient.replaceMCPSelections("profile", seedData.agentProfileId, seedData.workspaceId, [
      profileOnly.id,
      shared.id,
    ]);
    await apiClient.replaceMCPSelections(
      "repository",
      seedData.repositoryId,
      seedData.workspaceId,
      [repositoryOnly.id, shared.id, multiRepo.id],
    );
    await apiClient.replaceMCPSelections("repository", repository.id, seedData.workspaceId, [
      multiRepo.id,
    ]);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "MCP origin sections",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId, repository.id],
      },
    );
    taskId = task.id;
    if (!task.session_id) throw new Error("Missing origin-section session");
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "Waiting for origin section session",
    );
    await apiClient.replaceMCPSelections("task", task.id, seedData.workspaceId, [
      taskOnly.id,
      shared.id,
    ]);
    definitions[6] = await apiClient.updateMCPServer(seedData.workspaceId, disabled.id, {
      expected_revision: disabled.revision,
      enabled: false,
    });
    definitions[2] = await apiClient.updateMCPServer(seedData.workspaceId, profileOnly.id, {
      expected_revision: profileOnly.revision,
      enabled: false,
    });
    await page.goto(`/t/${task.id}`);
    const explorer = await openMCPComposerCustomization(page);
    await expectOrigins(
      explorer,
      definitions.map((d) => d.id),
    );
    const inherited = explorer.getByTestId(`mcp-inherited-server-${multiRepo.id}`);
    await expect(inherited).toContainText((await apiClient.getRepository(repository.id)).name);
    await expect(inherited).toContainText(
      (await apiClient.getRepository(seedData.repositoryId)).name,
    );
    await expectRichChoice(explorer, extra.id, extra.display_name, mobile);
    await expect(
      explorer.getByTestId(`mcp-choice-${disabled.id}`).getByRole("checkbox"),
    ).toBeDisabled();
    const choice = explorer.getByTestId(`mcp-choice-${extra.id}`);
    if (mobile && (await page.evaluate(() => navigator.maxTouchPoints > 0))) await choice.tap();
    else await choice.getByRole("checkbox").click();
    await expect
      .poll(
        async () =>
          (await apiClient.getMCPSelections("task_session", task.session_id!, seedData.workspaceId))
            .definition_ids,
      )
      .toEqual([extra.id]);
    await page.reload();
    const reopened = await openMCPComposerCustomization(page);
    await expect(
      reopened.getByTestId(`mcp-choice-${extra.id}`).getByRole("checkbox"),
    ).toBeChecked();
    await expectRichChoice(reopened, extra.id, extra.display_name, mobile);
    await expectOriginSectionsDisappear(
      { page, apiClient, seedData, mobile },
      { taskId: task.id, repositoryId: repository.id, extra },
    );
  } finally {
    if (taskId) await apiClient.deleteTask(taskId, { discardWorktreeChanges: true });
    await apiClient.replaceMCPSelections(
      "profile",
      seedData.agentProfileId,
      seedData.workspaceId,
      previous[0].definition_ids,
    );
    await apiClient.replaceMCPSelections(
      "repository",
      seedData.repositoryId,
      seedData.workspaceId,
      previous[1].definition_ids,
    );
    await apiClient.deleteRepository(repository.id);
    for (const definition of definitions) {
      await apiClient.deleteMCPServer(seedData.workspaceId, definition.id, definition.revision);
    }
    rmSync(root, { recursive: true, force: true });
  }
}

async function createOriginDefinitions(api: ApiClient, workspaceId: string) {
  return Promise.all(
    [
      "Task notes",
      "Repository guides",
      "Profile search",
      "Shared docs",
      "Two-repository docs",
      "Optional issue tracker",
      "Disabled archives",
    ].map((display_name, i) =>
      api.createMCPServer(workspaceId, {
        runtime_name: `origin-section-${i}`,
        display_name,
        description: i === 5 ? "Find and update project issues." : undefined,
        execution_mode: "remote",
        transport: "sse",
        source: "custom",
        configuration: { url: `https://mcp.example.test/origins-${i}` },
      }),
    ),
  );
}

async function expectOrigins(explorer: Locator, ids: string[]) {
  for (const name of ["Selected for this task", "From repositories", "From agent profile"])
    await expect(explorer.getByRole("heading", { name, exact: true })).toHaveCount(1);
  await expect(explorer.getByRole("heading", { name: "From task setup", exact: true })).toHaveCount(
    0,
  );
  for (const [scope, members] of [
    ["task", [ids[0], ids[3]]],
    ["repository", [ids[1], ids[4]]],
    ["profile", [ids[2]]],
  ] as const) {
    const section = explorer.getByTestId(`mcp-origin-section-${scope}`);
    for (const id of members) {
      await expect(section.getByTestId(`mcp-inherited-server-${id}`)).toHaveCount(1);
      await expect(explorer.getByTestId(`mcp-inherited-server-${id}`)).toHaveCount(1);
      await expect(explorer.getByTestId(`mcp-choice-${id}`)).toHaveCount(0);
    }
    await expect(section.getByRole("checkbox")).toHaveCount(0);
  }
  const shared = explorer.getByTestId(`mcp-inherited-server-${ids[3]}`);
  await expect(shared).toContainText("Also selected by");
  await expect(shared).toContainText("Repository:");
  await expect(shared).toContainText("Profile:");
  await expect(explorer.getByTestId(`mcp-inherited-server-${ids[2]}`)).toContainText("Disabled");
  await expect(shared.getByTestId("mcp-origin-badge")).toHaveCount(0);
  await expect(
    explorer
      .getByTestId(`mcp-inherited-server-${ids[0]}`)
      .getByTestId("mcp-definition-description"),
  ).toHaveCount(0);
}

async function expectRichChoice(explorer: Locator, id: string, name: string, mobile: boolean) {
  const row = explorer.getByTestId(`mcp-choice-${id}`);
  await expect(row).toContainText("Find and update project issues.");
  await expect(row.getByTestId("mcp-definition-badges")).toContainText("Remote endpoint");
  await expect(row.getByTestId("mcp-definition-badges")).toContainText("Server-sent events");
  await expect(row.getByTestId("mcp-definition-badges")).toContainText("Custom");
  await expect(row.getByRole("checkbox", { name, exact: true })).toHaveCount(1);
  await row.scrollIntoViewIfNeeded();
  if (mobile) expect((await row.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  expect(await explorer.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);
}

async function expectOriginSectionsDisappear(
  {
    page,
    apiClient,
    seedData,
    mobile,
  }: { page: Page; apiClient: ApiClient; seedData: SeedData; mobile: boolean },
  {
    taskId,
    repositoryId,
    extra,
  }: { taskId: string; repositoryId: string; extra: { id: string; display_name: string } },
) {
  await apiClient.replaceMCPSelections("task", taskId, seedData.workspaceId, []);
  await apiClient.replaceMCPSelections(
    "profile",
    seedData.agentProfileId,
    seedData.workspaceId,
    [],
  );
  await page.reload();
  const repositoryView = await openMCPComposerCustomization(page);
  await expect(
    repositoryView.getByRole("heading", { name: "Selected for this task", exact: true }),
  ).toHaveCount(0);
  await expect(
    repositoryView.getByRole("heading", { name: "From agent profile", exact: true }),
  ).toHaveCount(0);
  await expect(
    repositoryView.getByRole("heading", { name: "From repositories", exact: true }),
  ).toHaveCount(1);
  for (const owner of [seedData.repositoryId, repositoryId])
    await apiClient.replaceMCPSelections("repository", owner, seedData.workspaceId, []);
  await page.reload();
  const emptyView = await openMCPComposerCustomization(page);
  await expect(emptyView.getByTestId("mcp-inherited-group")).toHaveCount(0);
  await expectRichChoice(emptyView, extra.id, extra.display_name, mobile);
}
