/**
 * Standalone Playwright debug script for diff expansion.
 * Run with: npx tsx e2e/debug-expansion.ts
 *
 * Seeds its own workspace/repo against the running dev server (localhost:3001)
 * and backend (localhost:8085), then drives the full expansion flow and
 * takes screenshots at each step.
 */

import { chromium } from "@playwright/test";
import { execSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const BACKEND = "http://localhost:8085";
const FRONTEND = "http://localhost:3001";
const SCREENSHOTS = path.resolve("/tmp/expansion-debug-screenshots");

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

async function seed() {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-debug-"));
  const repoDir = path.join(tmpDir, "repo");
  fs.mkdirSync(repoDir, { recursive: true });

  const gitEnv = {
    ...process.env,
    GIT_AUTHOR_NAME: "Debug",
    GIT_AUTHOR_EMAIL: "debug@test.local",
    GIT_COMMITTER_NAME: "Debug",
    GIT_COMMITTER_EMAIL: "debug@test.local",
  };
  execSync("git init -b main", { cwd: repoDir, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });

  const ws = await api<{ id: string }>("POST", "/api/v1/workspaces", { name: "Debug WS" });
  const wf = await api<{ id: string }>("POST", "/api/v1/workflows", {
    workspace_id: ws.id,
    name: "Debug WF",
    workflow_template_id: "simple",
  });
  const { steps } = await api<{
    steps: Array<{ id: string; is_start_step: boolean; position: number }>;
  }>("GET", `/api/v1/workflows/${wf.id}/workflow/steps`);
  const sorted = steps.sort((a, b) => a.position - b.position);
  const startStep = sorted.find((s) => s.is_start_step) ?? sorted[0];

  const repo = await api<{ id: string }>("POST", `/api/v1/workspaces/${ws.id}/repositories`, {
    name: "Debug Repo",
    source_type: "local",
    local_path: repoDir,
    default_branch: "main",
  });

  const { agents } = await api<{ agents: Array<{ profiles: Array<{ id: string }> }> }>(
    "GET",
    "/api/v1/agents",
  );
  const agentProfileId = agents[0]?.profiles[0]?.id;
  if (!agentProfileId) throw new Error("No agent profile found — is KANDEV_MOCK_AGENT=true?");

  const task = await api<{ task: { id: string } }>("POST", "/api/v1/tasks", {
    workspace_id: ws.id,
    title: "Debug expansion task",
    description: "",
    workflow_id: wf.id,
    workflow_step_id: startStep.id,
    start_agent: true,
    agent_profile_id: agentProfileId,
    repositories: [{ repository_id: repo.id }],
  });

  await api("PATCH", "/api/v1/user/settings", {
    workspace_id: ws.id,
    workflow_filter_id: wf.id,
  });

  return { taskId: task.task.id, backendUrl: BACKEND };
}

async function run() {
  console.log("Seeding backend data...");
  const { taskId, backendUrl } = await seed();
  console.log(`Task ID: ${taskId}`);

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ baseURL: FRONTEND });
  const page = await context.newPage();

  // Collect console logs
  const logs: string[] = [];
  page.on("console", (msg) => {
    const text = `[${msg.type()}] ${msg.text()}`;
    logs.push(text);
    if (
      msg.text().includes("Expansion") ||
      msg.text().includes("expandable") ||
      msg.text().includes("expandUnchanged")
    ) {
      console.log("  CONSOLE:", text);
    }
  });
  page.on("pageerror", (err) => console.error("  PAGE ERROR:", err.message));

  await page.addInitScript(
    ({ backendUrl: url }: { backendUrl: string }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_BASE_URL = url;
    },
    { backendUrl },
  );

  console.log("Opening frontend...");
  await page.goto(FRONTEND, { waitUntil: "networkidle" });
  await page.screenshot({ path: path.join(SCREENSHOTS, "01-home.png") });
  console.log("  Screenshot: 01-home.png");

  // Navigate to the task
  console.log(`Navigating to task ${taskId}...`);
  await page.goto(`${FRONTEND}/tasks/${taskId}`, { waitUntil: "networkidle" });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: path.join(SCREENSHOTS, "02-task-opened.png") });
  console.log("  Screenshot: 02-task-opened.png");

  // Type and send the scenario command
  console.log("Sending /e2e:diff-expansion-setup command...");
  const input = page.locator('textarea, [contenteditable="true"], input[type="text"]').first();
  await input.click();
  await input.fill("/e2e:diff-expansion-setup");
  await page.screenshot({ path: path.join(SCREENSHOTS, "03-command-typed.png") });

  await page.keyboard.press("Enter");
  await page.waitForTimeout(500);
  await page.screenshot({ path: path.join(SCREENSHOTS, "04-command-sent.png") });

  // Wait for the agent to complete (look for "diff-expansion-setup complete" in the UI)
  console.log("Waiting for agent to complete...");
  try {
    await page.waitForSelector('text="diff-expansion-setup complete"', { timeout: 30000 });
    console.log("  Agent completed!");
  } catch {
    console.log("  Timed out waiting for completion text — continuing anyway");
  }
  await page.screenshot({ path: path.join(SCREENSHOTS, "05-agent-complete.png") });

  // Click the Changes tab
  console.log("Looking for Changes tab...");
  const changesTab = page
    .locator('[data-title="Changes"], [title="Changes"], text="Changes"')
    .first();
  try {
    await changesTab.waitFor({ timeout: 5000 });
    await changesTab.click();
    await page.waitForTimeout(1500);
    await page.screenshot({ path: path.join(SCREENSHOTS, "06-changes-tab.png") });
    console.log("  Screenshot: 06-changes-tab.png");
  } catch {
    console.log("  Could not find/click Changes tab");
    await page.screenshot({ path: path.join(SCREENSHOTS, "06-no-changes-tab.png") });
  }

  // Look for the expansion_test.go file in the changes list
  console.log("Looking for expansion_test.go in changes...");
  await page.waitForTimeout(2000);
  await page.screenshot({ path: path.join(SCREENSHOTS, "07-changes-list.png") });
  console.log("  Screenshot: 07-changes-list.png");

  const fileEntry = page.locator('text="expansion_test.go"').first();
  try {
    await fileEntry.waitFor({ timeout: 5000 });
    console.log("  Found expansion_test.go — clicking...");
    await fileEntry.click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: path.join(SCREENSHOTS, "08-diff-opened.png") });
    console.log("  Screenshot: 08-diff-opened.png");
  } catch {
    console.log("  expansion_test.go not found in changes list");
  }

  // Check for expansion handles
  await page.waitForTimeout(3000);
  await page.screenshot({ path: path.join(SCREENSHOTS, "09-after-wait.png") });

  const expandBtns = await page
    .locator(
      '[aria-label*="xpand" i], [title*="xpand" i], [class*="expand"], button:has-text("↕"), button:has-text("expand")',
    )
    .all();
  console.log(`  Found ${expandBtns.length} potential expand button(s)`);

  // Dump all interactive elements near the diff
  const allBtns = await page.locator("button").all();
  console.log(`  Total buttons on page: ${allBtns.length}`);
  for (const btn of allBtns.slice(0, 20)) {
    const text = await btn.textContent().catch(() => "");
    const ariaLabel = await btn.getAttribute("aria-label").catch(() => "");
    const cls = await btn.getAttribute("class").catch(() => "");
    if (text || ariaLabel) {
      console.log(
        `    button: text="${text?.trim()}" aria-label="${ariaLabel}" class="${cls?.slice(0, 60)}"`,
      );
    }
  }

  await page.screenshot({ path: path.join(SCREENSHOTS, "10-final.png"), fullPage: true });
  console.log("  Screenshot: 10-final.png (full page)");

  // Print relevant console logs
  const expansionLogs = logs.filter(
    (l) =>
      l.includes("Expansion") ||
      l.includes("expandable") ||
      l.includes("expand") ||
      l.includes("FileDiff"),
  );
  if (expansionLogs.length > 0) {
    console.log("\nExpansion-related console logs:");
    expansionLogs.forEach((l) => console.log(" ", l));
  } else {
    console.log("\nNo expansion-related console logs captured.");
  }

  fs.writeFileSync(path.join(SCREENSHOTS, "console.log"), logs.join("\n"));
  console.log(`\nAll console logs saved to ${SCREENSHOTS}/console.log`);
  console.log(`Screenshots saved to ${SCREENSHOTS}/`);

  await browser.close();
}

run().catch((err) => {
  console.error("Debug script failed:", err);
  process.exit(1);
});
