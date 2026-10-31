import path from "node:path";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type { BackendContext } from "../fixtures/backend";
import { SessionPage } from "../pages/session-page";
import { GitHelper, makeGitEnv } from "./git-helper";
import { dwell } from "./causal-waits";
import { showNavigationFiles } from "../tests/task/task-navigation-helpers";

export async function verifyLazyMonaco(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  { info, mobile = false }: { info: TestInfo; mobile?: boolean },
) {
  await page.addInitScript(() =>
    localStorage.setItem(
      "kandev-editor-providers",
      JSON.stringify({
        version: 3,
        state: {
          providers: {
            "code-editor": "monaco",
            "diff-viewer": "pierre-diffs",
            "chat-code-block": "shiki",
            "chat-diff": "pierre-diffs",
            "plan-editor": "tiptap",
          },
        },
      }),
    ),
  );
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile("lazy-editor.ts", "export const lazyEditorReady = true;\n");
  git.stageAll();
  git.commit("seed lazy editor content");
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Lazy editor fixture",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Network.enable");
  const requests: unknown[] = [];
  cdp.on("Network.requestWillBeSent", (event) => {
    if (/\/(monaco-loader|editor\.api2)-/.test(event.request.url)) requests.push(event);
  });
  await page.goto(`/threads?workspace=${seed.workspaceId}`);
  await expect(page.getByTestId("threads-board")).toBeVisible();
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await showNavigationFiles(page, mobile);
  await expect(session.fileTreeNode("lazy-editor.ts")).toBeVisible();
  await dwell(
    page,
    500,
    "negative-assertion",
    "observe late Monaco requests while editor surfaces stay closed",
  );
  const beforeOpen = [...requests];
  expect.soft(beforeOpen, "closed editor surfaces must not load the Monaco runtime").toEqual([]);
  if (mobile) {
    await session.fileTreeNode("lazy-editor.ts").tap();
    await expect(page.getByTestId("mobile-file-viewer-panel").locator(".cm-content")).toContainText(
      "lazyEditorReady",
    );
    await dwell(
      page,
      500,
      "negative-assertion",
      "observe late Monaco requests after the phone viewer opens",
    );
    expect(requests).toEqual([]);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  } else {
    await session.fileTreeNode("lazy-editor.ts").click();
    await expect(page.locator(".monaco-editor:visible .view-lines")).toContainText(
      "lazyEditorReady",
      { timeout: 20_000 },
    );
    expect(requests.length).toBeGreaterThan(0);
    await page.locator(".monaco-editor:visible .view-lines").click();
    await page.keyboard.press("ControlOrMeta+End");
    await page.keyboard.type(" // lazy input works");
    await expect(page.locator(".monaco-editor:visible .view-lines")).toContainText(
      "lazy input works",
    );
  }
  const resources = await page.evaluate(() =>
    performance.getEntriesByType("resource").map((entry) => {
      const r = entry as PerformanceResourceTiming;
      return {
        name: new URL(r.name).pathname,
        encodedBytes: r.encodedBodySize,
        decodedBytes: r.decodedBodySize,
      };
    }),
  );
  const compressedScripts = resources.filter(
    (r) => r.name.endsWith(".js") && r.decodedBytes > 1024 && r.encodedBytes > 0,
  );
  expect(compressedScripts.some((r) => r.encodedBytes < r.decodedBytes)).toBe(true);
  const output = { beforeOpen, afterOpen: requests, resources };
  await info.attach("monaco-import-initiators", {
    body: JSON.stringify(output, null, 2),
    contentType: "application/json",
  });
  await cdp.detach();
}
