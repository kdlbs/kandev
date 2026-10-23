import {
  expectNeutralDisabledButton,
  expectInheritedButtonForeground,
  setButtonTheme,
  holdFileSaveResponse,
} from "../../helpers/button-state-colors";
import fs from "node:fs";
import {
  seedMarkdownWalkthrough,
  openMarkdownWalkthrough,
  expectMarkdownWalkthroughRange,
} from "../../helpers/markdown-walkthrough";
import path from "node:path";
import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { dwell, watchWs, type WsWatcher } from "../../helpers/causal-waits";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import { markdownDocumentEnd, codeMirrorDocumentEnd } from "../../helpers/markdown-keyboard";
import { selectMobileMarkdownMode as selectMobileMode } from "../../helpers/markdown-preview";

const MOBILE_MARKDOWN_CONTENT = `# Mobile Markdown

Edit this file from the phone Files surface.

#### Mobile source markers

| Area | State | Notes |
| --- | --- | --- |
| Preview | Ready | The table remains contained |

${Array.from(
  { length: 56 },
  (_, index) => `Long mobile paragraph ${index + 1} keeps the editor content below the fold.`,
).join("\n\n")}

<div data-unsupported="true">Unsupported mobile source</div>
`;
const UNSUPPORTED_MOBILE_MARKDOWN_SOURCE =
  '<div data-unsupported="true">Unsupported mobile source</div>';
let restoreMobileRepository: (() => void) | undefined;

async function seedMobileMarkdownSession({
  testPage,
  apiClient,
  seedData,
  backend,
  fileName,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: { tmpDir: string };
  fileName: string;
}): Promise<{ session: SessionPage; filePath: string; gateway: WsWatcher; taskId: string }> {
  const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
  const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
  const originalSha = git.getCurrentSha();
  restoreMobileRepository = () => git.exec(`git reset --hard ${originalSha}`);
  git.createFile(fileName, MOBILE_MARKDOWN_CONTENT);
  git.stageAll();
  git.commit(`seed ${fileName}`);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Markdown editing",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const gateway = watchWs(testPage);
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return { session, filePath: path.join(repoDir, fileName), gateway, taskId: task.id };
}

async function appendToMobileSource(page: Page, viewer: Locator, marker: string) {
  const source = viewer.locator(".cm-content");
  const read = () =>
    source.evaluate((element) => {
      const state = (
        element as HTMLElement & {
          cmTile: {
            root: {
              view: {
                state: {
                  doc: { toString(): string };
                  selection: { main: { anchor: number; head: number } };
                };
              };
            };
          };
        }
      ).cmTile.root.view.state;
      return {
        text: state.doc.toString(),
        anchor: state.selection.main.anchor,
        head: state.selection.main.head,
      };
    });
  await source.click();
  await expect.poll(async () => (await read()).text).toBe(MOBILE_MARKDOWN_CONTENT);
  await page.keyboard.press(await codeMirrorDocumentEnd(page));
  await expect.poll(read).toMatchObject({
    anchor: MOBILE_MARKDOWN_CONTENT.length,
    head: MOBILE_MARKDOWN_CONTENT.length,
  });
  await page.keyboard.press("Enter");
  await expect.poll(read).toEqual({
    text: `${MOBILE_MARKDOWN_CONTENT}\n`,
    anchor: MOBILE_MARKDOWN_CONTENT.length + 1,
    head: MOBILE_MARKDOWN_CONTENT.length + 1,
  });
  await page.keyboard.type(marker);
  await expect.poll(async () => (await read()).text).toBe(`${MOBILE_MARKDOWN_CONTENT}\n${marker}`);
}

async function appendToHybrid(testPage: Page, viewer: Locator, marker: string): Promise<void> {
  const editor = viewer.getByTestId("hybrid-markdown-editor");
  await expect(editor).toBeVisible({ timeout: 15_000 });
  const paragraph = editor.locator(".md-paragraph").last();
  await expect(paragraph).toBeVisible({ timeout: 15_000 });
  await paragraph.tap();
  await testPage.keyboard.press(await markdownDocumentEnd(testPage));
  await testPage.keyboard.insertText(`\n\n${marker}`);
  await expect(editor).toContainText(marker);
}

