import { expect, test } from "../../fixtures/test-base";
import fs from "node:fs";
import path from "node:path";
import {
  createWorkspaceRootFilesFixture,
  deepRootDirectoryPath,
  deepRootFileContent,
  deepRootFilePath,
  DEEP_TREE_LEVEL_COUNT,
  expectRootFileUnstagedInRepository,
  expectWorkspaceRootFileContents,
  moveRootFileToSecondRepository,
  setChangesPanelLayout,
  WORKSPACE_ROOT_NESTED_FILE_CONTENT,
  WORKSPACE_ROOT_NESTED_FILE_PATH,
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
    includeDeepTree: true,
  });
  await setChangesPanelLayout(testPage, "tree");

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
  const groupToggle = rootFiles.getByTestId("changes-root-files-toggle");
  const folderToggle = rootFiles.getByTestId("tree-dir-reports-daily");
  const nestedRow = rootFiles.getByTestId(
    `workspace-root-file-row-${WORKSPACE_ROOT_NESTED_FILE_PATH.replaceAll("/", "-")}`,
  );
  await expect(groupToggle).toHaveAttribute("aria-expanded", "true");
  await expect(folderToggle).toHaveAttribute("aria-expanded", "true");
  await groupToggle.tap();
  await expect(groupToggle).toHaveAttribute("aria-expanded", "false");
  await expect(folderToggle).toBeHidden();
  await groupToggle.tap();
  await expect(folderToggle).toBeVisible();
  await folderToggle.tap();
  await expect(folderToggle).toHaveAttribute("aria-expanded", "false");
  await expect(nestedRow).toBeHidden();
  await folderToggle.tap();
  await expect(nestedRow).toBeVisible();

  const scrollOwner = panel.getByTestId("changes-panel-scroll-owner");
  const scrollOwnerBox = await scrollOwner.boundingBox();
  expect(scrollOwnerBox).not.toBeNull();
  const controls = [
    groupToggle,
    folderToggle,
    nestedRow.getByRole("button", { name: WORKSPACE_ROOT_NESTED_FILE_PATH }),
    nestedRow.getByRole("button", { name: "Show more actions" }),
  ];
  for (const control of controls) {
    await control.scrollIntoViewIfNeeded();
    const bounds = await control.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.height).toBeGreaterThanOrEqual(44);
    expect(bounds!.x).toBeGreaterThanOrEqual(scrollOwnerBox!.x - 2);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(
      scrollOwnerBox!.x + scrollOwnerBox!.width + 2,
    );
  }
  const rootFilesBox = await rootFiles.boundingBox();
  expect(rootFilesBox).not.toBeNull();
  expect(rootFilesBox!.x).toBeGreaterThanOrEqual(scrollOwnerBox!.x - 2);
  expect(rootFilesBox!.x + rootFilesBox!.width).toBeLessThanOrEqual(
    scrollOwnerBox!.x + scrollOwnerBox!.width + 2,
  );
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);

  for (let level = 1; level <= DEEP_TREE_LEVEL_COUNT; level++) {
    const directoryPath = deepRootDirectoryPath(level);
    const toggle = rootFiles.getByTestId(`tree-dir-${directoryPath.replaceAll("/", "-")}`);
    await toggle.scrollIntoViewIfNeeded();
    const bounds = await toggle.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.height).toBeGreaterThanOrEqual(44);
    expect(bounds!.x).toBeGreaterThanOrEqual(scrollOwnerBox!.x);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(
      scrollOwnerBox!.x + scrollOwnerBox!.width,
    );
    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
  }

  const deepFilePath = deepRootFilePath(DEEP_TREE_LEVEL_COUNT);
  const deepFileRow = rootFiles.getByTestId(
    `workspace-root-file-row-${deepFilePath.replaceAll("/", "-")}`,
  );
  const deepFileActions = deepFileRow.getByRole("button", { name: "Show more actions" });
  await deepFileActions.scrollIntoViewIfNeeded();
  const deepFileBounds = await deepFileActions.boundingBox();
  expect(deepFileBounds).not.toBeNull();
  expect(deepFileBounds!.height).toBeGreaterThanOrEqual(44);
  expect(deepFileBounds!.x).toBeGreaterThanOrEqual(scrollOwnerBox!.x);
  expect(deepFileBounds!.x + deepFileBounds!.width).toBeLessThanOrEqual(
    scrollOwnerBox!.x + scrollOwnerBox!.width,
  );
  await deepFileActions.tap();
  const openDeepFile = testPage.getByRole("menuitem", { name: "Open", exact: true });
  await expect(openDeepFile).toBeVisible();
  await openDeepFile.tap();
  const viewer = testPage.getByTestId("mobile-file-viewer-panel");
  await expect(viewer).toBeVisible();
  await expect(
    viewer.getByText(deepRootFileContent(DEEP_TREE_LEVEL_COUNT), { exact: true }),
  ).toBeVisible();

  await testPage
    .getByRole("navigation")
    .getByRole("button", { name: /Changes$/ })
    .tap();
  await nestedRow.getByRole("button", { name: WORKSPACE_ROOT_NESTED_FILE_PATH }).tap();
  await expect(viewer).toBeVisible();
  await expect(viewer.getByText(WORKSPACE_ROOT_NESTED_FILE_CONTENT, { exact: true })).toBeVisible();
  await testPage
    .getByRole("navigation")
    .getByRole("button", { name: /Changes$/ })
    .tap();
  await rootFiles.getByRole("button", { name: "dashboard.json" }).tap();

  await expect(viewer).toBeVisible();
  await expect(viewer.getByText(workspaceRootFileMarker, { exact: true })).toBeVisible();

  await testPage
    .getByRole("navigation")
    .getByRole("button", { name: /Changes$/ })
    .tap();
  const destinationName = await moveRootFileToSecondRepository(
    testPage,
    rootFiles,
    "tap",
    "dashboard.json",
    {
      watcher,
      sessionId,
      expectSuccess: true,
    },
  );
  await expect(rootFiles.getByTestId("workspace-root-file-row-dashboard.json")).toHaveCount(0);
  await expect(rootFiles).toBeVisible();
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
  await moveRootFileToSecondRepository(testPage, rootFiles, "tap", "dashboard.json", {
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
