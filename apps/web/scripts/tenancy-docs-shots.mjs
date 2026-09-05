// Capture the multi-tenancy screenshots from the seeded demo instance.
//   cd apps/web && node scripts/tenancy-docs-shots.mjs
import { chromium } from "@playwright/test";
import { mkdirSync } from "node:fs";

const BASE = process.env.KANDEV_DEMO_URL ?? "http://127.0.0.1:8232";
const OUT = process.env.SHOT_DIR ?? "../../docs/screenshots";
const PASSWORD = "kandev-demo-pw-1";
// The operator's email differs by seed: the tenancy seed uses acme.test, the
// team-access seed used for the Organizations shot uses example.com.
const OPERATOR = process.env.KANDEV_DEMO_ADMIN ?? "ana@acme.test";
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

async function shot(page, name) {
  await page.waitForTimeout(1200);
  await page.screenshot({ path: `${OUT}/${name}.png` });
  console.log(`captured ${name}.png`);
}

const desktop = { width: 1440, height: 1100 };
const browser = await chromium.launch();
try {
  // Operator: the organization list.
  const ana = await signIn(await browser.newContext({ viewport: desktop }), OPERATOR);
  await ana.goto(`${BASE}/settings/system/organizations`, { waitUntil: "networkidle" });
  await shot(ana, "tenancy-organizations");

  // The type-to-confirm delete dialog.
  // The default organization's delete button is disabled on purpose, so pick
  // a row that can actually be deleted.
  const deleteButtons = ana.getByRole("button", { name: /delete organization/i });
  const count = await deleteButtons.count();
  let target = null;
  for (let i = 0; i < count; i += 1) {
    if (await deleteButtons.nth(i).isEnabled()) {
      target = deleteButtons.nth(i);
      break;
    }
  }
  if (target) {
    await target.click();
    await ana.waitForTimeout(700);
    await shot(ana, "tenancy-delete-confirm");
    await ana.keyboard.press("Escape");
    await ana.waitForTimeout(400);
  }

  // The first-administrator dialog.
  const addAdmin = ana.getByRole("button", { name: /add administrator/i });
  if (await addAdmin.count()) {
    await addAdmin.first().click();
    await ana.waitForTimeout(700);
    await shot(ana, "tenancy-first-admin");
    await ana.keyboard.press("Escape");
  }

  // Acme's shared board.
  const bruno = await signIn(await browser.newContext({ viewport: desktop }), "bruno@acme.test");
  await bruno.goto(`${BASE}/`, { waitUntil: "networkidle" });
  await shot(bruno, "tenancy-acme-board");

  // Globex sees only its own world, on the same server.
  const gary = await signIn(await browser.newContext({ viewport: desktop }), "gary@globex.test");
  await gary.goto(`${BASE}/`, { waitUntil: "networkidle" });
  await shot(gary, "tenancy-globex-board");

  // A suspended organization refuses login.
  const ivan = await browser.newContext({ viewport: desktop });
  const page = await ivan.newPage();
  await page.goto(`${BASE}/login`, { waitUntil: "domcontentloaded" });
  await page.getByLabel(/email/i).fill("ivan@initech.test");
  await page.getByLabel(/password/i).fill(PASSWORD);
  await page.getByRole("button", { name: /sign in|log in/i }).click();
  await page.waitForTimeout(1500);
  await shot(page, "tenancy-suspended-login");

  console.log(`\nwritten to ${OUT}`);
} finally {
  await browser.close();
}
