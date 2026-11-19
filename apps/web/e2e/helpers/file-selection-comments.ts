import fs from "node:fs";
import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { expect } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";

export const CODE_SELECTION_QUOTE = "const target = 1;";
export const CODE_SELECTION_SENTINEL = "target";
export const CODE_SELECTION_LINE = 57;
export const CODE_SELECTION_CONTENT = [
  'export const header = "selection comment fixture";',
  ...Array.from(
    { length: 55 },
    (_, index) => `export const filler${index + 1} = "line ${index + 1}";`,
  ),
  CODE_SELECTION_QUOTE,
].join("\n");
const PROVIDER_KEY = "kandev-editor-providers";
const BASELINE_KEY = "kandev-e2e-editor-providers-baseline";
const NULL_BASELINE = "__KANDEV_E2E_NULL__";

export type CodeEditorProvider = "codemirror" | "monaco";

type CodeSelectionCoordinates = {
  start: { x: number; y: number };
  end: { x: number; y: number };
};

async function readStableCodeSelectionCoordinates(
  line: Element,
  quote: string,
): Promise<CodeSelectionCoordinates | null> {
  const nextFrame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  const rectSnapshot = () => {
    const rect = line.getBoundingClientRect();
    return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
  };
  const sameRect = (
    left: ReturnType<typeof rectSnapshot>,
    right: ReturnType<typeof rectSnapshot>,
  ) =>
    Math.abs(left.x - right.x) < 0.5 &&
    Math.abs(left.y - right.y) < 0.5 &&
    Math.abs(left.width - right.width) < 0.5 &&
    Math.abs(left.height - right.height) < 0.5;

  if (!line.isConnected) return null;
  const firstRect = rectSnapshot();
  await nextFrame();
  if (!line.isConnected) return null;
  const secondRect = rectSnapshot();
  await nextFrame();
  if (!line.isConnected || !sameRect(firstRect, secondRect)) return null;
  if (!sameRect(secondRect, rectSnapshot())) return null;

  const walker = document.createTreeWalker(line, NodeFilter.SHOW_TEXT);
  const textNodes: Text[] = [];
  let node = walker.nextNode();
  while (node) {
    textNodes.push(node as Text);
    node = walker.nextNode();
  }
  const text = textNodes
    .map((textNode) => textNode.nodeValue ?? "")
    .join("")
    .replace(/\u00a0/g, " ");
  const startIndex = text.indexOf(quote);
  if (startIndex < 0) return null;
  const endIndex = startIndex + quote.length;
  const locate = (index: number) => {
    let offset = 0;
    for (const textNode of textNodes) {
      const nextOffset = offset + (textNode.nodeValue?.length ?? 0);
      if (index <= nextOffset) return { node: textNode, offset: index - offset };
      offset = nextOffset;
    }
    return null;
  };
  const startPosition = locate(startIndex);
  const endPosition = locate(endIndex);
  if (!startPosition || !endPosition) return null;
  const range = document.createRange();
  range.setStart(startPosition.node, startPosition.offset);
  range.setEnd(endPosition.node, endPosition.offset);
  const rect = range.getBoundingClientRect();
  if (rect.width === 0 || rect.height === 0) return null;
  const y = rect.top + rect.height / 2;
  return { start: { x: rect.left + 1, y }, end: { x: rect.right - 1, y } };
}

export async function setCodeEditorProvider(page: Page, provider: CodeEditorProvider) {
  await page.addInitScript(
    ({ provider, providerKey, baselineKey, nullBaseline }) => {
      if (localStorage.getItem(baselineKey) === null) {
        localStorage.setItem(baselineKey, localStorage.getItem(providerKey) ?? nullBaseline);
      }
      let existing: { version?: number; state?: { providers?: Record<string, string> } } = {};
      try {
        existing = JSON.parse(localStorage.getItem(providerKey) ?? "{}");
      } catch {
        existing = {};
      }
      localStorage.setItem(
        providerKey,
        JSON.stringify({
          version: 3,
          state: {
            ...existing.state,
            providers: { ...existing.state?.providers, "code-editor": provider },
          },
        }),
      );
    },
    { provider, providerKey: PROVIDER_KEY, baselineKey: BASELINE_KEY, nullBaseline: NULL_BASELINE },
  );
}

export async function restoreCodeEditorProvider(page: Page) {
  if (page.isClosed()) return;
  return page.evaluate(
    ({ providerKey, baselineKey, nullBaseline }) => {
      const baseline = localStorage.getItem(baselineKey);
      if (baseline !== null) {
        if (baseline === nullBaseline) localStorage.removeItem(providerKey);
        else localStorage.setItem(providerKey, baseline);
        localStorage.removeItem(baselineKey);
      }
    },
    { providerKey: PROVIDER_KEY, baselineKey: BASELINE_KEY, nullBaseline: NULL_BASELINE },
  );
}

