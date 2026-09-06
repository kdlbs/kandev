// Capture the documentation screenshots for docs/public/team-access.md from the
// seeded demo instance. Run after scripts/demo-team-access.sh up:
//
//   cd apps/web && node scripts/team-access-docs-shots.mjs
import { chromium } from "@playwright/test";
import { mkdirSync } from "node:fs";

const BASE = process.env.KANDEV_DEMO_URL ?? "http://127.0.0.1:8231";
const OUT = process.env.SHOT_DIR ?? "../../docs/screenshots";
const PASSWORD = "kandev-demo-pw-1";
mkdirSync(OUT, { recursive: true });

async function signIn(context, email) {
  const page = await context.newPage();
  // The hermetic demo has no agent CLIs installed, so the board route would
  // otherwise open the onboarding dialog over every shot.
  await page.addInitScript(() =>
    window.localStorage.setItem("kandev.onboarding.completed", "true"),
  );
  await page.goto(`${BASE}/login`, { waitUntil: "domcontentloaded" });
  await page.getByLabel(/email/i).fill(email);
  await page.getByLabel(/password/i).fill(PASSWORD);
  await Promise.all([
    page.waitForResponse(
      (r) => r.url().includes("/api/v1/auth/login") && r.request().method() === "POST",
    ),
    page.getByRole("button", { name: /sign in|log in/i }).click(),
  ]);
  await page.waitForLoadState("networkidle");
  // Fail loudly. Without this a wrong password or a seed that uses different
  // emails leaves the browser sitting on /login, and every screenshot below
  // silently captures the sign-in card over a good committed one.
  if (new URL(page.url()).pathname.startsWith("/login")) {
    throw new Error(`sign in as ${email} did not leave /login: wrong seed or password?`);
  }
  for (let i = 0; i < 6; i += 1) {
    const skip = page.getByRole("button", { name: /^skip$/i });
    if (!(await skip.count())) break;
    await skip
      .first()
      .click()
      .catch(() => {});
    await page.waitForTimeout(500);
  }
  return page;
}

// shot captures `focus` when given, and the whole page otherwise.
//
// It used to scroll the focused element into view and then screenshot the
// page, which meant every "card" shot was really a full page: two shots of the
// same route came out byte-identical, and the subject was a small part of a
// tall image. Clipping to the element is the whole point of naming one.
async function shot(page, name, focus) {
  await page.waitForTimeout(1200);
  if (focus) {
    const target = page.locator(focus).first();
    if (await target.count()) {
      await target.scrollIntoViewIfNeeded();
      await page.waitForTimeout(400);
      await target.screenshot({ path: `${OUT}/${name}.png` });
      console.log(`captured ${name}.png (clipped to ${focus})`);
      return;
    }
    throw new Error(`shot "${name}": focus selector ${focus} matched nothing`);
  }
  await page.screenshot({ path: `${OUT}/${name}.png` });
  console.log(`captured ${name}.png`);
}

