import fs from "node:fs";
import path from "node:path";
import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const MARKDOWN_CONTENT = `# Markdown lifecycle

This paragraph stays in the canonical source.

> The rendered editor should match Preview typography.

\`\`\`ts
const ready = true;
\`\`\`

| Area | State |
| --- | --- |
| Preview | Ready |

${Array.from(
  { length: 56 },
  (_, index) => `Long desktop paragraph ${index + 1} keeps the preview scrollable.`,
).join("\n\n")}

<div data-unsupported="true">Unsupported source</div>
`;
const UNSUPPORTED_MARKDOWN_SOURCE = '<div data-unsupported="true">Unsupported source</div>';
const LONG_EDIT_PARAGRAPH =
  "Kandev should read as a restrained developer workbench. The shell is dense and quiet, with panels, command surfaces, and state indicators arranged so users can keep their place across tasks, sessions, repositories, and integrations.";

async function seedMarkdownSession({
  testPage,
  apiClient,
  seedData,
  backend,
  fileName,
  content,
  taskTitle,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: { tmpDir: string };
  fileName: string;
  content: string;
  taskTitle: string;
}): Promise<{ session: SessionPage; filePath: string; sessionId: string }> {
  const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
  const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
  git.createFile(fileName, content);
  git.stageAll();
  git.commit(`seed ${fileName}`);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    taskTitle,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return {
    session,
    filePath: path.join(repoDir, fileName),
    sessionId: task.session_id,
  };
}

async function openFile(session: SessionPage, testPage: Page, fileName: string): Promise<void> {
  await session.clickTab("Files");
  await expect(session.files).toBeVisible({ timeout: 10_000 });
  const fileNode = session.fileTreeNode(fileName);
  await expect(fileNode).toBeVisible({ timeout: 15_000 });
  await fileNode.click();
  await expect(testPage.getByTestId("markdown-file-editor")).toBeVisible({ timeout: 15_000 });
}

async function appendToHybrid(testPage: Page, marker: string): Promise<void> {
  const editor = testPage.getByTestId("hybrid-markdown-editor");
  await expect(editor).toBeVisible({ timeout: 15_000 });
  const paragraph = editor.locator(".md-paragraph").last();
  await expect(paragraph).toBeVisible({ timeout: 15_000 });
  await paragraph.click();
  await testPage.keyboard.press(process.platform === "darwin" ? "Meta+End" : "Control+End");
  await testPage.keyboard.insertText(`\n\n${marker}`);
  await expect(editor).toContainText(marker);
}

async function editExistingHybridParagraph(testPage: Page): Promise<void> {
  const editor = testPage.getByTestId("hybrid-markdown-editor");
  const paragraph = editor.locator(".md-paragraph", {
    hasText: "This paragraph stays in the canonical source.",
  });
  await expect(paragraph).toHaveCount(1);
  const caretPoint = await paragraph.evaluate((element) => {
    const target = "canonical";
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
      const node = walker.currentNode;
      const offset = node.textContent?.indexOf(target) ?? -1;
      if (offset < 0) continue;
      const range = document.createRange();
      range.setStart(node, offset);
      range.setEnd(node, offset + target.length);
      const rect = range.getBoundingClientRect();
      return { x: rect.right - 1, y: rect.top + rect.height / 2 };
    }
    return null;
  });
  expect(caretPoint).not.toBeNull();
  await testPage.mouse.click(caretPoint!.x, caretPoint!.y);
  await testPage.keyboard.insertText("!");
  await expect(
    editor.locator(".md-paragraph", {
      hasText: "This paragraph stays in the canonical! source.",
    }),
  ).toHaveCount(1);
  await expect(
    editor.getByText("This paragraph stays in the canonical source.", { exact: true }),
  ).toHaveCount(0);
}

async function clickAfterText(testPage: Page, block: Locator, target: string) {
  const point = await block.evaluate((element, text) => {
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
      const node = walker.currentNode;
      const offset = node.textContent?.indexOf(text) ?? -1;
      if (offset < 0) continue;
      const range = document.createRange();
      range.setStart(node, offset);
      range.setEnd(node, offset + text.length);
      const rect = range.getBoundingClientRect();
      return { x: rect.right - 1, y: rect.top + rect.height / 2 };
    }
    return null;
  }, target);
  expect(point).not.toBeNull();
  await testPage.mouse.click(point!.x, point!.y);
}

