import fs from "node:fs";
import path from "node:path";

const BACKEND_DIR = path.resolve(__dirname, "../../../apps/backend");
const WEB_DIR = path.resolve(__dirname, "..");

export default function globalSetup() {
  const kandevBin = path.join(BACKEND_DIR, "bin", "kandev");
  const mockAgentBin = path.join(BACKEND_DIR, "bin", "mock-agent");

  for (const bin of [kandevBin, mockAgentBin]) {
    if (!fs.existsSync(bin)) {
      throw new Error(`Required binary not found: ${bin}\nRun "make build-backend" first.`);
    }
  }

  const spaIndex = path.join(WEB_DIR, "dist", "index.html");
  if (!fs.existsSync(spaIndex)) {
    throw new Error(`Vite web build not found: ${spaIndex}\nRun "make build-web" first.`);
  }

  assertPseudoCatalogBundled();

  // tests/plugins/plugins.spec.ts installs this package through the real
  // upload UI. Like the binaries above, this only checks existence — not
  // freshness — so rebuild after touching cmd/plugin-fixture (see
  // apps/backend/Makefile's e2e-plugin-package target).
  const pluginPackage = path.join(BACKEND_DIR, ".build", "kandev-plugin-e2e-1.0.0.tar.gz");
  if (!fs.existsSync(pluginPackage)) {
    throw new Error(
      `E2E fixture plugin package not found: ${pluginPackage}\nRun "make -C apps/backend e2e-plugin-package" first.`,
    );
  }
}

/**
 * Fail fast when the coverage oracle is asked to run against a bundle that has
 * no pseudo catalog.
 *
 * A production `vite build` deliberately drops it (see lib/i18n/bundling.ts).
 * Run tests/i18n/pseudo-coverage.spec.ts against such a bundle and every screen
 * falls back to `en` — so all ten tests fail with a wall of findings that reads
 * exactly like a mass externalization regression, when the real cause is that
 * the artifact was built with `make build-web` instead of `make build-web-e2e`.
 * Only checked under KANDEV_I18N_COVERAGE=1; nothing else needs the catalog.
 *
 * The markers are read from the catalog itself rather than hardcoded, so this
 * cannot drift as `pnpm run i18n:pseudo` regenerates it.
 */
function assertPseudoCatalogBundled() {
  if (process.env.KANDEV_I18N_COVERAGE !== "1") return;

  const catalog = path.join(WEB_DIR, "src", "locales", "pseudo", "common.json");
  if (!fs.existsSync(catalog)) return;
  const markers = Object.values(
    JSON.parse(fs.readFileSync(catalog, "utf8")) as Record<string, unknown>,
  )
    .filter((value): value is string => typeof value === "string" && value.length >= 20)
    .slice(0, 20);
  if (markers.length === 0) return;

  const assets = path.join(WEB_DIR, "dist", "assets");
  const bundled = fs
    .readdirSync(assets)
    .filter((name) => name.endsWith(".js"))
    .some((name) => {
      const source = fs.readFileSync(path.join(assets, name), "utf8");
      return markers.some((marker) => source.includes(marker));
    });

  if (!bundled) {
    throw new Error(
      "KANDEV_I18N_COVERAGE=1 but dist/ contains no pseudo catalog, so the coverage " +
        "oracle would report every screen as un-externalized.\n" +
        'Rebuild the SPA with the QA locale: "make build-web-e2e" ' +
        '(or "pnpm --filter @kandev/web build:e2e"). See docs/i18n.md.',
    );
  }
}
