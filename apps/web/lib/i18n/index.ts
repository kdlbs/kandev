import i18next from "i18next";
import { initReactI18next } from "react-i18next";

import { writeLocaleCookie } from "./cookie";

/**
 * i18n runtime for the Kandev web SPA (i18next).
 *
 * `en` is the source locale; `pseudo` is a QA-only accented/padded locale used
 * to visually prove every string was externalized (any plain-ASCII text on
 * screen under `pseudo` is a literal that was never routed through `t`).
 * `pseudo` is filtered out of the language switcher in production builds, and
 * its catalog is not compiled into them at all — see `PSEUDO_LOCALE_BUNDLED`.
 *
 * Catalogs live at `src/locales/<locale>/<namespace>.json` and are loaded
 * eagerly per locale via Vite's glob import, so a locale switch needs no
 * network round-trip.
 */
export const DEFAULT_LOCALE = "en";
export const DEFAULT_NAMESPACE = "common";
export const SUPPORTED_LOCALES = ["en", "pt-pt", "zh-cn", "pseudo"] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

/** Human-readable labels for the language switcher. */
export const LOCALE_LABELS: Record<SupportedLocale, string> = {
  en: "English",
  "pt-pt": "Português (Portugal)",
  "zh-cn": "简体中文",
  pseudo: "Pseudo (QA)",
};

/** Canonicalize locale input the same way the Go shell does (trim + lower). */
function canonicalLocale(value: string): string {
  return value.trim().toLowerCase();
}

/**
 * Whether `value` names a shipped locale. Accepts case variants such as
 * `zh-CN`; prefer `normalizeLocale` when you need the canonical id to pass on.
 */
export function isSupportedLocale(value: unknown): boolean {
  if (typeof value !== "string") return false;
  return (SUPPORTED_LOCALES as readonly string[]).includes(canonicalLocale(value));
}

/**
 * Coerce any value to a supported canonical locale, defaulting to `en`.
 * Case-insensitive so a hand-edited `zh-CN` cookie matches the backend.
 */
export function normalizeLocale(value: unknown): SupportedLocale {
  if (typeof value !== "string") return DEFAULT_LOCALE;
  const canonical = canonicalLocale(value);
  return (SUPPORTED_LOCALES as readonly string[]).includes(canonical)
    ? (canonical as SupportedLocale)
    : DEFAULT_LOCALE;
}

/**
 * Whether this bundle contains the pseudo catalog at all.
 *
 * `pseudo` stays in `SUPPORTED_LOCALES` unconditionally — that type drives the
 * `en` ↔ `pseudo` parity gate in `i18n:check` and the generator — so this is the
 * only thing that distinguishes "shipped" from "supported". `false` in a plain
 * `vite build`; see `./bundling.ts`.
 */
export const PSEUDO_LOCALE_BUNDLED: boolean = __KANDEV_PSEUDO_LOCALE_BUNDLED__;

/**
 * Locales offered by the switcher. The pseudo QA locale is hidden in production.
 *
 * Takes "is this a production build?" from the caller rather than reading
 * `import.meta.env.PROD`: the e2e harness serves a production bundle, so that
 * constant is true there too and would hide the locale the QA tests need. The
 * boot payload's `nonProduction` flag is the authoritative signal.
 *
 * `PSEUDO_LOCALE_BUNDLED` is checked as well, because those two signals can now
 * disagree: a released binary started with `KANDEV_DEBUG_DEV_MODE=true` reports
 * `nonProduction` while serving a bundle that has no pseudo catalog. Offering it
 * there would switch the user to a locale that resolves entirely through the
 * `en` fallback — plain English under a "Pseudo (QA)" label, which reads exactly
 * like a total externalization failure.
 */
export function selectableLocales(isProd: boolean): SupportedLocale[] {
  return SUPPORTED_LOCALES.filter(
    (locale) => locale !== "pseudo" || (!isProd && PSEUDO_LOCALE_BUNDLED),
  );
}

type CatalogModule = Record<string, unknown>;

