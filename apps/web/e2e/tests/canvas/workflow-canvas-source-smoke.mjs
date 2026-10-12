import { existsSync, readFileSync } from "node:fs";
import { createServer } from "node:http";
import path from "node:path";
import { chromium } from "@playwright/test";

const source = path.resolve(process.argv[2] || "");
if (!existsSync(path.join(source, "script.js")))
  throw new Error("Pass the canvas source directory");
const FLOW_ID = "flow";
const TASK_TITLE = "Review this change";
const envelope = (items) => ({ items, page_info: { has_more: false } });
const workflow = { id: FLOW_ID, name: "Feature delivery" };
const steps = [
  { id: "draft", name: "Draft", position: 0 },
  { id: "review", name: "Review", position: 1 },
];
const task = {
  id: "task",
  title: TASK_TITLE,
  state: "IN_PROGRESS",
  workflow_id: FLOW_ID,
  workflow_step_id: "draft",
  updated_at: "2026-09-22T10:00:00Z",
};
const moves = [
  {
    id: "2",
    from_workflow_id: FLOW_ID,
    from_workflow_step_id: "review",
    to_workflow_id: FLOW_ID,
    to_workflow_step_id: "draft",
    occurred_at: "2026-09-22T09:00:00Z",
    trigger: "manual",
  },
];
let scope = "task";
let historyStatus = 200;

function mockData(api) {
  switch (api) {
    case "context":
      return { scope_kind: scope, task_id: scope === "task" ? "task" : undefined };
    case "data/tasks/task":
      return task;
    case "data/tasks":
      return envelope([task]);
    case "data/workflows":
      return envelope([workflow]);
    case "data/workflows/flow/steps":
      return envelope(steps);
    case "data/workflows/flow/transition-groups":
      return envelope([{ kind: "within", from_step_id: "review", to_step_id: "draft", count: 1 }]);
    case "data/tasks/task/step-transitions":
      return historyStatus === 200 ? envelope(moves) : { error: "unavailable" };
    default:
      return null;
  }
}

function serve(request, response) {
  const pathname = new URL(request.url, "http://localhost").pathname;
  if (pathname.includes("/_kandev/v1/")) {
    const api = pathname.split("/_kandev/v1/")[1];
    const body = mockData(api);
    let status = 200;
    if (body === null) status = 404;
    else if (api === "data/tasks/task/step-transitions") status = historyStatus;
    response.writeHead(status, { "content-type": "application/json" });
    response.end(JSON.stringify(body ?? {}));
    return;
  }
  const file = path.join(source, pathname === "/" ? "index.html" : pathname);
  if (!file.startsWith(source + path.sep)) {
    response.writeHead(403);
    response.end();
    return;
  }
  try {
    let contentType = "text/html";
    if (file.endsWith(".js")) contentType = "text/javascript";
    else if (file.endsWith(".css")) contentType = "text/css";
    response.writeHead(200, { "content-type": contentType });
    response.end(readFileSync(file));
  } catch {
    response.writeHead(404);
    response.end();
  }
}

async function checkTask(browser, url) {
  const page = await browser.newPage();
  await page.addInitScript(() => {
    window.__tones = 0;
    window.AudioContext = class {
      currentTime = 0;
      destination = {};
      createOscillator() {
        return {
          frequency: { value: 0 },
          connect() {},
          start() {
            window.__tones += 1;
          },
          stop() {},
        };
      }
      createGain() {
        return { gain: { value: 0 }, connect() {} };
      }
    };
  });
  await page.goto(url);
  await page.getByText(TASK_TITLE).first().waitFor();
  await page.getByText("Return: Review → Draft").waitFor();
  if (await page.getByText("sample tasks").count()) throw new Error("Sample data visible");
  task.workflow_step_id = "review";
  await page.getByRole("button", { name: "Refresh" }).click();
  await page.getByText(TASK_TITLE).first().waitFor();
  if ((await page.evaluate(() => window.__tones)) !== 0)
    throw new Error("Sound played without opt-in");
  await page.getByLabel("Play sound for new moves").check();
  task.workflow_step_id = "draft";
  await page.getByRole("button", { name: "Refresh" }).click();
  await page.waitForFunction(() => window.__tones === 1);
  await page.close();
}

async function checkWorkspacePhone(browser, url) {
  scope = "workspace";
  const page = await browser.newPage({
    viewport: { width: 390, height: 800 },
    isMobile: true,
    hasTouch: true,
  });
  await page.goto(url);
  await page.getByLabel("Workflow").waitFor();
  await page.getByRole("button", { name: "Task trail" }).click();
  await page.getByText("Return: Review → Draft").waitFor();
  if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth))
    throw new Error("Horizontal mobile overflow");
  await page.close();
}

async function checkOlderHost(browser, url) {
  scope = "task";
  historyStatus = 404;
  const page = await browser.newPage();
  await page.goto(url);
  await page
    .getByText(/does not provide transition history/)
    .first()
    .waitFor();
  await page.getByText(TASK_TITLE).first().waitFor();
  await page.close();
}

const server = createServer(serve);
try {
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const url = `http://127.0.0.1:${server.address().port}/`;
  const browser = await chromium.launch({ headless: true });
  try {
    await checkTask(browser, url);
    await checkWorkspacePhone(browser, url);
    await checkOlderHost(browser, url);
    console.log("Exact canvas source passed task, workspace phone, older-host, and sound checks.");
  } finally {
    await browser.close();
  }
} finally {
  await new Promise((resolve) => server.close(resolve));
}
