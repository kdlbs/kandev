import fs from "node:fs";
import path from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

import { shouldBundlePseudoLocale } from "./lib/i18n/bundling";

const LOCALES_DIR = path.resolve(__dirname, "src/locales");

/**
 * One chunk per non-`en` locale, instead of one per locale-namespace file.
 *
 * `lib/i18n/index.ts` glob-imports the catalogs lazily, which by default emits a
 * chunk per JSON file — 29 namespaces × 3 locales. i18next is seeded with every
 * namespace and any surface may use any of them, so a locale is always loaded
 * whole; per-namespace chunks buy no smaller download and cost a non-English
 * boot 29 round trips before it can paint. Grouping makes that one request.
 *
 * Read from disk rather than mirroring `SUPPORTED_LOCALES`, so adding a locale
 * directory needs no edit here. `en` is deliberately absent: it is statically
 * imported into the entry chunk as the synchronous fallback.
 */
function localeChunkGroups() {
  return fs
    .readdirSync(LOCALES_DIR, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && entry.name !== "en")
    .map((entry) => ({
      name: `i18n-${entry.name}`,
      test: new RegExp(`/src/locales/${escapeRegExp(entry.name)}/[^/]+\\.json$`),
    }));
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export default defineConfig({
  plugins: [react(), pseudoLocaleBundling()],
  server: {
    port: readPort(process.env.PORT),
    strictPort: Boolean(process.env.PORT),
    // Remote dev hosts (cloud VMs, LAN hostnames) are otherwise blocked by
    // Vite's DNS-rebinding host check. Opt in per environment with an
    // explicit comma-separated host list, e.g.
    // KANDEV_VITE_ALLOWED_HOSTS="a.example.com,b.example.com".
    allowedHosts: readAllowedHosts(process.env.KANDEV_VITE_ALLOWED_HOSTS),
  },
  preview: {
    port: readPort(process.env.PORT),
    strictPort: Boolean(process.env.PORT),
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "."),
      "@kandev/ui": path.resolve(__dirname, "../packages/ui/src"),
      "@kandev/theme": path.resolve(__dirname, "../packages/theme/src"),
      "@kandev/types": path.resolve(__dirname, "../packages/types/src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rolldownOptions: {
      output: {
        advancedChunks: { groups: localeChunkGroups() },
      },
    },
  },
});

/**
 * Defines `__KANDEV_PSEUDO_LOCALE_BUNDLED__`, which gates the `pseudo` catalog's
 * `import.meta.glob` in `lib/i18n/index.ts`. See `lib/i18n/bundling.ts` for why
 * the build invocation — not `import.meta.env.PROD` — decides this.
 *
 * A plugin rather than a top-level `define` because only the `config` hook is
 * handed `ConfigEnv.command`, and keeping the default export an object literal
 * leaves `vitest.config.ts`'s `mergeConfig(viteConfig, …)` working unchanged.
 */
function pseudoLocaleBundling(): Plugin {
  return {
    name: "kandev:i18n-pseudo-locale-bundling",
    config(_userConfig, env) {
      return {
        define: {
          __KANDEV_PSEUDO_LOCALE_BUNDLED__: JSON.stringify(
            shouldBundlePseudoLocale({ command: env.command, env: process.env }),
          ),
        },
      };
    },
  };
}

function readPort(value: string | undefined): number | undefined {
  if (!value) return undefined;
  const port = Number(value);
  return Number.isInteger(port) && port > 0 ? port : undefined;
}

/**
 * Reads the comma-separated KANDEV_VITE_ALLOWED_HOSTS override. Returns
 * undefined (Vite's default host policy) when unset, or the explicit host
 * list. Wildcards are deliberately unsupported: allowing every host would
 * expose the dev server to DNS-rebinding attacks, so only named hosts can
 * be opted in.
 */
function readAllowedHosts(value: string | undefined): string[] | undefined {
  if (!value) return undefined;
  return value
    .split(",")
    .map((host) => host.trim())
    .filter(Boolean);
}