async function hybridPalette(testPage: Page) {
  return testPage.locator(".md-editor.kandev-hybrid-markdown-editor").evaluate((editor) => {
    const activeBlock = editor.querySelector<HTMLElement>(".md-block-active");
    const codeBlock = editor.querySelector<HTMLElement>(".md-code-block");
    const styles = getComputedStyle(editor);
    return {
      editorColor: styles.color,
      editorBackground: styles.backgroundColor,
      activeBackground: activeBlock ? getComputedStyle(activeBlock).backgroundColor : "",
      codeColor: codeBlock ? getComputedStyle(codeBlock).color : "",
      codeBackground: codeBlock ? getComputedStyle(codeBlock).backgroundColor : "",
    };
  });
}

async function presentationMetrics(testPage: Page, mode: "preview" | "edit") {
  const root =
    mode === "preview"
      ? testPage.getByTestId("markdown-preview-scroll-container")
      : testPage.locator(".md-editor.kandev-hybrid-markdown-editor");
  return root.evaluate(
    (element, selectors) => {
      const query = (selector: string) => element.querySelector<HTMLElement>(selector);
      const elementStyle = (selector: string) => {
        const node = query(selector);
        if (!node) throw new Error(`Missing Markdown element: ${selector}`);
        return { node, computed: getComputedStyle(node) };
      };
      const heading = elementStyle(selectors.heading);
      const blockquote = elementStyle(selectors.blockquote);
      const code = elementStyle(selectors.code);
      return {
        heading: {
          x: heading.node.getBoundingClientRect().x,
          fontSize: heading.computed.fontSize,
          fontWeight: heading.computed.fontWeight,
          lineHeight: heading.computed.lineHeight,
          marginTop: heading.computed.marginTop,
          marginBottom: heading.computed.marginBottom,
        },
        blockquote: {
          borderLeftWidth: blockquote.computed.borderLeftWidth,
          paddingLeft: blockquote.computed.paddingLeft,
          marginTop: blockquote.computed.marginTop,
          marginBottom: blockquote.computed.marginBottom,
          fontStyle: blockquote.computed.fontStyle,
        },
        code: {
          fontSize: code.computed.fontSize,
        },
      };
    },
    {
      heading: mode === "preview" ? "h1" : "h1.md-heading",
      blockquote: mode === "preview" ? "blockquote" : "blockquote.md-blockquote",
      code:
        mode === "preview"
          ? ":is(.monaco-editor, .cm-editor, .shiki-code-block, pre)"
          : "pre.md-code-block code",
    },
  );
}

async function expectSingleCompactToolbar(testPage: Page) {
  const editor = testPage.getByTestId("markdown-file-editor");
  const toolbar = editor.locator(".markdown-file-toolbar:visible");
  const editorBox = await editor.boundingBox();
  const toolbarBox = await toolbar.boundingBox();
  const visibleSurface = editor.locator(
    ":scope > .min-h-0.flex-1 :is(.monaco-editor:visible, [data-testid='markdown-preview-scroll-container']:visible, [data-testid='hybrid-markdown-editor']:visible)",
  );
  const surfaceBox = await visibleSurface.first().boundingBox();
  const modeButton = editor.locator("[data-testid^='markdown-mode-']:visible").first();
  const saveButton = editor.getByTestId("markdown-file-save");
  const buttonBox = await modeButton.boundingBox();
  const saveButtonBox = (await saveButton.count()) > 0 ? await saveButton.boundingBox() : null;
  await expect(toolbar).toHaveCount(1);
  expect(editorBox).not.toBeNull();
  expect(toolbarBox).not.toBeNull();
  expect(surfaceBox).not.toBeNull();
  expect(buttonBox).not.toBeNull();
  expect(surfaceBox!.y - editorBox!.y).toBeLessThanOrEqual(40);
  expect(buttonBox!.height).toBeLessThanOrEqual(20);
  expect(buttonBox!.height).toBeLessThan(toolbarBox!.height);
  if (saveButtonBox) {
    expect(saveButtonBox.height).toBeLessThanOrEqual(24);
    expect(saveButtonBox.height).toBeLessThan(toolbarBox!.height);
  }
}

