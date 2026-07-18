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

// Hand-authored "How it works" architecture diagram (SVG), rendered to
// docs/screenshots/plugin-architecture.png (plugins.md embeds it). A designed
// figure rather than mermaid so the boxes/arrows read as a real diagram.
const ARCH_HTML = `<!doctype html><html><head><meta charset="utf-8" /><style>
* { margin:0; padding:0; box-sizing:border-box; }
body { background:#fff; padding:28px; }
svg { font-family:-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Inter, Arial, sans-serif; }
.box { fill:#fff; stroke:#e2e8f0; stroke-width:1.5; }
.box-soft { fill:#f8fafc; stroke:#e2e8f0; stroke-width:1.5; }
.lane { fill:#f8fafc; stroke:#e2e8f0; stroke-width:1.5; }
.t-title { font-size:14px; font-weight:700; }
.t-step { font-size:13px; font-weight:650; fill:#0f172a; }
.t-sub { font-size:11px; fill:#64748b; }
.t-body { font-size:11.5px; fill:#475569; }
.badge { fill:#6366f1; } .badge-t { fill:#fff; font-size:11px; font-weight:700; }
.edge { stroke:#94a3b8; stroke-width:1.6; fill:none; }
.edge-dotted { stroke:#94a3b8; stroke-width:1.6; fill:none; stroke-dasharray:4 4; }
.lbl { font-size:11.5px; font-weight:700; fill:#4f46e5; font-family:ui-monospace, SFMono-Regular, Menlo, monospace; paint-order:stroke; stroke:#fff; stroke-width:5px; stroke-linejoin:round; }
.lbl-p { font-size:11px; fill:#475569; paint-order:stroke; stroke:#fff; stroke-width:5px; stroke-linejoin:round; }
.lbl-mut { font-size:10.5px; fill:#94a3b8; paint-order:stroke; stroke:#fff; stroke-width:4px; stroke-linejoin:round; }
.chip-p { fill:#ecfdf5; stroke:#a7f3d0; } .chip-p-t { font-size:11px; font-weight:650; fill:#047857; }
</style></head><body>
<svg id="diagram" width="960" height="500" viewBox="0 0 960 500">
<defs><marker id="ah" markerWidth="9" markerHeight="9" refX="6.5" refY="3" orient="auto"><path d="M0,0 L6.5,3 L0,6 z" fill="#94a3b8"/></marker></defs>
<text x="20" y="14" class="t-sub" style="font-weight:700;fill:#6366f1;letter-spacing:.06em">INSTALL &amp; LOAD</text>
<rect x="20" y="24" width="204" height="60" rx="12" class="box-soft"/><circle cx="44" cy="54" r="11" class="badge"/><text x="44" y="58" text-anchor="middle" class="badge-t">1</text><text x="64" y="49" class="t-step">Install</text><text x="64" y="66" class="t-sub">URL · upload · sync</text>
<rect x="256" y="24" width="204" height="60" rx="12" class="box-soft"/><circle cx="280" cy="54" r="11" class="badge"/><text x="280" y="58" text-anchor="middle" class="badge-t">2</text><text x="300" y="49" class="t-step">Verify</text><text x="300" y="66" class="t-sub">checksums · manifest.yaml</text>
<rect x="492" y="24" width="204" height="60" rx="12" class="box-soft"/><circle cx="516" cy="54" r="11" class="badge"/><text x="516" y="58" text-anchor="middle" class="badge-t">3</text><text x="536" y="49" class="t-step">Extract</text><text x="536" y="66" class="t-sub">~/.kandev/plugins/&lt;id&gt;/</text>
<rect x="728" y="24" width="204" height="60" rx="12" class="box-soft"/><circle cx="752" cy="54" r="11" class="badge"/><text x="752" y="58" text-anchor="middle" class="badge-t">4</text><text x="772" y="49" class="t-step">Spawn</text><text x="772" y="66" class="t-sub">go-plugin subprocess</text>
<line x1="226" y1="54" x2="252" y2="54" class="edge" marker-end="url(#ah)"/><line x1="462" y1="54" x2="488" y2="54" class="edge" marker-end="url(#ah)"/><line x1="698" y1="54" x2="724" y2="54" class="edge" marker-end="url(#ah)"/>
<path d="M830,84 C830,150 726,150 726,208" class="edge-dotted" marker-end="url(#ah)"/><text x="812" y="150" class="lbl-mut">spawned &amp; supervised</text>
<rect x="70" y="130" width="250" height="42" rx="10" class="box" fill="#fffbeb" stroke="#fde68a"/><text x="195" y="156" text-anchor="middle" style="font-size:12px;font-weight:650;fill:#b45309">External caller</text>
<path d="M195,172 L195,208" class="edge" marker-end="url(#ah)"/><text x="212" y="196" class="lbl-p">HTTP POST/GET&#8195;/api/plugins/{id}/webhooks/{key}</text>
<rect x="70" y="210" width="330" height="182" rx="14" class="lane"/><text x="90" y="240" class="t-title" fill="#4338ca">kandev backend</text><text x="90" y="264" class="t-body">event bus · webhook route</text><text x="90" y="284" class="t-body">Host API · secrets · supervisor</text>
<rect x="560" y="210" width="330" height="182" rx="14" class="lane"/><text x="580" y="240" class="t-title" fill="#047857">plugin subprocess</text><text x="580" y="264" class="t-body">OnEvent · HandleWebhook</text><text x="580" y="284" class="t-body">Host calls back over gRPC</text>
<rect x="580" y="344" width="228" height="28" rx="8" class="chip-p"/><text x="594" y="362" class="chip-p-t">native UI bundle · optional</text>
<text x="480" y="228" text-anchor="middle" class="lbl-mut">gRPC · AutoMTLS</text>
<line x1="404" y1="256" x2="556" y2="256" class="edge" marker-end="url(#ah)"/><text x="480" y="249" text-anchor="middle" class="lbl">DeliverEvent</text>
<line x1="404" y1="300" x2="556" y2="300" class="edge" marker-end="url(#ah)"/><text x="480" y="293" text-anchor="middle" class="lbl">HandleWebhook</text>
<line x1="556" y1="348" x2="404" y2="348" class="edge" marker-end="url(#ah)"/><text x="480" y="341" text-anchor="middle" class="lbl">Host API</text>
<rect x="560" y="432" width="250" height="42" rx="10" class="box"/><text x="685" y="458" text-anchor="middle" style="font-size:12px;font-weight:650;fill:#334155">Browser SPA</text>
<path d="M690,392 L690,430" class="edge" marker-end="url(#ah)"/><text x="704" y="416" class="lbl-p">loads ui.bundle at boot</text>
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
    await page
      .locator("#diagram")
      .screenshot({ path: path.join(SCREENSHOTS_DIR, "plugin-architecture.png") });
    await ctx.close();
  });
});
