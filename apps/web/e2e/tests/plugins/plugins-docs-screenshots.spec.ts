/**
 * Docs media generator — NOT a CI assertion. Skipped unless
 * CAPTURE_DOCS_MEDIA=1 is set, so it never runs in the normal e2e shards.
 *
 * When enabled, it installs a real gRPC plugin into the worker's isolated
 * backend (same harness as tests/plugins/plugins.spec.ts) and screenshots
 * the operator-facing surfaces straight into docs/screenshots/plugin-*.png,
 * which the public plugin docs embed as ../screenshots/plugin-*.png (the
 * landing publisher only rewrites/copies images under screenshots/; media/
 * is reserved for <DocsVideo> assets).
 *
 * Regenerate (from apps/web), pointing at the polished kandev-plugin-hello
 * example so the captures match the docs prose (display name "Hello Plugin",
 * "Hello World" nav item, /hello-world route):
 *
 *   CAPTURE_DOCS_MEDIA=1 \
 *   DOCS_PLUGIN_PACKAGE=/abs/path/to/kandev-plugin-hello-1.5.0.tar.gz \
 *   DOCS_PLUGIN_ID=kandev-plugin-hello \
 *   DOCS_PLUGIN_NAV_ID=hello-world \
 *   DOCS_PLUGIN_ROUTE=/hello-world \
 *   pnpm e2e --project=chromium --workers=1 tests/plugins/plugins-docs-screenshots.spec.ts
 *
 * With no DOCS_PLUGIN_* overrides it falls back to the repo's own e2e fixture
 * package (apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz), so the spec is
 * always self-runnable even without the external example checked out.
 */
import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";

const CAPTURE = process.env.CAPTURE_DOCS_MEDIA === "1";

const PACKAGE_PATH =
  process.env.DOCS_PLUGIN_PACKAGE ??
  path.resolve(__dirname, "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz");
const PLUGIN_ID = process.env.DOCS_PLUGIN_ID ?? "kandev-plugin-e2e";
const NAV_ITEM_ID = process.env.DOCS_PLUGIN_NAV_ID ?? "e2e-hello";
const PLUGIN_ROUTE = process.env.DOCS_PLUGIN_ROUTE ?? "/plugins/e2e-hello";

const SCREENSHOTS_DIR = path.resolve(__dirname, "../../../../../docs/screenshots");
const VIEWPORT = { width: 1280, height: 860 };

