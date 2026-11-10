import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { waitForWorkspacePath } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

export const HTML_PREVIEW_ROOT = "html-diff-preview";
export const SOURCE_COPY = "Current workspace file";
export const TRUST_COPY = "Previewing runs workspace code";

export function nativeHtmlDocument(heading: string): string {
  return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <link rel="stylesheet" href="./assets/preview.css">
  </head>
  <body>
    <h1>${heading}</h1>
    <p id="css-status">CSS asset pending</p>
    <p id="script-status">Browser script pending</p>
    <script src="./assets/preview.js"></script>
  </body>
</html>`;
}

const PREVIEW_SCRIPT = `(() => {
  const status = document.querySelector("#script-status");
  if (status) status.textContent = "script:ready";
})();`;

const PREVIEW_CSS = "#css-status { font-weight: 700; }";

export type HtmlDiffFixture = {
  session: SessionPage;
  git: GitHelper;
  filePath: string;
};

/**
 * Seeds an uncommitted HTML file with relative CSS and script assets in a
 * fresh task workspace. The diff row and the current workspace file then have
 * the same known contents until a test deliberately diverges them.
 */
export async function createHtmlDiffTask({
  testPage,
  apiClient,
  seedData,
  backend,
  title,
  fileName,
  entryContent,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  title: string;
  fileName: string;
  entryContent: string;
}): Promise<HtmlDiffFixture> {
  const filePath = `${HTML_PREVIEW_ROOT}/${fileName}`;
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("HTML diff preview task has no session");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });

  const workspacePath = await waitForWorkspacePath(apiClient, task.id, task.session_id);
  const git = new GitHelper(workspacePath, makeGitEnv(backend.tmpDir));
  git.createFile(`${HTML_PREVIEW_ROOT}/assets/preview.css`, PREVIEW_CSS);
  git.createFile(`${HTML_PREVIEW_ROOT}/assets/preview.js`, PREVIEW_SCRIPT);
  git.createFile(filePath, entryContent);
  return { session, git, filePath };
}