async function dragTouch(page: Page, target: Locator, deltaX: number): Promise<void> {
  const box = await target.boundingBox();
  expect(box).not.toBeNull();
  const x = box!.x + box!.width / 2;
  const y = box!.y + box!.height / 2;
  if (page.context().browser()?.browserType().name() !== "chromium") {
    // WebKit exposes no CDP touch drag. Test its touch-pointer handlers; real iOS gestures remain manual.
    await target.dispatchEvent("pointerdown", {
      pointerType: "touch",
      pointerId: 1,
      clientX: x,
      clientY: y,
      button: 0,
      buttons: 1,
    });
    await page.evaluate(
      ({ x, y }) => {
        window.dispatchEvent(
          new PointerEvent("pointermove", {
            pointerType: "touch",
            pointerId: 1,
            clientX: x,
            clientY: y,
            buttons: 1,
          }),
        );
        window.dispatchEvent(
          new PointerEvent("pointerup", {
            pointerType: "touch",
            pointerId: 1,
            clientX: x,
            clientY: y,
            buttons: 0,
          }),
        );
      },
      { x: x + deltaX, y },
    );
    return;
  }
  const client = await page.context().newCDPSession(page);
  try {
    await client.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ id: 1, x, y }],
    });
    await dwell(
      page,
      50,
      "browser-chrome",
      "allow Chromium to dispatch the touch pointer sequence",
    );
    await client.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ id: 1, x: x + deltaX, y }],
    });
    await dwell(
      page,
      50,
      "browser-chrome",
      "allow Chromium to dispatch the touch pointer sequence",
    );
    await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await dwell(page, 50, "browser-chrome", "allow Chromium to finish the touch pointer sequence");
  } finally {
    await client.detach();
  }
}