// Hand-authored "How it works" architecture diagram, rendered to
// docs/screenshots/plugin-architecture.png (plugins.md embeds it). Kept as
// HTML/CSS rather than mermaid so it reads as a designed figure.
const ARCH_HTML = `<!doctype html><html><head><meta charset="utf-8" /><style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { background: #fff; padding: 30px; }
#diagram { width: 940px; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Inter, "Helvetica Neue", Arial, sans-serif; color: #0f172a; background: #fff; }
.phase { margin-bottom: 26px; } .phase:last-child { margin-bottom: 0; }
.phase-label { font-size: 12px; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; color: #6366f1; margin-bottom: 4px; }
.phase-sub { font-size: 12.5px; color: #64748b; margin-bottom: 14px; }
.pipeline { display: flex; align-items: stretch; gap: 6px; }
.step { flex: 1; background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 12px; padding: 12px 14px; display: flex; gap: 10px; align-items: flex-start; box-shadow: 0 1px 2px rgba(15,23,42,.04); }
.step .ico { width: 26px; height: 26px; flex: none; border-radius: 8px; display: grid; place-items: center; background: #eef2ff; color: #6366f1; font-size: 14px; font-weight: 700; }
.step-title { font-size: 13.5px; font-weight: 650; line-height: 1.25; }
.step-sub { font-size: 11.5px; color: #64748b; margin-top: 2px; line-height: 1.3; }
.chev { align-self: center; color: #cbd5e1; font-size: 18px; font-weight: 700; }
.supervise-note { margin-top: 12px; font-size: 12.5px; color: #475569; line-height: 1.45; padding: 10px 14px; background: #fafafa; border: 1px dashed #e2e8f0; border-radius: 10px; }
.supervise-note b { color: #0f172a; font-weight: 650; }
.flows { display: flex; flex-direction: column; gap: 10px; }
.flow { border: 1px solid #e5e7eb; border-radius: 12px; padding: 12px 16px; background: #fff; box-shadow: 0 1px 2px rgba(15,23,42,.04); }
.flow.optional { border-style: dashed; background: #fcfcfd; }
.actors { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.chip { font-size: 12px; font-weight: 650; padding: 3px 10px; border-radius: 999px; border: 1px solid; white-space: nowrap; }
.chip.k { background: #eef2ff; color: #4338ca; border-color: #c7d2fe; }
.chip.p { background: #ecfdf5; color: #047857; border-color: #a7f3d0; }
.chip.e { background: #fffbeb; color: #b45309; border-color: #fde68a; }
.chip.b { background: #f1f5f9; color: #334155; border-color: #cbd5e1; }
.arrow { display: inline-flex; align-items: center; }
.arrow .line { width: 22px; height: 0; border-top: 2px solid #cbd5e1; position: relative; }
.arrow .line::after { content: ""; position: absolute; right: -1px; top: -4px; border-left: 6px solid #cbd5e1; border-top: 4px solid transparent; border-bottom: 4px solid transparent; }
.method { font-size: 12.5px; font-weight: 700; color: #6366f1; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.method.plain { color: #475569; }
.desc { font-size: 12.5px; color: #475569; margin-top: 7px; line-height: 1.45; }
.desc code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11.5px; background: #f1f5f9; padding: 1px 5px; border-radius: 5px; color: #334155; }
.opt-tag { font-size: 10.5px; font-weight: 700; text-transform: uppercase; letter-spacing: .05em; color: #94a3b8; margin-left: 6px; }
</style></head><body><div id="diagram">
<section class="phase"><div class="phase-label">1 · Install &amp; supervise</div>
<div class="pipeline">
<div class="step"><div class="ico">↓</div><div><div class="step-title">Install</div><div class="step-sub">URL · upload · filesystem sync</div></div></div>
<div class="chev">›</div>
<div class="step"><div class="ico">✓</div><div><div class="step-title">Verify</div><div class="step-sub">checksums.txt · validate manifest.yaml</div></div></div>
<div class="chev">›</div>
<div class="step"><div class="ico">◲</div><div><div class="step-title">Extract</div><div class="step-sub">~/.kandev/plugins/&lt;id&gt;/&lt;version&gt;/</div></div></div>
<div class="chev">›</div>
<div class="step"><div class="ico">⚙</div><div><div class="step-title">Spawn</div><div class="step-sub">go-plugin gRPC subprocess</div></div></div>
</div>
<div class="supervise-note">kandev owns the lifecycle: completes the handshake, health-checks with <b>Ping</b> every 30s, and restarts on crash or repeated failure (backoff, max 5 attempts).</div>
</section>
<section class="phase"><div class="phase-label">2 · While the plugin runs</div>
<div class="phase-sub">One supervised gRPC connection — unix socket (loopback + AutoMTLS on Windows). The plugin serves no HTTP itself.</div>
<div class="flows">
<div class="flow"><div class="actors"><span class="chip k">kandev</span><span class="arrow"><span class="line"></span></span><span class="method">DeliverEvent</span><span class="arrow"><span class="line"></span></span><span class="chip p">plugin</span></div><div class="desc">Bus events delivered at-least-once, buffered while the plugin is unhealthy.</div></div>
<div class="flow"><div class="actors"><span class="chip e">external caller</span><span class="arrow"><span class="line"></span></span><span class="method plain">HTTP POST/GET</span><span class="arrow"><span class="line"></span></span><span class="chip k">kandev</span><span class="arrow"><span class="line"></span></span><span class="method">HandleWebhook</span><span class="arrow"><span class="line"></span></span><span class="chip p">plugin</span></div><div class="desc">kandev's <code>/api/plugins/{id}/webhooks/{key}</code> route relays the request to the plugin over gRPC.</div></div>
<div class="flow"><div class="actors"><span class="chip p">plugin</span><span class="arrow"><span class="line"></span></span><span class="method">Host API</span><span class="arrow"><span class="line"></span></span><span class="chip k">kandev</span></div><div class="desc">Calls back on the same connection: state / config / secrets, <code>EmitEvent</code>, and capability-gated read-only data (tasks, sessions, workspaces, …).</div></div>
<div class="flow optional"><div class="actors"><span class="chip p">plugin</span><span class="arrow"><span class="line"></span></span><span class="method plain">ui.bundle</span><span class="arrow"><span class="line"></span></span><span class="chip b">browser SPA</span><span class="opt-tag">optional</span></div><div class="desc">The SPA loads the native bundle at boot to register real routes, nav items, slot components, and WS handlers.</div></div>
</div></section>
</div></body></html>`;

