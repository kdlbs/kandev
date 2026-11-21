import { expect, test } from "../../fixtures/test-base";
import fs from "node:fs";
import path from "node:path";
import {
  createWorkspaceRootFilesFixture,
  expectRootFileUnstagedInRepository,
  expectWorkspaceRootFileContents,
  moveRootFileToSecondRepository,
  workspaceRootFileMarker,
  writeWorkspaceRootFileAndWait,
} from "./workspace-root-files.helpers";
import { watchWs } from "../../helpers/causal-waits";

test("opens and moves workspace root files on a phone without staging or overwriting", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  const watcher = watchWs(testPage);
  const { sessionId, repoPaths, gitEnv, workspaceRoot } = await createWorkspaceRootFilesFixture({
    testPage,
    apiClient,
    seedData,
    backend,
    watcher,
  });

  const initialListing = watcher.waitForResponse("workspace.root_files.list", {
    requestWhere: (payload) => payload.session_id === sessionId,
  });
  await testPage
    .getByRole("navigation")
    .getByRole("button", { name: /Changes$/ })
    .tap();
  await initialListing;
  const panel = testPage.getByTestId("mobile-changes-panel");
  const rootFiles = panel.getByTestId("changes-workspace-root-files");
  await expect(rootFiles).toBeVisible();
  await expect(rootFiles.getByRole("heading", { name: "Outside repositories" })).toBeVisible();
  await rootFiles.getByRole("button", { name: "dashboard.json" }).tap();

  const viewer = testPage.getByTestId("mobile-file-viewer-panel");
  await expect(viewer).toBeVisible();
  await expect(viewer.locator(".cm-content, .monaco-editor .view-lines")).toContainText(
    workspaceRootFileMarker,
  );

  await testPage
    .getByRole("navigation")
    .getByRole("button", { name: /Changes$/ })
    .tap();
  const destinationName = await moveRootFileToSecondRepository(testPage, rootFiles, "tap", {
    watcher,
    sessionId,
    expectSuccess: true,
  });
  await expect(rootFiles).toHaveCount(0);
  const unstagedRow = panel.getByTestId("file-row-dashboard.json");
  await expect(unstagedRow).toBeVisible();
  await expect(panel.getByRole("button", { name: destinationName })).toBeVisible();
  await expect(panel.getByRole("button", { name: /Unstaged \(1\)/ })).toBeVisible();
  await expectRootFileUnstagedInRepository(repoPaths, gitEnv, 1, "dashboard.json");

  const conflictContent = "root-conflict-marker\n";
  await writeWorkspaceRootFileAndWait({
    watcher,
    sessionId,
    workspaceRoot,
    filePath: "dashboard.json",
    content: conflictContent,
  });
  await expect(rootFiles).toBeVisible();
  await moveRootFileToSecondRepository(testPage, rootFiles, "tap", {
    watcher,
    sessionId,
    expectSuccess: false,
  });
  const dialog = testPage.getByRole("dialog");
  await expect(
    testPage.getByTestId("toast-message").filter({ hasText: "Could not move file" }),
  ).toBeVisible();
  await expect(testPage.getByTestId("toast-message")).toContainText("target already exists");
  await expect(dialog).toBeVisible();
  expectWorkspaceRootFileContents(workspaceRoot, "dashboard.json", conflictContent);
  expect(fs.readFileSync(path.join(repoPaths[1], "dashboard.json"), "utf8")).toBe(
    `${workspaceRootFileMarker}\n`,
  );
  await expectRootFileUnstagedInRepository(repoPaths, gitEnv, 1, "dashboard.json");
});
