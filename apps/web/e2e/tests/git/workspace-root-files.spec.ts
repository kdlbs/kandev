import { expect, test } from "../../fixtures/test-base";
import fs from "node:fs";
import path from "node:path";
import {
  createWorkspaceRootFilesFixture,
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
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";

test("opens and moves workspace root files without staging or overwriting", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  const watcher = watchWs(testPage);
  const { session, sessionId, repoPaths, gitEnv, workspaceRoot } =
    await createWorkspaceRootFilesFixture({
      testPage,
      apiClient,
      seedData,
      backend,
      watcher,
    });
  await setChangesPanelLayout(testPage, "tree");

  const initialListing = watcher.waitForResponse("workspace.root_files.list", {
    requestWhere: (payload) => payload.session_id === sessionId,
  });
  await session.clickTab("Changes");
  await initialListing;
  const rootFiles = session.changes.getByTestId("changes-workspace-root-files");
  await expect(rootFiles).toBeVisible();
  await expect(rootFiles.getByTestId("changes-root-files-toggle")).toBeVisible();
  const nestedRow = rootFiles.getByTestId(
    `workspace-root-file-row-${WORKSPACE_ROOT_NESTED_FILE_PATH.replaceAll("/", "-")}`,
  );
  await expect(rootFiles.getByTestId("tree-dir-reports-daily")).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  await expect(nestedRow).toBeVisible();
  await nestedRow.getByRole("button", { name: WORKSPACE_ROOT_NESTED_FILE_PATH }).click();

  await expect(
    testPage.getByText(WORKSPACE_ROOT_NESTED_FILE_CONTENT, { exact: true }),
  ).toBeVisible();

  await rootFiles.getByRole("button", { name: "dashboard.json" }).click();

  const editor = testPage.getByTestId("file-editor-content");
  const editorContent = editor.locator(".cm-content:visible, .monaco-editor:visible .view-lines");
  await expect(editorContent).toHaveCount(1);
  await expect(editorContent).toContainText(workspaceRootFileMarker);

  const destinationName = await moveRootFileToSecondRepository(
    testPage,
    rootFiles,
    "click",
    WORKSPACE_ROOT_NESTED_FILE_PATH,
    {
      watcher,
      sessionId,
      expectSuccess: true,
    },
  );
  await expect(
    rootFiles.getByTestId(
      `workspace-root-file-row-${WORKSPACE_ROOT_NESTED_FILE_PATH.replaceAll("/", "-")}`,
    ),
  ).toHaveCount(0);
  await expect(rootFiles).toBeVisible();
  const unstagedRow = session.changes.getByTestId("file-row-reports-daily-summary.md");
  await expect(unstagedRow).toBeVisible();
  await expect(session.changes.getByRole("button", { name: destinationName })).toBeVisible();
  await expect(session.changes.getByRole("button", { name: /Unstaged \(1\)/ })).toBeVisible();
  await expectRootFileUnstagedInRepository(repoPaths, gitEnv, 1, WORKSPACE_ROOT_NESTED_FILE_PATH);
  expect(fs.readFileSync(path.join(repoPaths[1], WORKSPACE_ROOT_NESTED_FILE_PATH), "utf8")).toBe(
    `${WORKSPACE_ROOT_NESTED_FILE_CONTENT}\n`,
  );

  const conflictContent = "nested-root-conflict-marker\n";
  await writeWorkspaceRootFileAndWait({
    watcher,
    sessionId,
    workspaceRoot,
    filePath: WORKSPACE_ROOT_NESTED_FILE_PATH,
    content: conflictContent,
  });
  await expect(rootFiles).toBeVisible();
  await moveRootFileToSecondRepository(
    testPage,
    rootFiles,
    "click",
    WORKSPACE_ROOT_NESTED_FILE_PATH,
    {
      watcher,
      sessionId,
      expectSuccess: false,
    },
  );
  const dialog = testPage.getByRole("dialog");
  await expect(
    testPage.getByTestId("toast-message").filter({ hasText: "Could not move file" }),
  ).toBeVisible();
  await expect(testPage.getByTestId("toast-message")).toContainText("target already exists");
  await expect(dialog).toBeVisible();
  expectWorkspaceRootFileContents(workspaceRoot, WORKSPACE_ROOT_NESTED_FILE_PATH, conflictContent);
  expect(fs.readFileSync(path.join(repoPaths[1], WORKSPACE_ROOT_NESTED_FILE_PATH), "utf8")).toBe(
    `${WORKSPACE_ROOT_NESTED_FILE_CONTENT}\n`,
  );
  await expectRootFileUnstagedInRepository(repoPaths, gitEnv, 1, WORKSPACE_ROOT_NESTED_FILE_PATH);
});