/** Let transient success toasts auto-dismiss so they don't sit in captures. */
async function waitForToastsGone(page: Page): Promise<void> {
  await page
    .locator("[data-sonner-toast]")
    .first()
    .waitFor({ state: "detached", timeout: 8_000 })
    .catch(() => undefined);
}

async function shot(page: Page, name: string): Promise<void> {
  fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true });
  await waitForToastsGone(page);
  await page.screenshot({ path: path.join(SCREENSHOTS_DIR, `plugin-${name}.png`) });
}

test.describe("Plugin docs screenshots", () => {
  test.skip(!CAPTURE, "docs media generator — set CAPTURE_DOCS_MEDIA=1 to run");

  test("captures the operator-facing plugin surfaces", async ({ testPage, apiClient }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize(VIEWPORT);

    // --- Install dialog (upload tab), captured before we actually upload ---
    await testPage.goto("/settings/plugins");
    await testPage.getByTestId("install-plugin-trigger").click();
    await expect(testPage.getByTestId("install-plugin-dialog")).toBeVisible();
    await testPage.getByTestId("install-plugin-tab-upload").click();
    await shot(testPage, "install-dialog");

    // --- Install the package, land on the list with an Active row ---
    await testPage.getByTestId("install-plugin-file-input").setInputFiles(PACKAGE_PATH);
    await testPage.getByTestId("install-plugin-upload-submit").click();
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
    await expect(testPage.getByTestId("install-plugin-dialog")).toBeHidden();
    await shot(testPage, "settings-list");

    // --- Per-plugin settings page: config_schema form + manifest card ---
    await testPage.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();
    await expect(testPage).toHaveURL(new RegExp(`/settings/plugins/${PLUGIN_ID}$`));
    await expect(testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`)).toBeVisible();
    await expect(testPage.getByTestId("plugin-manifest-card")).toBeVisible();

    // Capture the pristine first-open form (schema-driven fields at their
    // defaults + the manifest card) — no edits, so no unsaved-changes banner.
    await shot(testPage, "settings-page");

    // --- The plugin's own native route + its sidebar nav item ---
    await testPage.goto("/");
    await testPage.reload();
    const navItem = testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`);
    await expect(navItem).toBeVisible({ timeout: 15_000 });
    await navItem.click();
    await expect(testPage).toHaveURL(new RegExp(`${PLUGIN_ROUTE}$`));
    await shot(testPage, "native-page");

    // Clean up so a repeat run reinstalls from a clean slate.
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("renders the how-it-works architecture diagram", async ({ browser }) => {
    const ctx = await browser.newContext({ deviceScaleFactor: 2 });
    const page = await ctx.newPage();
    await page.setContent(ARCH_HTML, { waitUntil: "networkidle" });
    fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true });
    await page
      .locator("#diagram")
      .screenshot({ path: path.join(SCREENSHOTS_DIR, "plugin-architecture.png") });
    await ctx.close();
  });
});
