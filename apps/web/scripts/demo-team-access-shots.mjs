// Capture screenshots of the team-access surfaces from the seeded demo
// instance. Run after scripts/demo-team-access.sh up:
//
//   cd apps/web && node scripts/demo-team-access-shots.mjs
//
// It lives under apps/web because Node resolves imports relative to the
// script file, and Playwright is installed in this workspace.
import { chromium } from "@playwright/test";
import { mkdirSync } from "node:fs";

const BASE = process.env.KANDEV_DEMO_URL ?? "http://127.0.0.1:8231";
const OUT = process.env.SHOT_DIR ?? "/tmp/kandev-team-access-demo/shots";
const PASSWORD = "kandev-demo-pw-1";

mkdirSync(OUT, { recursive: true });

async function signIn(context, email) {
  const page = await context.newPage();
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
  await dismissOnboarding(page);
  return page;
}

// Each account hits the first-run wizard on its first sign-in, which would
// otherwise sit on top of every board screenshot.
async function dismissOnboarding(page) {
  for (let step = 0; step < 6; step += 1) {
    const skip = page.getByRole("button", { name: /^skip$/i });
    if (!(await skip.count())) return;
    await skip
      .first()
      .click()
      .catch(() => {});
    await page.waitForTimeout(500);
  }
}

async function shot(page, name, focus) {
  await page.waitForTimeout(1200); // let the SPA settle before capture
  if (focus) {
    const target = page.locator(focus).first();
    if (await target.count()) {
      await target.scrollIntoViewIfNeeded();
      await page.waitForTimeout(400);
    }
  }
  await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: false });
  console.log(`captured ${name}.png`);
}

// The Team access card sits below the workspace form, so scroll it into frame.
// Scroll the *bottom* of the card into frame: the heading alone is already
// on screen, so targeting it scrolls nothing and the members list stays cut off.
const TEAM_CARD = '#add-member-user, [data-testid="workspace-member-row"]:last-of-type';

const browser = await chromium.launch();
try {
  // 1. Owner: the Team access card on the shared workspace.
  const anaCtx = await browser.newContext({ viewport: { width: 1440, height: 1250 } });
  const ana = await signIn(anaCtx, "ana@example.com");

  const listResponse = await ana.request.get(`${BASE}/api/v1/workspaces`);
  const { workspaces } = await listResponse.json();
  const team = workspaces.find((w) => w.name === "Platform Team");
  const priv = workspaces.find((w) => w.name.startsWith("Ana - security"));

  await ana.goto(`${BASE}/settings/workspace/${team.id}`, { waitUntil: "networkidle" });
  await shot(ana, "01-owner-team-access-shared", TEAM_CARD);

  await ana.goto(`${BASE}/settings/workspace/${priv.id}`, { waitUntil: "networkidle" });
  await shot(ana, "02-owner-team-access-private", TEAM_CARD);

  await ana.goto(`${BASE}/`, { waitUntil: "networkidle" });
  await shot(ana, "03-owner-board");

  // 2. Member with no invitation: the shared board is simply there.
  const brunoCtx = await browser.newContext({ viewport: { width: 1440, height: 1250 } });
  const bruno = await signIn(brunoCtx, "bruno@example.com");
  await bruno.goto(`${BASE}/`, { waitUntil: "networkidle" });
  await shot(bruno, "04-member-sees-shared-board");
  await bruno.goto(`${BASE}/settings/workspace/${team.id}`, { waitUntil: "networkidle" });
  await shot(bruno, "05-member-team-access-readonly", TEAM_CARD);

  // 3. Viewer: reads the same workspace, holds one scope.
  const carlaCtx = await browser.newContext({ viewport: { width: 1440, height: 1250 } });
  const carla = await signIn(carlaCtx, "carla@example.com");
  await carla.goto(`${BASE}/settings/workspace/${team.id}`, { waitUntil: "networkidle" });
  await shot(carla, "06-viewer-team-access-readonly", TEAM_CARD);

  // 4. Guest: the shared workspace does not exist for her.
  const danaCtx = await browser.newContext({ viewport: { width: 1440, height: 1250 } });
  const dana = await signIn(danaCtx, "dana@example.com");
  await dana.goto(`${BASE}/`, { waitUntil: "networkidle" });
  await shot(dana, "07-guest-sees-only-invited-workspace");

  // 5. Mobile composition of the members surface.
  const mobileCtx = await browser.newContext({
    viewport: { width: 390, height: 844 },
    isMobile: true,
    hasTouch: true,
    deviceScaleFactor: 2,
  });
  const mobile = await signIn(mobileCtx, "ana@example.com");
  await mobile.goto(`${BASE}/settings/workspace/${team.id}`, { waitUntil: "networkidle" });
  await shot(mobile, "08-mobile-team-access", TEAM_CARD);

  console.log(`\nscreenshots written to ${OUT}`);
} finally {
  await browser.close();
}