// Vite resolves these at build time; each entry is `../../src/locales/<locale>/<ns>.json`.
//
// `pseudo` is globbed separately, behind the build-time constant, so a production
// build folds the branch away and rolldown never pulls those JSON modules into
// the graph. Filtering the merged object at runtime instead would still ship
// every byte — and the bytes are the entire problem: pseudo is the LARGEST
// catalog we have (accented characters are multi-byte) and no production user
// can select it.
const catalogModules: Record<string, CatalogModule> = {
  ...import.meta.glob<CatalogModule>(
    ["../../src/locales/*/*.json", "!../../src/locales/pseudo/*.json"],
    { eager: true, import: "default" },
  ),
  ...(__KANDEV_PSEUDO_LOCALE_BUNDLED__
    ? import.meta.glob<CatalogModule>("../../src/locales/pseudo/*.json", {
        eager: true,
        import: "default",
      })
    : {}),
};

function localeResources(locale: SupportedLocale): Record<string, CatalogModule> {
  const resources: Record<string, CatalogModule> = {};
  for (const [path, messages] of Object.entries(catalogModules)) {
    const match = /\/locales\/([^/]+)\/([^/]+)\.json$/.exec(path);
    if (!match) continue;
    const [, fileLocale, namespace] = match;
    if (fileLocale === locale) resources[namespace] = messages;
  }
  return resources;
}

/** Namespaces present in the source catalog; used to seed i18next. */
export function knownNamespaces(): string[] {
  const names = new Set<string>([DEFAULT_NAMESPACE]);
  for (const path of Object.keys(catalogModules)) {
    const match = /\/locales\/[^/]+\/([^/]+)\.json$/.exec(path);
    if (match) names.add(match[1]);
  }
  return [...names];
}

let initialized = false;

function ensureInitialized(locale: SupportedLocale) {
  if (initialized) return;
  void i18next.use(initReactI18next).init({
    lng: locale,
    lowerCaseLng: true,
    fallbackLng: DEFAULT_LOCALE,
    defaultNS: DEFAULT_NAMESPACE,
    ns: knownNamespaces(),
    resources: Object.fromEntries(
      SUPPORTED_LOCALES.map((candidate) => [candidate, localeResources(candidate)]),
    ),
    interpolation: {
      // React already escapes rendered output; double-escaping mangles copy.
      escapeValue: false,
    },
    // Missing keys fall back to the key itself rather than throwing, so a
    // missed extraction degrades to visible-but-wrong instead of a crash.
    returnNull: false,
  });
  initialized = true;
}

/**
 * Activate a locale: switch i18next, reflect it on `<html lang>`, and persist
 * the cookie. Unknown locales coerce to `en`. Returns the locale activated.
 */
export async function activateLocale(locale: string): Promise<SupportedLocale> {
  const normalized = normalizeLocale(locale);
  ensureInitialized(normalized);
  if (i18next.language !== normalized) {
    await i18next.changeLanguage(normalized);
  }
  if (typeof document !== "undefined") {
    document.documentElement.lang = normalized;
  }
  writeLocaleCookie(normalized);
  return normalized;
}

/**
 * Initialize the shared instance for the app. Idempotent.
 *
 * This must run before the first `useTranslation()`: react-i18next suspends on
 * an uninitialized instance, and there is no Suspense boundary above the React
 * root, so the tree never commits — a blank page with no error and no rejection.
 * `I18nProvider` calls this at module load; nothing else may be relied on to.
 */
export function initI18n(locale: SupportedLocale): void {
  ensureInitialized(locale);
}

/**
 * Initialize i18next synchronously for unit tests. Tests never render the app
 * shell, so nothing else would set the instance up; react-i18next's
 * `useTranslation` then resolves against this default instance with no provider
 * required. Not used by the app itself (see `activateLocale`).
 */
export function initI18nForTests(locale: SupportedLocale = DEFAULT_LOCALE): void {
  ensureInitialized(locale);
}

/**
 * Translate outside React (plain helpers, `.ts` modules). Inside components
 * prefer `useTranslation()` so the tree re-renders on a locale switch.
 */
export function t(key: string, options?: Record<string, unknown>): string {
  ensureInitialized(DEFAULT_LOCALE);
  return i18next.t(key, options) as string;
}

export { i18next as i18n };