test("keeps deep outside-file actions inside the desktop Changes panel", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  const watcher = watchWs(testPage);
  const { session, sessionId } = await createWorkspaceRootFilesFixture({
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
  await session.clickTab("Changes");
  await initialListing;

  const rootFiles = session.changes.getByTestId("changes-workspace-root-files");
  const deepestFilePath = deepRootFilePath(DEEP_TREE_LEVEL_COUNT);
  const deepestFile = rootFiles.getByTestId(
    `workspace-root-file-row-${deepestFilePath.replaceAll("/", "-")}`,
  );
  const openButton = deepestFile.getByRole("button", { name: deepestFilePath });
  const fileName = deepestFile.getByTestId("changes-file-name");
  const overflowButton = deepestFile.getByRole("button", { name: "Show more actions" });
  const scrollOwner = testPage.getByTestId("changes-panel-scroll-owner");
  await overflowButton.scrollIntoViewIfNeeded();

  const [scrollOwnerBox, openButtonBox, fileNameBox, overflowButtonBox] = await Promise.all([
    scrollOwner.boundingBox(),
    openButton.boundingBox(),
    fileName.boundingBox(),
    overflowButton.boundingBox(),
  ]);
  expect(scrollOwnerBox).not.toBeNull();
  expect(openButtonBox).not.toBeNull();
  expect(fileNameBox).not.toBeNull();
  expect(overflowButtonBox).not.toBeNull();
  expect(fileNameBox!.width).toBeGreaterThan(0);
  expect(fileNameBox!.x).toBeGreaterThanOrEqual(scrollOwnerBox!.x - 2);
  expect(fileNameBox!.x + fileNameBox!.width).toBeLessThanOrEqual(
    scrollOwnerBox!.x + scrollOwnerBox!.width + 2,
  );
  expect(openButtonBox!.x).toBeGreaterThanOrEqual(scrollOwnerBox!.x - 2);
  expect(openButtonBox!.x + openButtonBox!.width).toBeLessThanOrEqual(
    scrollOwnerBox!.x + scrollOwnerBox!.width + 2,
  );
  expect(overflowButtonBox!.x).toBeGreaterThanOrEqual(scrollOwnerBox!.x - 2);
  expect(overflowButtonBox!.x + overflowButtonBox!.width).toBeLessThanOrEqual(
    scrollOwnerBox!.x + scrollOwnerBox!.width + 2,
  );

  await overflowButton.click();
  await expect(testPage.getByRole("menuitem", { name: "Open", exact: true })).toBeVisible();
});

test("matches repository headers and file names in both Changes layouts", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  const watcher = watchWs(testPage);
  const { session, repoPaths } = await createWorkspaceRootFilesFixture({
    testPage,
    apiClient,
    seedData,
    backend,
    watcher,
  });
  const gitEnv = makeGitEnv(backend.tmpDir);
  for (const [index, repoPath] of repoPaths.entries()) {
    new GitHelper(repoPath, gitEnv).createFile(`src/reference-${index}.ts`, `reference ${index}\n`);
  }
  await setChangesPanelLayout(testPage, "tree");
  await session.clickTab("Changes");
  const panel = session.changes;
  const rootFiles = panel.getByTestId("changes-workspace-root-files");
  await expect(rootFiles).toBeVisible();
  await expect(panel.getByTestId("changes-repo-group")).toHaveCount(2, { timeout: 30_000 });

  const groupToggle = rootFiles.getByTestId("changes-root-files-toggle");
  const repositoryToggle = panel.getByTestId("changes-repo-header").first();
  const [outsideHeaderBox, repositoryHeaderBox] = await Promise.all([
    groupToggle.boundingBox(),
    repositoryToggle.boundingBox(),
  ]);
  expect(outsideHeaderBox).not.toBeNull();
  expect(repositoryHeaderBox).not.toBeNull();
  expect(Math.abs(outsideHeaderBox!.x - repositoryHeaderBox!.x)).toBeLessThanOrEqual(4);
  const outsideHeaderStyle = await groupToggle.evaluate((element) => {
    const style = getComputedStyle(element);
    return [style.fontFamily, style.fontSize, style.fontWeight, style.letterSpacing];
  });
  const repositoryHeaderStyle = await repositoryToggle.evaluate((element) => {
    const style = getComputedStyle(element);
    return [style.fontFamily, style.fontSize, style.fontWeight, style.letterSpacing];
  });
  expect(outsideHeaderStyle).toEqual(repositoryHeaderStyle);

  const outsideName = rootFiles
    .getByTestId(`workspace-root-file-row-${WORKSPACE_ROOT_NESTED_FILE_PATH.replaceAll("/", "-")}`)
    .getByTestId("changes-file-name");
  const repositoryName = panel
    .getByTestId("file-row-src-reference-0.ts")
    .getByTestId("changes-file-name");
  const outsideNameStyle = await outsideName.evaluate((element) => {
    const style = getComputedStyle(element);
    return [style.fontFamily, style.fontSize, style.fontWeight];
  });
  const repositoryNameStyle = await repositoryName.evaluate((element) => {
    const style = getComputedStyle(element);
    return [style.fontFamily, style.fontSize, style.fontWeight];
  });
  expect(outsideNameStyle).toEqual(repositoryNameStyle);

  await groupToggle.click();
  await expect(groupToggle).toHaveAttribute("aria-expanded", "false");
  await expect(groupToggle).toContainText("2");
  await testPage.keyboard.press("Enter");
  await expect(groupToggle).toHaveAttribute("aria-expanded", "true");

  await setChangesPanelLayout(testPage, "flat");
  await expect(rootFiles.getByTestId("tree-dir-reports-daily")).toHaveCount(0);
  await expect(
    rootFiles.getByRole("button", { name: WORKSPACE_ROOT_NESTED_FILE_PATH }),
  ).toBeVisible();
  await expect(
    rootFiles
      .getByTestId(
        `workspace-root-file-row-${WORKSPACE_ROOT_NESTED_FILE_PATH.replaceAll("/", "-")}`,
      )
      .getByTestId("changes-file-name"),
  ).toContainText("summary.md");
});