export async function openCodeFileForSelectionComment({
  page,
  apiClient,
  seedData,
  fileName,
  title,
  provider,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  fileName: string;
  title: string;
  provider: CodeEditorProvider;
}) {
  await setCodeEditorProvider(page, provider);
  const profile = seedData.agentProfileId;
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    executor_profile_id: seedData.worktreeExecutorProfileId,
  });

  let worktreePath = "";
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(task.id);
        worktreePath =
          environment?.repos?.find(
            (repository) => repository.repository_id === seedData.repositoryId,
          )?.worktree_path ??
          environment?.workspace_path ??
          "";
        return environment?.status === "ready" && worktreePath && fs.existsSync(worktreePath);
      },
      { timeout: 30_000 },
    )
    .toBeTruthy();

  const diskPath = path.join(worktreePath, fileName);
  fs.writeFileSync(diskPath, CODE_SELECTION_CONTENT);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  const isMobile = (page.viewportSize()?.width ?? Number.POSITIVE_INFINITY) < 768;
  const filesTab = page.getByRole("button", { name: "Files", exact: true });
  if (isMobile) await filesTab.tap();
  else await session.clickTab("Files");
  await expect(session.files).toBeVisible({ timeout: 10_000 });
  const file = await session.fileTree.waitForFileTreeNode(fileName, 30_000);
  if (isMobile) await file.tap();
  else await file.click();

  if (isMobile) {
    await expect(page.getByTestId("mobile-file-viewer-panel")).toBeVisible({ timeout: 15_000 });
  } else {
    await expect(page.locator(`.dv-default-tab:has-text('${fileName}')`)).toBeVisible({
      timeout: 15_000,
    });
  }

  const editor =
    provider === "codemirror"
      ? page.locator(".cm-editor:visible").last()
      : page.locator(".monaco-editor:visible").last();
  await expect(editor).toBeVisible({ timeout: 15_000 });
  const owner = page.getByTestId(`${provider}-editor-area`);
  await expect(owner).toBeVisible();
  if (provider === "codemirror") {
    await expect(owner.locator(".cm-content")).toHaveAttribute("contenteditable", "false");
  } else {
    await expect(owner.getByRole("textbox", { name: "Editor content" })).toHaveAttribute(
      "aria-autocomplete",
      "none",
    );
  }
  return {
    task,
    session,
    sessionId: task.session_id,
    diskPath,
    repositoryName: "",
    owner,
    editor,
    isMobile,
  };
}

export async function selectFinalCodeLine({
  page,
  owner,
  provider,
}: {
  page: Page;
  owner: Locator;
  provider: CodeEditorProvider;
}) {
  const scroller =
    provider === "codemirror"
      ? owner.locator(".cm-scroller")
      : owner.locator(".monaco-scrollable-element").first();
  await scroller.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  const finalLine =
    provider === "codemirror"
      ? owner.locator(".cm-line").filter({ hasText: CODE_SELECTION_QUOTE }).last()
      : owner.locator(".view-line").filter({ hasText: CODE_SELECTION_QUOTE }).last();
  await expect(finalLine).toBeVisible();
  let coordinates: CodeSelectionCoordinates | null = null;
  await expect
    .poll(
      async () => {
        coordinates = await finalLine
          .evaluate(readStableCodeSelectionCoordinates, CODE_SELECTION_QUOTE)
          .catch(() => null);
        return coordinates !== null;
      },
      { message: "editor line geometry did not settle before code selection" },
    )
    .toBe(true);
  if (!coordinates) throw new Error("Could not resolve stable code selection coordinates");
  const { start, end } = coordinates;
  await page.mouse.move(start.x, start.y);
  await page.mouse.down();
  await page.mouse.move(end.x, end.y, { steps: 8 });
  await page.mouse.up();
}

export async function expectStoredCodeComment({
  page,
  sessionId,
  fileName,
  repositoryName,
  commentText,
}: {
  page: Page;
  sessionId: string;
  fileName: string;
  repositoryName: string;
  commentText: string;
}) {
  const readComments = async () =>
    page.evaluate((id) => {
      const raw = window.sessionStorage.getItem(`kandev.comments.${id}`);
      return raw ? (JSON.parse(raw) as Array<Record<string, unknown>>) : [];
    }, sessionId);
  try {
    await expect
      .poll(async () => {
        const comments = await readComments();
        return comments.some(
          (comment) =>
            comment.source === "diff" &&
            comment.status === "pending" &&
            comment.filePath === fileName &&
            comment.repositoryName === repositoryName &&
            comment.codeContent === CODE_SELECTION_QUOTE &&
            comment.text === commentText &&
            comment.startLine === CODE_SELECTION_LINE &&
            comment.endLine === CODE_SELECTION_LINE,
        );
      })
      .toBe(true);
  } catch (error) {
    const comments = await readComments();
    throw new Error(
      `Code comment did not match expected fields. Stored comments: ${JSON.stringify(comments)}`,
      {
        cause: error,
      },
    );
  }
}
