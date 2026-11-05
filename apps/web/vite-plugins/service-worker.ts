import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { build, type Plugin, type ResolvedConfig } from "vite";

const SW_ENTRY = path.resolve(__dirname, "../sw/service-worker.ts");
const SW_OUTPUT = "sw.js";
const OFFLINE_COPY_KEYS = ["offlineTitle", "offlineDescription", "offlineRetry"] as const;

type OfflineCopy = { title: string; description: string; retry: string };

/** Build a small offline dictionary from the same catalogs used by the app. */
export function createOfflineCopyScript(localeRoot: string): string {
  const messages: Record<string, OfflineCopy> = {};
  const locales = fs
    .readdirSync(localeRoot)
    .filter((locale) => fs.existsSync(path.join(localeRoot, locale, "common.json")))
    .sort();
  for (const locale of locales) {
    const catalogPath = path.join(localeRoot, locale, "common.json");
    const catalog = JSON.parse(fs.readFileSync(catalogPath, "utf8")) as Record<string, unknown>;
    const values = OFFLINE_COPY_KEYS.map((key) => catalog[key]);
    if (values.some((value) => typeof value !== "string")) {
      throw new Error(`offline copy is missing from ${locale}/common.json`);
    }
    messages[locale] = {
      title: values[0] as string,
      description: values[1] as string,
      retry: values[2] as string,
    };
  }
  const serialized = JSON.stringify(messages).replace(/</g, "\\u003c");
  // i18n-exempt: the page text comes from locale catalogs serialized above.
  return `<script>
    const messages = ${serialized};
    let savedLocale;
    for (const item of document.cookie.split(";")) {
      const [name, ...value] = item.trim().split("=");
      if (name !== "kandev_locale") continue;
      try { savedLocale = decodeURIComponent(value.join("=")); } catch { /* Use the browser locale. */ }
      break;
    }
    const requestedLocale = (savedLocale || navigator.language || "en").toLowerCase();
    let locale = requestedLocale;
    if (!Object.prototype.hasOwnProperty.call(messages, locale)) {
      if (locale.startsWith("zh-hk") || locale.startsWith("zh-hant-hk")) locale = "zh-hk";
      else if (locale.startsWith("zh-tw") || locale.startsWith("zh-hant")) locale = "zh-tw";
      else if (locale.startsWith("zh")) locale = "zh-cn";
      else if (locale.startsWith("pt")) locale = "pt-pt";
      else locale = locale.split("-")[0];
    }
    const copy = Object.prototype.hasOwnProperty.call(messages, locale) ? messages[locale] : messages.en;
    document.documentElement.lang = Object.prototype.hasOwnProperty.call(messages, locale) ? locale : "en";
    document.getElementById("offline-title").textContent = copy.title;
    document.getElementById("offline-description").textContent = copy.description;
    document.getElementById("offline-retry").textContent = copy.retry;
  </script>`;
}

function localizeOfflinePage(outDir: string): void {
  const offlinePath = path.join(outDir, "offline.html");
  if (!fs.existsSync(offlinePath)) return;
  const html = fs.readFileSync(offlinePath, "utf8");
  const marker = "<!-- OFFLINE_COPY_SCRIPT -->";
  if (!html.includes(marker)) throw new Error("offline.html has no localization marker");
  const localeRoot = path.resolve(__dirname, "../src/locales");
  fs.writeFileSync(offlinePath, html.replace(marker, createOfflineCopyScript(localeRoot)));
}

/** Non-hashed static files the worker precaches next to the shell's own assets. */
const STATIC_PRECACHE = [
  "/offline.html",
  "/manifest.webmanifest",
  "/web-app-manifest-192x192.png",
  "/web-app-manifest-512x512.png",
  "/maskable-icon-192x192.png",
  "/maskable-icon-512x512.png",
  "/apple-touch-icon.png",
  "/icon.svg",
];

/**
 * The entry script and stylesheets the built index.html loads. Modulepreload
 * hints are deliberately skipped: they list most of the bundle, and precaching
 * that on every deploy would cost megabytes on a phone. Lazy chunks are cached
 * on first use instead (cache-first for content-hashed assets).
 */
export function shellAssetsFromHtml(html: string): string[] {
  const found = new Set<string>();
  const tags = html.match(/<(?:script|link)\b[^>]*>/g) ?? [];
  for (const tag of tags) {
    if (/\brel="modulepreload"/.test(tag)) continue;
    const url = /\b(?:src|href)="(\/assets\/[^"?#]+)"/.exec(tag)?.[1];
    if (url) found.add(url);
  }
  return [...found].sort();
}

/**
 * Hash of everything the worker precaches plus its own source. Content-hashed
 * assets already change names with their content; the static files do not, so
 * their bytes feed the id and a changed icon invalidates the shell cache.
 */
export function computeBuildId(outDir: string, precache: string[], swSource: string): string {
  const hash = createHash("sha256");
  hash.update(swSource);
  for (const url of precache) {
    hash.update(url);
    const file = path.join(outDir, url);
    if (fs.existsSync(file)) hash.update(fs.readFileSync(file));
  }
  return hash.digest("hex").slice(0, 12);
}

/**
 * Builds `sw.js` into the output root after the main bundle is written. A
 * nested build (rather than a second rollup input) keeps the worker an
 * un-chunked classic script at a fixed, unhashed URL, which a service worker
 * registration requires. Skipped when the app is served under a sub-path (the
 * browser demo), because a worker there could not claim the origin root.
 */
export function serviceWorkerPlugin(): Plugin {
  let config: ResolvedConfig;
  return {
    name: "kandev:service-worker",
    apply: "build",
    configResolved(resolved) {
      config = resolved;
    },
    async writeBundle() {
      if (config.base !== "/") return;
      const outDir = path.resolve(config.root, config.build.outDir);
      localizeOfflinePage(outDir);
      const htmlPath = path.join(outDir, "index.html");
      const shell = fs.existsSync(htmlPath)
        ? shellAssetsFromHtml(fs.readFileSync(htmlPath, "utf8"))
        : [];
      const precache = [...STATIC_PRECACHE, ...shell].filter((url) =>
        fs.existsSync(path.join(outDir, url)),
      );
      const buildId = computeBuildId(outDir, precache, fs.readFileSync(SW_ENTRY, "utf8"));
      await build({
        configFile: false,
        logLevel: "warn",
        publicDir: false,
        define: {
          __KANDEV_SW_BUILD_ID__: JSON.stringify(buildId),
          __KANDEV_SW_PRECACHE__: JSON.stringify(precache),
        },
        build: {
          outDir,
          emptyOutDir: false,
          copyPublicDir: false,
          minify: true,
          target: "es2020",
          lib: {
            entry: SW_ENTRY,
            formats: ["iife"],
            name: "KandevServiceWorker",
            fileName: () => SW_OUTPUT,
          },
        },
      });
    },
  };
}
