import { test, expect } from "@playwright/test";
import { execSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const BACKEND = "http://localhost:8085";
const FRONTEND = "http://localhost:3001";
const SCREENSHOTS = "/tmp/expansion-debug-screenshots";

fs.mkdirSync(SCREENSHOTS, { recursive: true });

async function api<T>(method: string, endpoint: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BACKEND}${endpoint}`, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`API ${method} ${endpoint} → ${res.status}: ${text}`);
  }
  return res.json() as Promise<T>;
}

async function seedTask() {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-dbg-"));
  const repoDir = path.join(tmpDir, "repo");
  fs.mkdirSync(repoDir, { recursive: true });

  const gitEnv = {
    ...process.env,
    GIT_AUTHOR_NAME: "Debug", GIT_AUTHOR_EMAIL: "debug@test.local",
    GIT_COMMITTER_NAME: "Debug", GIT_COMMITTER_EMAIL: "debug@test.local",
  };
  execSync("git init -b main", { cwd: repoDir, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });

  const ws = await api<{ id: string }>("POST", "/api/v1/workspaces", { name: "Debug WS" });
  const wf = await api<{ id: string }>("POST", "/api/v1/workflows", {
    workspace_id: ws.id, name: "Debug WF", workflow_template_id: "simple",
  });
  const { steps } = await api<{ steps: Array<{ id: string; is_start_step: boolean; position: number }> }>(
    "GET", `/api/v1/workflows/${wf.id}/workflow/steps`,
  );
  const startStep = steps.sort((a, b) => a.position - b.position).find((s) => s.is_start_step) ?? steps[0];
  const repo = await api<{ id: string }>("POST", `/api/v1/workspaces/${ws.id}/repositories`, {
    name: "Debug Repo", source_type: "local", local_path: repoDir, default_branch: "main",
  });
  const { agents } = await api<{ agents: Array<{ profiles: Array<{ id: string }> }> }>("GET", "/api/v1/agents");
  const agentProfileId = agents[0]?.profiles[0]?.id;
  if (!agentProfileId) throw new Error("No mock agent profile found");

  const task = await api<{ id: string; session_id?: string }>("POST", "/api/v1/tasks", {
    workspace_id: ws.id,
    title: "Debug expansion",
    // Pass the scenario as description so the mock agent runs it on start
    description: "/e2e:diff-expansion-setup",
    workflow_id: wf.id, workflow_step_id: startStep.id,
    start_agent: true, agent_profile_id: agentProfileId,
    repositories: [{ repository_id: repo.id }],
  });
  if (!task.session_id) throw new Error(`Task has no session_id: ${JSON.stringify(task)}`);
  await api("PATCH", "/api/v1/user/settings", { workspace_id: ws.id, workflow_filter_id: wf.id });
  return task.session_id;
}

test("debug: diff expansion flow with screenshots", async ({ page }) => {
  const expansionLogs: string[] = [];
  page.on("console", (msg) => {
    const text = msg.text();
    if (text.includes("expansion") || text.includes("Expansion") || text.includes("expandable") || text.includes("FileDiff") || text.includes("expand") || text.includes("DiffViewer")) {
      expansionLogs.push(`[${msg.type()}] ${text}`);
    }
  });

  await page.addInitScript(() => {
    localStorage.setItem("kandev.onboarding.completed", "true");
    window.__KANDEV_API_BASE_URL = "http://localhost:8085";
  });

  const sessionId = await seedTask();
  console.log("Session ID:", sessionId);

  // 1. Open the session at /s/<session_id>
  await page.goto(`${FRONTEND}/s/${sessionId}`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1000);
  await page.screenshot({ path: `${SCREENSHOTS}/01-session-opened.png` });

  // 2. Wait for agent to complete its initial turn (description = "/e2e:diff-expansion-setup")
  const chat = page.getByTestId("session-chat");
  await chat.waitFor({ timeout: 15000 });
  await expect(
    chat.getByText("diff-expansion-setup complete", { exact: false })
  ).toBeVisible({ timeout: 30000 });
  await page.screenshot({ path: `${SCREENSHOTS}/02-agent-complete.png` });
  console.log("Agent completed. Expansion logs so far:", expansionLogs);

  // 3. Click the Changes dockview tab
  const changesTab = page.locator(".dv-default-tab", { hasText: "Changes" });
  await changesTab.waitFor({ timeout: 10000 });
  await changesTab.click();
  await page.waitForTimeout(2000);
  await page.screenshot({ path: `${SCREENSHOTS}/03-changes-tab.png` });

  // 4. Find expansion_test.go and open its diff
  const fileRow = page.locator("button, [role='button'], [class*='file']")
    .filter({ hasText: "expansion_test.go" })
    .first();
  await fileRow.waitFor({ timeout: 10000 });
  await page.screenshot({ path: `${SCREENSHOTS}/04-changes-list.png` });
  await fileRow.click();
  await page.waitForTimeout(3000);
  await page.screenshot({ path: `${SCREENSHOTS}/05-diff-opened.png` });

  // 5. Check which diff viewer is being used (Monaco vs Pierre)
  const monacoEditors = await page.locator('.monaco-diff-editor, .monaco-editor').count();
  const pierreViewer = await page.locator('.diff-viewer, [class*="pierre"], [data-testid*="diff"]').count();
  console.log(`  Monaco editors: ${monacoEditors}, Pierre-like elements: ${pierreViewer}`);

  // 6. Check editor provider setting via localStorage/window
  const diffViewerProvider = await page.evaluate(() => {
    return (window as unknown as Record<string, unknown>).__KANDEV_EDITOR_PROVIDERS
      ?? localStorage.getItem("kandev.editor.diff-viewer")
      ?? "unknown";
  });
  console.log(`  Diff viewer provider setting: ${diffViewerProvider}`);

  // 7. Log expansion state from console
  console.log("Expansion logs after opening diff:", expansionLogs);

  // 8. Inspect diff DOM
  const diffEl = page.locator('.diff-viewer').first();
  const diffHtml = await diffEl.innerHTML().catch(() => "(not found)");
  fs.writeFileSync(`${SCREENSHOTS}/diff-html.txt`, diffHtml.slice(0, 100000));
  console.log("Diff HTML length:", diffHtml.length);

  // 9. Query inside shadow DOM for pierre/diffs expand UI
  const shadowDomInfo = await page.evaluate(() => {
    const results: Record<string, unknown> = {};
    // Pierre diffs uses <pierre-diffs> custom element with shadow DOM
    const diffsEls = document.querySelectorAll("diffs-container");
    results.pierreDiffsCount = diffsEls.length;

    let expandBtns = 0;
    let separators = 0;
    let lineInfoSeparators = 0;
    let simpleSeparators = 0;
    const expandIndexes: string[] = [];

    for (const el of diffsEls) {
      const shadow = el.shadowRoot;
      if (!shadow) continue;
      expandBtns += shadow.querySelectorAll("[data-expand-button]").length;
      separators += shadow.querySelectorAll("[data-separator]").length;
      lineInfoSeparators += shadow.querySelectorAll("[data-separator='line-info']").length;
      simpleSeparators += shadow.querySelectorAll("[data-separator='']:not([data-separator='line-info']):not([data-separator='metadata']):not([data-separator='custom'])").length;
      for (const sep of shadow.querySelectorAll("[data-expand-index]")) {
        expandIndexes.push(sep.getAttribute("data-expand-index") ?? "?");
      }
    }
    results.expandButtons = expandBtns;
    results.totalSeparators = separators;
    results.lineInfoSeparators = lineInfoSeparators;
    results.simpleSeparators = simpleSeparators;
    results.expandIndexes = expandIndexes;
    return results;
  });
  fs.writeFileSync(`${SCREENSHOTS}/shadow-dom-info.json`, JSON.stringify(shadowDomInfo, null, 2));

  // Also dump the full separator HTML from shadow DOM
  const separatorHtml = await page.evaluate(() => {
    const results: string[] = [];
    for (const el of document.querySelectorAll("diffs-container")) {
      const shadow = el.shadowRoot;
      if (!shadow) continue;
      for (const sep of shadow.querySelectorAll("[data-separator]")) {
        results.push(sep.outerHTML.slice(0, 2000));
      }
    }
    return results;
  });
  fs.writeFileSync(`${SCREENSHOTS}/separators.json`, JSON.stringify(separatorHtml, null, 2));

  // Dump expansion-related console logs
  fs.writeFileSync(`${SCREENSHOTS}/expansion-logs.json`, JSON.stringify(expansionLogs, null, 2));

  // 10. Full page screenshot
  await page.screenshot({ path: `${SCREENSHOTS}/08-full-page.png`, fullPage: true });
  console.log(`\nScreenshots: ${SCREENSHOTS}/`);

  expect(diffHtml.length).toBeGreaterThan(100);
  // Verify expand buttons exist in shadow DOM
  expect(shadowDomInfo.expandButtons).toBeGreaterThan(0);
});
