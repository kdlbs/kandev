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

// Hand-authored "How it works" flowchart (SVG), rendered to
// docs/screenshots/plugin-architecture.png (plugins.md embeds it). A clean
// dark top-down flowchart (mermaid's shape, drawn properly) that matches the
// docs page; the render uses omitBackground so the rounded card is transparent.
const ARCH_HTML = `<!doctype html><html><head><meta charset="utf-8" /><style>
* { margin:0; padding:0; box-sizing:border-box; }
body { background:transparent; }
svg { font-family:-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Inter, Arial, sans-serif; }
.card { fill:#0f172a; stroke:#1e293b; stroke-width:1; }
.node { fill:#1e293b; stroke:#334155; stroke-width:1.4; }
.node-dash { fill:#1e293b; stroke:#334155; stroke-width:1.4; stroke-dasharray:4 4; }
.title { font-size:13px; font-weight:650; fill:#e2e8f0; }
.sub { font-size:11px; fill:#94a3b8; }
.edge { stroke:#64748b; stroke-width:1.4; fill:none; }
.edge-dash { stroke:#64748b; stroke-width:1.4; fill:none; stroke-dasharray:4 4; }
.lbl { font-size:11px; fill:#cbd5e1; font-family:ui-monospace, SFMono-Regular, Menlo, monospace; paint-order:stroke; stroke:#0f172a; stroke-width:5px; stroke-linejoin:round; }
.opt { font-size:10px; fill:#64748b; paint-order:stroke; stroke:#0f172a; stroke-width:4px; stroke-linejoin:round; }
</style></head><body>
<svg id="diagram" width="760" height="694" viewBox="0 0 760 694">
<defs><marker id="ah" markerWidth="9" markerHeight="9" refX="6" refY="3" orient="auto"><path d="M0,0 L6,3 L0,6 z" fill="#64748b"/></marker></defs>
<rect x="0.5" y="0.5" width="759" height="693" rx="16" class="card"/>
<rect x="250" y="20" width="260" height="56" rx="10" class="node"/><text x="380" y="44" text-anchor="middle" class="title">Install</text><text x="380" y="62" text-anchor="middle" class="sub">URL · upload · filesystem sync</text>
<line x1="380" y1="76" x2="380" y2="114" class="edge" marker-end="url(#ah)"/>
<rect x="250" y="116" width="260" height="56" rx="10" class="node"/><text x="380" y="140" text-anchor="middle" class="title">Verify</text><text x="380" y="158" text-anchor="middle" class="sub">checksums.txt · validate manifest.yaml</text>
<line x1="380" y1="172" x2="380" y2="210" class="edge" marker-end="url(#ah)"/>
<rect x="250" y="212" width="260" height="56" rx="10" class="node"/><text x="380" y="236" text-anchor="middle" class="title">Extract</text><text x="380" y="254" text-anchor="middle" class="sub">~/.kandev/plugins/&lt;id&gt;/&lt;version&gt;/</text>
<line x1="380" y1="268" x2="380" y2="306" class="edge" marker-end="url(#ah)"/>
<rect x="250" y="308" width="260" height="56" rx="10" class="node"/><text x="380" y="332" text-anchor="middle" class="title">Spawn subprocess</text><text x="380" y="350" text-anchor="middle" class="sub">go-plugin · gRPC over unix socket · AutoMTLS</text>
<path d="M380,364 V388 H214 V406" class="edge" marker-end="url(#ah)"/><path d="M380,364 V388 H546 V406" class="edge" marker-end="url(#ah)"/>
<text x="297" y="382" text-anchor="middle" class="lbl">DeliverEvent</text><text x="463" y="382" text-anchor="middle" class="lbl">HandleWebhook</text>
<rect x="64" y="410" width="300" height="64" rx="10" class="node"/><text x="214" y="436" text-anchor="middle" class="title">kandev delivers bus events</text><text x="214" y="455" text-anchor="middle" class="sub">at-least-once · buffered while unhealthy</text>
<rect x="396" y="410" width="300" height="64" rx="10" class="node"/><text x="546" y="436" text-anchor="middle" class="title">kandev relays a webhook</text><text x="546" y="455" text-anchor="middle" class="sub">POST/GET /api/plugins/{id}/webhooks/{key}</text>
<path d="M214,474 V496 H300 V514" class="edge" marker-end="url(#ah)"/><path d="M546,474 V496 H460 V514" class="edge" marker-end="url(#ah)"/>
<rect x="170" y="516" width="420" height="64" rx="10" class="node"/><text x="380" y="542" text-anchor="middle" class="title">Plugin calls back — Host API</text><text x="380" y="561" text-anchor="middle" class="sub">state · config · secrets · EmitEvent · read-only data</text>
<line x1="380" y1="580" x2="380" y2="616" class="edge-dash" marker-end="url(#ah)"/><text x="398" y="601" class="opt">optional</text>
<rect x="210" y="618" width="340" height="56" rx="10" class="node-dash"/><text x="380" y="642" text-anchor="middle" class="title">SPA loads the native UI bundle</text><text x="380" y="660" text-anchor="middle" class="sub">routes · nav · slots · WS handlers</text>
</svg></body></html>`;

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
    await page.locator("#diagram").screenshot({
      path: path.join(SCREENSHOTS_DIR, "plugin-architecture.png"),
      omitBackground: true,
    });
    await ctx.close();
  });
});