// The card itself, not a bottom anchor: shots clip to what they name.
const TEAM_ACCESS_CARD = '[data-testid="workspace-team-access-card"]';
const browser = await chromium.launch();
try {
  const desktop = { width: 1440, height: 1250 };

  const ana = await signIn(await browser.newContext({ viewport: desktop }), "ana@example.com");
  const { workspaces } = await (await ana.request.get(`${BASE}/api/v1/workspaces`)).json();
  const team = workspaces.find((w) => w.name === "Platform Team");

  // The tree replaced per-workspace visibility, so the first two shots are the
  // structure and who is in it.
  await ana.goto(`${BASE}/settings/units`, { waitUntil: "networkidle" });
  await ana.getByTestId("unit-row").first().waitFor({ state: "visible", timeout: 20000 });
  await shot(ana, "team-access-units-tree");

  const platformRow = ana.getByTestId("unit-row").filter({ hasText: "Platform" }).first();
  await platformRow.getByTestId("unit-members").click();
  await ana.getByTestId("unit-members-dialog").waitFor({ state: "visible", timeout: 20000 });
  await ana.waitForTimeout(600);
  await shot(ana, "team-access-unit-members", '[data-testid="unit-members-dialog"]');
  await ana.keyboard.press("Escape");

  // Creating a unit, and the placement control that decides who reaches a
  // workspace. Placement is the only way to narrow access, so it earns a shot.
  await ana.getByTestId("unit-add-child").first().click();
  await ana.getByTestId("unit-name-input").waitFor({ state: "visible", timeout: 20000 });
  await ana.getByTestId("unit-name-input").fill("Security");
  await ana.waitForTimeout(400);
  await shot(ana, "team-access-unit-create");
  await ana.getByRole("button", { name: /cancel/i }).click();

  await ana.goto(`${BASE}/settings/workspace/${team.id}`, { waitUntil: "networkidle" });
  await ana.getByTestId("workspace-placement-card").waitFor({ state: "visible", timeout: 20000 });
  await shot(ana, "team-access-workspace-placement", '[data-testid="workspace-placement-card"]');

  await ana.goto(`${BASE}/settings/workspace/${team.id}`, { waitUntil: "networkidle" });
  await shot(ana, "team-access-owner-card", TEAM_ACCESS_CARD);

  await ana.goto(`${BASE}/settings/system/users`, { waitUntil: "networkidle" });
  await ana.getByTestId("users-table-card").waitFor({ state: "visible", timeout: 20000 });
  await shot(ana, "team-access-user-roles");

  const bruno = await signIn(await browser.newContext({ viewport: desktop }), "bruno@example.com");
  await bruno
    .context()
    .addCookies([{ name: "kandev-active-workspace", value: team.id, url: BASE }]);
  await bruno.goto(`${BASE}/`, { waitUntil: "networkidle" });
  await shot(bruno, "team-access-shared-board");

  // The human assignee, on both boards. Ana assigns Bruno through the API so
  // the shots show a task somebody else owns, which is what the "Assign to me"
  // affordance is for.
  const tasks = await (await ana.request.get(`${BASE}/api/v1/workspaces/${team.id}/tasks`)).json();
  const task = (tasks.tasks ?? tasks)[0];
  const { users } = await (await ana.request.get(`${BASE}/api/v1/users/directory`)).json();
  const brunoId = users.find((u) => u.display_name.includes("Bruno"))?.id;
  if (task && brunoId) {
    await ana.request.patch(`${BASE}/api/v1/tasks/${task.id}`, {
      data: { assignee_user_id: brunoId },
    });
    await ana.goto(`${BASE}/t/${task.id}`, { waitUntil: "networkidle" });
    await ana.getByTestId("task-assignee-control").waitFor({ state: "visible", timeout: 25000 });
    await ana.waitForTimeout(800);
    await ana.screenshot({
      path: `${OUT}/team-access-kanban-assignee-topbar.png`,
      clip: { x: 540, y: 0, width: 900, height: 56 },
    });
    console.log("captured team-access-kanban-assignee-topbar.png");

    // The board shot needs the shared workspace selected; the root route
    // otherwise opens whichever workspace the cookie last named.
    await ana
      .context()
      .addCookies([{ name: "kandev-active-workspace", value: team.id, url: BASE }]);
    await ana.goto(`${BASE}/`, { waitUntil: "networkidle" });
    const badge = ana.getByTestId("kanban-card-assignee").first();
    await badge.waitFor({ state: "visible", timeout: 25000 });
    const card = badge.locator("xpath=ancestor::*[@data-testid][1]");
    await ((await card.count()) ? card : badge).screenshot({
      path: `${OUT}/team-access-kanban-assignee-card.png`,
    });
    console.log("captured team-access-kanban-assignee-card.png");
  }

  const carla = await signIn(await browser.newContext({ viewport: desktop }), "carla@example.com");
  await carla.goto(`${BASE}/settings/workspace/${team.id}`, { waitUntil: "networkidle" });
  await shot(carla, "team-access-viewer-card", TEAM_ACCESS_CARD);

  const mobile = await signIn(
    await browser.newContext({
      viewport: { width: 390, height: 844 },
      isMobile: true,
      hasTouch: true,
      deviceScaleFactor: 2,
    }),
    "ana@example.com",
  );
  await mobile.goto(`${BASE}/settings/workspace/${team.id}`, { waitUntil: "networkidle" });
  await shot(mobile, "team-access-mobile", TEAM_ACCESS_CARD);

  console.log(`\nwritten to ${OUT}`);
} finally {
  await browser.close();
}