test("keeps outside repository controls at phone size with a fine pointer", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 390, height: 844 });
  const watcher = watchWs(testPage);
  await createWorkspaceRootFilesFixture({
    testPage,
    apiClient,
    seedData,
    backend,
    watcher,
  });
  await setChangesPanelLayout(testPage, "tree");
  await testPage
    .getByRole("navigation")
    .getByRole("button", { name: /Changes$/ })
    .click();
  const rootFiles = testPage
    .getByTestId("mobile-changes-panel")
    .getByTestId("changes-workspace-root-files");
  await expect(rootFiles).toBeVisible();
  const controls = [
    rootFiles.getByTestId("changes-root-files-toggle"),
    rootFiles.getByTestId("tree-dir-reports-daily"),
    rootFiles
      .getByTestId(
        `workspace-root-file-row-${WORKSPACE_ROOT_NESTED_FILE_PATH.replaceAll("/", "-")}`,
      )
      .getByRole("button", { name: WORKSPACE_ROOT_NESTED_FILE_PATH }),
    rootFiles
      .getByTestId(
        `workspace-root-file-row-${WORKSPACE_ROOT_NESTED_FILE_PATH.replaceAll("/", "-")}`,
      )
      .getByRole("button", { name: "Show more actions" }),
  ];
  for (const control of controls) {
    const bounds = await control.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.height).toBeGreaterThanOrEqual(44);
  }
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await expect(
    testPage.getByTestId("mobile-changes-panel").getByTestId("changes-panel-scroll-owner"),
  ).toBeVisible();
});