test.describe("Markdown file editing", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("opens in Preview, edits the rendered hybrid buffer, saves, and restores Edit mode", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const fileName = `markdown-lifecycle-${Date.now()}.md`;
    const marker = `saved markdown marker ${Date.now()}`;
    const { session, filePath, sessionId } = await seedMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
      content: MARKDOWN_CONTENT,
      taskTitle: "Markdown lifecycle",
    });

    await openFile(session, testPage, fileName);
    const editor = testPage.getByTestId("markdown-file-editor");
    const preview = testPage.getByTestId("markdown-preview");
    await expect(editor.getByTestId("markdown-mode-preview")).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await expect(preview.locator("h1")).toHaveText("Markdown lifecycle");
    await expectSingleCompactToolbar(testPage);
    const previewMetrics = await presentationMetrics(testPage, "preview");

    await editor.getByTestId("markdown-mode-edit").click();
    await expect(testPage.getByTestId("hybrid-markdown-editor")).toBeVisible({ timeout: 15_000 });
    await expectSingleCompactToolbar(testPage);
    const editMetrics = await presentationMetrics(testPage, "edit");
    expect(editMetrics.heading).toEqual(previewMetrics.heading);
    expect(editMetrics.blockquote).toEqual(previewMetrics.blockquote);
    expect(editMetrics.code).toEqual(previewMetrics.code);
    await editExistingHybridParagraph(testPage);
    await appendToHybrid(testPage, marker);

    const lightPalette = await hybridPalette(testPage);
    expect(lightPalette.editorColor).not.toBe(lightPalette.codeBackground);
    expect(lightPalette.activeBackground).not.toBe("");
    await testPage.evaluate(() => document.documentElement.classList.add("dark"));
    await expect.poll(() => hybridPalette(testPage), { timeout: 5_000 }).not.toEqual(lightPalette);
    const darkPalette = await hybridPalette(testPage);
    expect(darkPalette.editorColor).not.toBe(darkPalette.codeBackground);
    expect(darkPalette.editorBackground).not.toBe(lightPalette.editorBackground);
    expect(darkPalette.codeBackground).not.toBe(lightPalette.codeBackground);

    const saveButton = editor.getByTestId("markdown-file-save");
    await expect(saveButton).toContainText(/Save\s*\((?:Ctrl|⌘)\+S\)/);
    await expect(saveButton).toBeEnabled();
    await saveButton.click();
    await expect
      .poll(() => fs.readFileSync(filePath, "utf8"), { timeout: 15_000 })
      .toContain(marker);
    expect(fs.readFileSync(filePath, "utf8")).toBe(
      `${MARKDOWN_CONTENT.replace(
        "This paragraph stays in the canonical source.",
        "This paragraph stays in the canonical! source.",
      )}\n\n${marker}`,
    );
    expect(fs.readFileSync(filePath, "utf8")).toContain(UNSUPPORTED_MARKDOWN_SOURCE);
    await expect(saveButton).toBeDisabled();

    await editor.getByTestId("markdown-mode-preview").click();
    await expect(preview).toContainText(marker);
    const previewScroll = preview.getByTestId("markdown-preview-scroll-container");
    const previewScrollTop = await previewScroll.evaluate((element) => {
      const scroller = element as HTMLElement;
      scroller.scrollTop = scroller.scrollHeight;
      return scroller.scrollTop;
    });
    expect(previewScrollTop).toBeGreaterThan(0);
    await prCapture.screenshot("desktop-markdown-preview", {
      caption: "Desktop Markdown Preview with the saved source change",
    });
    await editor.getByTestId("markdown-mode-edit").click();
    await editor.getByTestId("markdown-mode-preview").click();
    await expect
      .poll(() => previewScroll.evaluate((element) => (element as HTMLElement).scrollTop))
      .toBe(previewScrollTop);
    await editor.getByTestId("markdown-mode-edit").click();
    await expect(testPage.getByTestId("hybrid-markdown-editor")).toBeVisible({ timeout: 15_000 });

    const storedTabs = await testPage.evaluate((sid) => {
      const raw = window.sessionStorage.getItem(`kandev.openFiles.${sid}`);
      return raw ? (JSON.parse(raw) as Array<{ path: string; markdownMode?: string }>) : [];
    }, sessionId);
    const storedTab = storedTabs.find((tab) => tab.path.endsWith(fileName));
    expect(storedTab?.markdownMode).toBe("edit");

    await testPage.reload();
    await expect(testPage.getByTestId("markdown-file-editor")).toBeVisible({ timeout: 30_000 });
    await expect(testPage.getByTestId("markdown-mode-edit")).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await expect(testPage.getByTestId("hybrid-markdown-editor")).toBeVisible({ timeout: 15_000 });
  });

  test("keeps normal block editing single, exposes heading markers, and creates blocks with Enter", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fileName = `markdown-block-input-${Date.now()}.md`;
    const { session } = await seedMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
      content: `# Direction\n\n${LONG_EDIT_PARAGRAPH}\n\n#### Existing subheader\n`,
      taskTitle: "Markdown block input",
    });

    await openFile(session, testPage, fileName);
    const editor = testPage.getByTestId("markdown-file-editor");
    await editor.getByTestId("markdown-mode-source").click();
    const sourceInput = testPage.getByRole("textbox", { name: "Editor content" });
    await expect(sourceInput).toBeAttached({ timeout: 15_000 });
    await sourceInput.focus();
    await testPage.keyboard.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
    await testPage.keyboard.insertText(
      `# Direction\n\n${LONG_EDIT_PARAGRAPH.replace("quiet", "quiet!abcde")}\n\n#### Existing subheader\n`,
    );
    await editor.getByTestId("markdown-mode-edit").click();
    const hybrid = testPage.getByTestId("hybrid-markdown-editor");
    const paragraph = hybrid.locator(".md-paragraph", {
      hasText: "The shell is dense and quiet!abcde",
    });
    await clickAfterText(testPage, paragraph, "quiet!abcde");
    await testPage.keyboard.type("z");

    const repeatedPrefixCount = await hybrid.evaluate((element) => {
      const prefix = "Kandev should read as a restrained developer workbench.";
      return element.textContent?.split(prefix).length - 1;
    });
    expect(repeatedPrefixCount).toBe(1);

    const subheader = hybrid.locator("h4.md-heading", { hasText: "Existing subheader" });
    await subheader.click();
    const marker = subheader.locator(".md-marker-headingMarker");
    const [rootBox, markerBox] = await Promise.all([hybrid.boundingBox(), marker.boundingBox()]);
    expect(rootBox).not.toBeNull();
    expect(markerBox).not.toBeNull();
    expect(markerBox!.x).toBeGreaterThanOrEqual(rootBox!.x);

    const editedParagraph = hybrid.locator(".md-paragraph", {
      hasText: "The shell is dense and quiet!abcdez",
    });
    await clickAfterText(testPage, editedParagraph, "quiet!abcdez");
    await testPage.keyboard.press("Enter");
    await testPage.keyboard.type("A new block created with Enter. ");

    await expect(
      hybrid.locator(".md-paragraph", { hasText: "A new block created with Enter." }),
    ).toHaveCount(1);
  });

  test("keeps MDX on the safe Preview and exact Source paths", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fileName = `markdown-components-${Date.now()}.mdx`;
    const { session } = await seedMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
      content: "# MDX content\n\n<Component />",
      taskTitle: "Markdown MDX modes",
    });

    await openFile(session, testPage, fileName);
    const editor = testPage.getByTestId("markdown-file-editor");
    await expect(editor.getByTestId("markdown-mode-preview")).toBeVisible();
    await expect(editor.getByTestId("markdown-mode-source")).toBeVisible();
    await expect(editor.getByTestId("markdown-mode-edit")).toHaveCount(0);
    await editor.getByTestId("markdown-mode-source").click();
    await expect(testPage.locator(".monaco-editor:visible").first()).toBeVisible({
      timeout: 15_000,
    });
    await expectSingleCompactToolbar(testPage);
    await expect(testPage.getByTestId("hybrid-markdown-editor")).toHaveCount(0);
  });
});