test.describe("Mobile Markdown file editing", () => {
  test.describe.configure({ retries: 0, timeout: 120_000 });
  test.afterEach(() => {
    restoreMobileRepository?.();
    restoreMobileRepository = undefined;
  });

  // @covers AC-UI-MARKDOWN-FILE-EDITING-002.16, AC-UI-MARKDOWN-FILE-EDITING-002.19
  test("button colors preserve phone Save states in both themes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    const hold = await holdFileSaveResponse(testPage);
    const fileName = `mobile-button-colors-${Date.now()}.md`;
    const { session, filePath } = await seedMobileMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
    });
    await testPage
      .getByTestId("session-mobile-bottom-nav")
      .getByRole("button", { name: "Files", exact: true })
      .tap();
    await (await session.fileTree.waitForFileTreeNode(fileName)).tap();
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible();
    await selectMobileMode(testPage, viewer, "edit");
    const save = viewer.getByTestId("mobile-file-save");
    let expected = MOBILE_MARKDOWN_CONTENT;
    for (const dark of [false, true]) {
      await setButtonTheme(testPage, dark);
      await expectNeutralDisabledButton(save);
      const marker = `Phone ${dark ? "dark" : "light"} colors`;
      await appendToHybrid(testPage, viewer, marker);
      expected += `\n\n${marker}`;
      await expect(save).toBeEnabled();
      await expectInheritedButtonForeground(save);
      await save.tap();
      await expect.poll(hold.isHeld).toBe(true);
      try {
        await expectNeutralDisabledButton(save);
        await expect(save).toContainText("Saving");
      } finally {
        hold.release();
      }
      await expect(save).toContainText("Save");
      await expectNeutralDisabledButton(save);
      expect(fs.readFileSync(filePath, "utf8")).toBe(expected);
      const box = await save.boundingBox();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
      await save.screenshot({
        path: testInfo.outputPath(`phone-save-${dark ? "dark" : "light"}.png`),
      });
    }
    for (const width of [767, 768]) {
      await testPage.setViewportSize({ width, height: 900 });
      if (width === 768) {
        const tablet = testPage.getByTestId("tablet-task-layout");
        await expect(tablet).toBeVisible();
        await tablet
          .locator(`[data-testid="file-tree-node"][data-path=${JSON.stringify(fileName)}]:visible`)
          .tap();
      }
      const activeSave = testPage.locator(
        '[data-testid="mobile-file-save"]:visible, [data-testid="markdown-file-save"]:visible',
      );
      await expect(activeSave).toHaveCount(1);
      await expectNeutralDisabledButton(activeSave);
      const bounds = await activeSave.boundingBox();
      expect(bounds!.x).toBeGreaterThanOrEqual(0);
      expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width);
    }
  });

  test("walkthrough outlines remain in mobile rendered Markdown with the bottom sheet", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const fileName = `phone-walkthrough-${Date.now()}.md`;
    const { taskId, filePath } = await seedMobileMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
    });
    const ranges = await seedMarkdownWalkthrough(
      backend.tmpDir,
      taskId,
      fileName,
      MOBILE_MARKDOWN_CONTENT,
    );
    const card = await openMarkdownWalkthrough(testPage);
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(card).toHaveAttribute("data-mobile-variant", "bottom-sheet");
    await expect(viewer.getByTestId("mobile-markdown-mode-trigger")).toContainText("Preview");
    await expectMarkdownWalkthroughRange(viewer, ranges[0], viewer.locator("table"));
    await card.getByTestId("walkthrough-close").tap();
    await selectMobileMode(testPage, viewer, "edit");
    await testPage.getByTestId("walkthrough-launcher").tap();
    const hybrid = viewer.getByTestId("hybrid-markdown-editor");
    await expectMarkdownWalkthroughRange(hybrid, ranges[0], hybrid.locator("table"));
    await card.getByTestId("walkthrough-next").tap();
    const paragraph = hybrid.locator(".md-paragraph", {
      hasText: "Long mobile paragraph 45 keeps the editor content below the fold.",
    });
    await expectMarkdownWalkthroughRange(hybrid, ranges[1], paragraph);
    const cardBounds = await card.boundingBox();
    expect(cardBounds).not.toBeNull();
    await paragraph.evaluate((element, targetY) => {
      const scroller = element.closest('[data-testid="hybrid-markdown-editor"]') as HTMLElement;
      const rect = element.getBoundingClientRect();
      scroller.scrollTop += rect.top + rect.height / 2 - targetY;
    }, cardBounds!.y + 60);
    await expect
      .poll(async () =>
        paragraph.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          const hit = document.elementFromPoint(
            rect.left + rect.width / 2,
            rect.top + rect.height / 2,
          );
          return !!hit?.closest('[data-testid="walkthrough-floating"]');
        }),
      )
      .toBe(true);
    await expectMarkdownWalkthroughRange(hybrid, ranges[1], paragraph);
    await expect(testPage.getByTestId("walkthrough-connector")).toHaveCount(0);
    await card.getByTestId("walkthrough-next").tap();
    await expectMarkdownWalkthroughRange(hybrid, ranges[2], paragraph);
    await expect(viewer.locator('[data-testid="walkthrough-editor-range"]:visible')).toHaveCount(1);
    await prCapture.screenshot("mobile-markdown-walkthrough-range", {
      caption: "Phone Markdown retains its section indicator beside the walkthrough sheet",
    });
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await card.getByTestId("walkthrough-close").tap();
    await expect(viewer.getByTestId("walkthrough-editor-range")).toHaveCount(0);
    expect(fs.readFileSync(filePath, "utf8")).toBe(MOBILE_MARKDOWN_CONTENT);
  });

  // @covers AC-UI-MARKDOWN-FILE-EDITING-001.5, AC-UI-MARKDOWN-FILE-EDITING-002.4
  test("review regression: position handoff keeps phone reading context and selection", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fileName = `phone-position-${Date.now()}.md`;
    const { session, filePath, gateway } = await seedMobileMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
    });
    await testPage
      .getByTestId("session-mobile-bottom-nav")
      .getByRole("button", { name: "Files", exact: true })
      .tap();
    await (await session.fileTree.waitForFileTreeNode(fileName)).tap();
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await selectMobileMode(testPage, viewer, "edit");
    const paragraph = viewer
      .getByTestId("hybrid-markdown-editor")
      .locator(".md-paragraph", { hasText: "Long mobile paragraph 40 " });
    await paragraph.scrollIntoViewIfNeeded();
    const point = await paragraph.evaluate((element) => {
      const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
      while (walker.nextNode()) {
        const node = walker.currentNode;
        const offset = node.textContent?.indexOf("Long") ?? -1;
        if (offset < 0) continue;
        const range = document.createRange();
        range.setStart(node, offset);
        range.setEnd(node, offset + 4);
        const rect = range.getBoundingClientRect();
        return { x: rect.right - 1, y: rect.top + rect.height / 2 };
      }
      throw new Error("Missing long-document paragraph");
    });
    await testPage.touchscreen.tap(point.x, point.y);
    await testPage.keyboard.press("Home");
    for (let index = 0; index < 4; index++) await testPage.keyboard.press("ArrowRight");
    for (let index = 0; index < 4; index++) await testPage.keyboard.press("Shift+ArrowLeft");
    await expect
      .poll(() =>
        viewer.locator(".md-editor").evaluate((element) => {
          const input = element.shadowRoot?.querySelector("textarea");
          if (input) return input.value.slice(input.selectionStart, input.selectionEnd);
          const context = (
            element as HTMLElement & {
              editContext: { text: string; selectionStart: number; selectionEnd: number };
            }
          ).editContext;
          return context.text.slice(context.selectionStart, context.selectionEnd);
        }),
      )
      .toBe("Long");
    await selectMobileMode(testPage, viewer, "preview");
    await expect(
      viewer.getByTestId("markdown-preview").locator("p", {
        hasText: "Long mobile paragraph 40 ",
      }),
    ).toBeInViewport();
    await selectMobileMode(testPage, viewer, "source");
    await viewer.locator(".cm-content").focus();
    await testPage.keyboard.type("Selected");
    const saved = gateway.waitForResponse("workspace.file.update");
    await viewer.getByTestId("mobile-file-save").tap();
    await saved;
    expect(fs.readFileSync(filePath, "utf8")).toBe(
      MOBILE_MARKDOWN_CONTENT.replace("Long mobile paragraph 40 ", "Selected mobile paragraph 40 "),
    );
    await selectMobileMode(testPage, viewer, "edit");
    await expect(
      viewer.getByTestId("hybrid-markdown-editor").locator(".md-paragraph", {
        hasText: "Selected mobile paragraph 40 ",
      }),
    ).toBeInViewport();
  });

  // @covers AC-UI-MARKDOWN-FILE-EDITING-003.10
  for (const locale of ["en", "pt-pt"]) {
    test(`review regression: phone actions keep Save reachable at 320px with a remote update (${locale})`, async ({
      testPage,
      apiClient,
      seedData,
      backend,
      prCapture,
    }) => {
      await testPage.setViewportSize({ width: 320, height: 600 });
      const fileName = `phone-actions-${Date.now()}.md`;
      const { session, filePath } = await seedMobileMarkdownSession({
        testPage,
        apiClient,
        seedData,
        backend,
        fileName,
      });
      await testPage
        .context()
        .addCookies([{ name: "kandev_locale", value: locale, url: backend.baseUrl }]);
      await testPage.reload();
      await testPage
        .getByTestId("session-mobile-bottom-nav")
        .getByRole("button", { name: locale === "en" ? "Files" : "Ficheiros", exact: true })
        .tap();
      await (await session.fileTree.waitForFileTreeNode(fileName)).tap();
      const viewer = testPage.getByTestId("mobile-file-viewer-panel");
      await selectMobileMode(testPage, viewer, "source");
      await appendToMobileSource(testPage, viewer, "Local phone draft");
      fs.writeFileSync(filePath, `${MOBILE_MARKDOWN_CONTENT}\nRemote phone content`);
      await expect(viewer.getByTestId("mobile-file-remote-update")).toBeVisible({
        timeout: 20_000,
      });
      for (const width of [320, 393, 767]) {
        await testPage.setViewportSize({ width, height: width === 320 ? 400 : 851 });
        const save = viewer.getByTestId("mobile-file-save");
        await expect(save).toBeEnabled();
        const bounds = await save.boundingBox();
        expect(bounds).not.toBeNull();
        expect(bounds!.x).toBeGreaterThanOrEqual(0);
        expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width);
        expect(bounds!.width).toBeGreaterThanOrEqual(44);
        expect(bounds!.height).toBeGreaterThanOrEqual(44);
        expect(
          await testPage.evaluate(
            () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
          ),
        ).toBe(true);
        if (width === 320) {
          await prCapture.screenshot(`phone-markdown-actions-${locale}`, {
            caption: `320px Markdown actions with a remote update (${locale})`,
          });
        }
      }
      await viewer.getByTestId("mobile-markdown-mode-trigger").tap();
      for (const mode of ["preview", "edit", "source"]) {
        const item = testPage.getByTestId(`mobile-markdown-mode-${mode}`);
        await expect
          .poll(async () => (await item.boundingBox())?.height ?? 0)
          .toBeGreaterThanOrEqual(44);
        const bounds = await item.boundingBox();
        expect(bounds!.x).toBeGreaterThanOrEqual(0);
        expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(767);
      }
      await testPage.keyboard.press("Escape");
      await expect(viewer.getByTestId("mobile-markdown-mode-trigger")).toBeFocused();
      await viewer.getByTestId("mobile-file-actions-trigger").tap();
      const reload = testPage.getByTestId("mobile-file-reload");
      await expect(reload).toBeEnabled();
      await expect
        .poll(async () => (await reload.boundingBox())?.height ?? 0)
        .toBeGreaterThanOrEqual(44);
      await reload.tap();
      await expect(viewer.locator(".cm-content")).toContainText("Remote phone content");
      await expect(viewer.getByTestId("mobile-file-save")).toBeDisabled();
      await testPage.setViewportSize({ width: 768, height: 900 });
      const tablet = testPage.getByTestId("tablet-task-layout");
      await tablet
        .locator(`[data-testid="file-tree-node"][data-path=${JSON.stringify(fileName)}]:visible`)
        .tap();
      await tablet.getByTestId("markdown-mode-source").tap();
      const tabletSave = tablet.getByRole("button", {
        name: locale === "en" ? /^Save/ : /^Guardar/,
      });
      await expect(tabletSave).toBeDisabled();
      await expect(async () => {
        const tabletBounds = await tabletSave.boundingBox();
        expect(tabletBounds).not.toBeNull();
        expect(tabletBounds!.x).toBeGreaterThanOrEqual(0);
        expect(tabletBounds!.x + tabletBounds!.width).toBeLessThanOrEqual(768);
      }).toPass();
    });
  }

  // @covers AC-UI-MARKDOWN-FILE-EDITING-002.15
  test("review regression: hybrid recovery retries on phone without losing the source draft", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await testPage.addInitScript(() => {
      const replaceChildren = Element.prototype.replaceChildren;
      let failed = false;
      Element.prototype.replaceChildren = function (...nodes) {
        if (
          !failed &&
          nodes.length > 0 &&
          this.getAttribute("data-testid") === "hybrid-markdown-editor"
        ) {
          failed = true;
          throw new Error("Expected Markdown initialization failure");
        }
        return replaceChildren.apply(this, nodes);
      };
    });
    const fileName = `phone-retry-${Date.now()}.md`;
    const { session, filePath, gateway } = await seedMobileMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
    });
    await testPage
      .getByTestId("session-mobile-bottom-nav")
      .getByRole("button", { name: "Files", exact: true })
      .tap();
    await (await session.fileTree.waitForFileTreeNode(fileName)).tap();
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await selectMobileMode(testPage, viewer, "source");
    const marker = "\nPhone retry retains this draft";
    await appendToMobileSource(testPage, viewer, marker.slice(1));
    await selectMobileMode(testPage, viewer, "edit", "source");
    await expect(viewer.getByTestId("mobile-markdown-mode-trigger")).toHaveAttribute(
      "data-mode",
      "source",
    );
    await selectMobileMode(testPage, viewer, "edit");
    await expect(viewer.getByTestId("hybrid-markdown-editor")).toContainText(marker.trim());
    const saved = gateway.waitForResponse("workspace.file.update");
    await viewer.getByTestId("mobile-file-save").tap();
    await saved;
    expect(fs.readFileSync(filePath, "utf8")).toBe(`${MOBILE_MARKDOWN_CONTENT}${marker}`);
  });

  // @covers AC-UI-MARKDOWN-FILE-EDITING-001.10
  test("browser parity: phone rendered Edit works without EditContext", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await testPage.addInitScript(() => Reflect.deleteProperty(window, "EditContext"));
    const fileName = `phone-capability-${Date.now()}.md`;
    const { session, filePath, gateway } = await seedMobileMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
    });
    await testPage
      .getByTestId("session-mobile-bottom-nav")
      .getByRole("button", { name: "Files", exact: true })
      .tap();
    await (await session.fileTree.waitForFileTreeNode(fileName)).tap();
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await selectMobileMode(testPage, viewer, "edit");
    const marker = "Phone rendered editing without EditContext";
    await appendToHybrid(testPage, viewer, marker);
    const saved = gateway.waitForResponse("workspace.file.update");
    await viewer.getByTestId("mobile-file-save").tap();
    await saved;
    expect(fs.readFileSync(filePath, "utf8")).toBe(`${MOBILE_MARKDOWN_CONTENT}\n\n${marker}`);
    await selectMobileMode(testPage, viewer, "preview");
    await expect(viewer.getByTestId("markdown-preview")).toContainText(marker);
  });

  // @covers AC-UI-MARKDOWN-FILE-EDITING-003.8
  test("review regression: responsive draft survives 767px to 768px and saves in the new layout", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await testPage.setViewportSize({ width: 767, height: 900 });
    const fileName = `responsive-markdown-${Date.now()}.md`;
    const marker = "retained responsive draft";
    const { session, filePath, gateway } = await seedMobileMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
    });
    await testPage
      .getByTestId("session-mobile-bottom-nav")
      .getByRole("button", { name: "Files", exact: true })
      .tap();
    await (await session.fileTree.waitForFileTreeNode(fileName)).tap();
    const mobile = testPage.getByTestId("mobile-file-viewer-panel");
    await selectMobileMode(testPage, mobile, "source");
    await appendToMobileSource(testPage, mobile, marker);
    await expect(mobile.getByTestId("mobile-file-save")).toBeEnabled();

    await testPage.setViewportSize({ width: 768, height: 900 });
    const tablet = testPage.getByTestId("tablet-task-layout");
    await expect(tablet).toBeVisible();
    await tablet
      .locator(`[data-testid="file-tree-node"][data-path=${JSON.stringify(fileName)}]:visible`)
      .tap();
    const editor = tablet.getByTestId("markdown-file-editor");
    await expect(editor).toBeVisible();
    const source = editor.getByRole("textbox", { name: "Editor content" });
    // Monaco virtualizes visible lines. Check its actual model before saving through the tablet UI.
    await expect
      .poll(() =>
        source.evaluate((element) => {
          const monaco = (window as Window & { monaco?: typeof import("monaco-editor") }).monaco;
          return monaco?.editor
            .getEditors()
            .find((candidate) => candidate.getDomNode()?.contains(element))
            ?.getModel()
            ?.getValue();
        }),
      )
      .toContain(marker);
    const saved = gateway.waitForResponse("workspace.file.update");
    await editor.getByRole("button", { name: /^Save/ }).tap();
    await saved;
    expect(fs.readFileSync(filePath, "utf8")).toBe(`${MOBILE_MARKDOWN_CONTENT}\n${marker}`);

    await testPage.setViewportSize({ width: 767, height: 900 });
    await expect(mobile).toBeVisible();
    await selectMobileMode(testPage, mobile, "preview");
    await expect(mobile.getByTestId("markdown-preview")).toContainText(marker);
    await expect(mobile.getByTestId("mobile-file-save")).toBeDisabled();
  });

  test("edits and saves the hybrid document below the fold and keeps phone controls reachable", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const fileName = `mobile-markdown-${Date.now()}.md`;
    const marker = `mobile saved marker ${Date.now()}`;
    const { session, filePath, gateway } = await seedMobileMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
    });

    await testPage
      .getByTestId("session-mobile-bottom-nav")
      .getByRole("button", { name: "Files", exact: true })
      .tap();
    const fileNode = await session.fileTree.waitForFileTreeNode(fileName);
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    const controls = viewer.getByTestId("mobile-markdown-mode-controls");
    await expect(controls).toBeVisible();
    await expect(viewer.getByTestId("mobile-markdown-mode-trigger")).toHaveAttribute(
      "data-mode",
      "preview",
    );

    for (const button of [
      viewer.getByTestId("mobile-markdown-mode-trigger"),
      viewer.getByTestId("mobile-file-actions-trigger"),
      viewer.getByTestId("mobile-file-save"),
    ]) {
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    await selectMobileMode(testPage, viewer, "edit");
    const hybrid = viewer.getByTestId("hybrid-markdown-editor");
    await expect(hybrid).toBeVisible({ timeout: 15_000 });
    const table = hybrid.locator(".md-table");
    await table.scrollIntoViewIfNeeded();
    await table.locator("td").first().tap();
    await expect(table.locator(".md-glue-tableCellGlue:visible")).toHaveCount(0);
    expect(
      await table
        .locator("tr:not(.md-table-delimiter-row) td")
        .last()
        .evaluate((cell) => getComputedStyle(cell).backgroundColor),
    ).not.toBe("rgba(0, 0, 0, 0)");
    const rowAction = hybrid.getByTestId("markdown-table-row-insert-0");
    const columnAction = hybrid.getByTestId("markdown-table-column-insert-1");
    for (const tableAction of [rowAction, columnAction]) {
      await expect(tableAction).toBeVisible();
      await expect(tableAction.locator("svg")).toHaveCount(1);
      const box = await tableAction.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    const [tableBox, rowActionBox, columnActionBox] = await Promise.all([
      table.boundingBox(),
      rowAction.boundingBox(),
      columnAction.boundingBox(),
    ]);
    expect(tableBox).not.toBeNull();
    expect(rowActionBox).not.toBeNull();
    expect(columnActionBox).not.toBeNull();
    expect(rowActionBox!.x + rowActionBox!.width).toBeLessThanOrEqual(tableBox!.x);
    expect(columnActionBox!.y + columnActionBox!.height).toBeLessThanOrEqual(tableBox!.y);
    await columnAction.tap();
    await rowAction.tap();

    const resizer = hybrid.locator('[data-testid^="markdown-table-resizer-"]').first();
    await expect(resizer).toBeVisible();
    await resizer.scrollIntoViewIfNeeded();
    const resizerBox = await resizer.boundingBox();
    expect(resizerBox).not.toBeNull();
    expect(resizerBox!.width).toBeGreaterThanOrEqual(44);
    expect(resizerBox!.height).toBeGreaterThanOrEqual(44);
    const tableAfterScroll = await table.boundingBox();
    expect(tableAfterScroll).not.toBeNull();
    expect(resizerBox!.y + resizerBox!.height).toBeLessThanOrEqual(tableAfterScroll!.y);
    expect(await resizer.evaluate((element) => getComputedStyle(element).touchAction)).toBe("none");
    await dragTouch(testPage, resizer, 16);
    await expect(hybrid.locator("colgroup")).toHaveCount(1);
    await expect(hybrid.locator("colgroup col").first()).toHaveAttribute("style", /width/);

    const mobileTableContent = `| Area | State |  | Notes |
| --- | --- | --- | --- |
|  |  |  |  |
| Preview | Ready |  | The table remains contained |
`;
    const subheader = hybrid.locator("h4.md-heading", { hasText: "Mobile source markers" });
    await subheader.tap();
    await expect(
      hybrid.locator(":is(.md-ws-newline-glyph, .md-ws-blockbreak-glyph):visible"),
    ).toHaveCount(0);
    const [hybridBox, markerBox] = await Promise.all([
      hybrid.boundingBox(),
      subheader.locator(".md-marker-headingMarker").boundingBox(),
    ]);
    expect(hybridBox).not.toBeNull();
    expect(markerBox).not.toBeNull();
    expect(markerBox!.x).toBeGreaterThanOrEqual(hybridBox!.x);
    const scrollMetrics = await hybrid.evaluate((element) => {
      const scroller = element as HTMLElement;
      const before = { scrollHeight: scroller.scrollHeight, clientHeight: scroller.clientHeight };
      scroller.scrollTop = scroller.scrollHeight;
      return { ...before, scrollTop: scroller.scrollTop };
    });
    expect(scrollMetrics.scrollHeight).toBeGreaterThan(scrollMetrics.clientHeight);
    expect(scrollMetrics.scrollTop).toBeGreaterThan(0);
    await appendToHybrid(testPage, viewer, marker);

    const saveButton = viewer.getByTestId("mobile-file-save");
    await expect(saveButton).toBeEnabled();
    await expect(saveButton).toBeInViewport();
    const saved = gateway.waitForResponse("workspace.file.update");
    await testPage.keyboard.press(process.platform === "darwin" ? "Meta+S" : "Control+S");
    await saved;
    expect(fs.readFileSync(filePath, "utf8")).toBe(
      `${MOBILE_MARKDOWN_CONTENT.replace(
        "| Area | State | Notes |\n| --- | --- | --- |\n| Preview | Ready | The table remains contained |\n",
        mobileTableContent,
      )}\n\n${marker}`,
    );
    expect(fs.readFileSync(filePath, "utf8")).toContain(UNSUPPORTED_MOBILE_MARKDOWN_SOURCE);
    await expect(saveButton).toBeDisabled();

    await selectMobileMode(testPage, viewer, "preview");
    const preview = viewer.getByTestId("markdown-preview");
    await expect(preview).toBeVisible();
    await expect(preview).toContainText(marker);
    await expect(preview.locator("table")).toBeVisible();
    await expect(preview).toContainText("Long mobile paragraph 56");
    const previewScroll = viewer.getByTestId("markdown-preview-scroll-container");
    expect(
      await previewScroll.evaluate((element) => {
        const styles = getComputedStyle(element);
        return { left: styles.paddingLeft, right: styles.paddingRight };
      }),
    ).toEqual({ left: "16px", right: "16px" });
    const previewScrollTop = await previewScroll.evaluate((element) => {
      const scroller = element as HTMLElement;
      scroller.scrollTop = scroller.scrollHeight;
      return scroller.scrollTop;
    });
    expect(previewScrollTop).toBeGreaterThan(0);
    await prCapture.screenshot("mobile-markdown-preview", {
      caption: "Mobile Markdown Preview with contained table content",
    });
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);
    expect(
      await testPage.evaluate(() => {
        const root = document.documentElement;
        return root.scrollHeight <= root.clientHeight + 1;
      }),
    ).toBe(true);

    await selectMobileMode(testPage, viewer, "edit");
    await expect(viewer.getByTestId("mobile-markdown-hybrid-editor-host")).toBeVisible({
      timeout: 15_000,
    });
    await expect(viewer.getByTestId("hybrid-markdown-editor")).toBeVisible({ timeout: 15_000 });
    await selectMobileMode(testPage, viewer, "preview");
    // Mode handoff preserves the visible source region, rather than a pixel-exact bottom anchor.
    await expect(preview.getByText(/Long mobile paragraph 56 /)).toBeInViewport();

    await selectMobileMode(testPage, viewer, "edit");
    const discardedMarker = `mobile discarded marker ${Date.now()}`;
    await appendToHybrid(testPage, viewer, discardedMarker);
    await viewer.getByRole("button", { name: "Back" }).tap();
    const discardDialog = testPage.getByTestId("discard-local-changes-dialog");
    await expect(discardDialog).toBeVisible();
    await discardDialog.getByTestId("discard-local-changes-cancel").tap();
    await expect(viewer).toContainText(discardedMarker);

    await testPage.getByRole("button", { name: "Plan" }).tap();
    await expect(discardDialog).toBeVisible();
    await discardDialog.getByTestId("discard-local-changes-cancel").tap();
    await expect(viewer).toContainText(discardedMarker);

    await viewer.getByRole("button", { name: "Back" }).tap();
    await expect(discardDialog).toBeVisible();
    await discardDialog.getByTestId("discard-local-changes-confirm").tap();
    await expect(session.fileTreeNode(fileName)).toBeVisible({ timeout: 15_000 });
  });
});
