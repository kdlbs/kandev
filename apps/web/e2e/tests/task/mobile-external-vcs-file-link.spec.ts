import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../../fixtures/backend";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const MOBILE_FILE = "mobile-external-link.ts";
const MOBILE_REMOTE = "https://github.com/testorg/mobile-external-links.git";

function initializeMobileRepository(backend: BackendContext): string {
  const repositoryPath = path.join(backend.tmpDir, "repos", `mobile-link-${Date.now()}`);
  fs.mkdirSync(repositoryPath, { recursive: true });
  const gitEnvironment = makeGitEnv(backend.tmpDir);
  execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, env: gitEnvironment });
  fs.writeFileSync(path.join(repositoryPath, MOBILE_FILE), "export const mobile = true;\n");
  execFileSync("git", ["add", "-A"], { cwd: repositoryPath, env: gitEnvironment });
  execFileSync("git", ["commit", "-m", "seed mobile provider repository"], {
    cwd: repositoryPath,
    env: gitEnvironment,
  });
  return repositoryPath;
}

async function createMobileRepository(
  apiClient: ApiClient,
  workspaceId: string,
  repositoryPath: string,
) {
  const options = {
    name: "mobile-external-links",
    provider: "github",
    provider_host: "https://github.com",
    provider_owner: "testorg",
    provider_name: "mobile-external-links",
  };
  return apiClient.createRepository(workspaceId, repositoryPath, "main", options);
}

function trustedMobileTaskRepository() {
  return {
    remote_url: MOBILE_REMOTE,
    provider: "github",
    provider_owner: "testorg",
    provider_name: "mobile-external-links",
    base_branch: "main",
  };
}

test.describe("Mobile external VCS file link", () => {
  test.describe.configure({ timeout: 120_000 });

  test("opens the provider file with a touch-sized action and no horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repositoryPath = initializeMobileRepository(backend);
    await createMobileRepository(apiClient, seedData.workspaceId, repositoryPath);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile external file link",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [trustedMobileTaskRepository()],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await testPage.getByRole("button", { name: "Files" }).tap();
    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${MOBILE_FILE}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible();
    const link = viewer.getByRole("link", { name: "Open file in GitHub" });
    const expectedURL =
      "https://github.com/testorg/mobile-external-links/blob/main/mobile-external-link.ts";
    await expect(link).toHaveAttribute("href", expectedURL);

    const bounds = await link.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.width).toBeGreaterThanOrEqual(44);
    expect(bounds!.height).toBeGreaterThanOrEqual(44);
    const overflow = await testPage.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
    }));
    expect(overflow.documentWidth).toBeLessThanOrEqual(overflow.viewportWidth + 1);

    await testPage.context().route("https://github.com/**", async (route) => {
      await route.fulfill({ status: 200, contentType: "text/html", body: "external file" });
    });
    const popupPromise = testPage.waitForEvent("popup");
    await link.tap();
    const popup = await popupPromise;
    await popup.waitForLoadState("domcontentloaded");
    expect(popup.url()).toBe(expectedURL);
    await popup.close();
  });
});
