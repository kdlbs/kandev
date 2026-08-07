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
 * Catalogs live at `src/locales/<locale>/<namespace>.json`. `en` is bundled into
 * the entry chunk; every other locale is a lazily-fetched chunk, loaded before
 * the locale is activated (see `loadLocale`).
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

/**
 * `en` is bundled eagerly and lands in the entry chunk.
 *
 * It is both `DEFAULT_LOCALE` and `fallbackLng`, and `returnNull: false` renders
 * a missing key as the key itself — so any instant where `en` is not in memory
 * is an instant where the UI paints `common:save` instead of "Save". Keeping it
 * eager is also what lets `ensureInitialized` stay synchronous, which the boot
 * path depends on: react-i18next suspends on an uninitialized instance and
 * there is no Suspense boundary above the React root.
 */
const eagerEnCatalogs = import.meta.glob<CatalogModule>("../../src/locales/en/*.json", {
  eager: true,
  import: "default",
});

/**
 * Every other locale, as loader thunks — one lazily-fetched chunk per locale.
 *
 * Without `eager` Vite still resolves the file list at BUILD time; only the
 * module bodies move out of the initial download. That is what keeps
 * `knownNamespaces()` a static, synchronous answer.
 *
 * `en` is excluded rather than left to overlap the eager glob: a file that is
 * both statically and dynamically imported stays in the static chunk anyway, and
 * only earns an `INEFFECTIVE_DYNAMIC_IMPORT` warning for saying so twice.
 *
 * `pseudo` is globbed separately, behind the build-time constant, so a
 * production build folds the branch away and rolldown never pulls those JSON
 * modules into the graph — no chunk is emitted for it at all. Lazy loading is
 * NOT a substitute for that exclusion: an unfetched chunk is still built,
 * deployed, and reachable by anyone who can set the cookie. The two changes are
 * orthogonal, and this is where they compose.
 */
const lazyCatalogs: Record<string, () => Promise<CatalogModule>> = {
  ...import.meta.glob<CatalogModule>(
    [
      "../../src/locales/*/*.json",
      "!../../src/locales/en/*.json",
      "!../../src/locales/pseudo/*.json",
    ],
    { import: "default" },
  ),
  ...(__KANDEV_PSEUDO_LOCALE_BUNDLED__
    ? import.meta.glob<CatalogModule>("../../src/locales/pseudo/*.json", { import: "default" })
    : {}),
};

const CATALOG_PATH = /\/locales\/([^/]+)\/([^/]+)\.json$/;

function parseCatalogPath(path: string): { locale: string; namespace: string } | undefined {
  const match = CATALOG_PATH.exec(path);
  if (!match) return undefined;
  return { locale: match[1], namespace: match[2] };
}

/** The bundled `en` catalogs, keyed by namespace. */
function enResources(): Record<string, CatalogModule> {
  const resources: Record<string, CatalogModule> = {};
  for (const [path, messages] of Object.entries(eagerEnCatalogs)) {
    const parsed = parseCatalogPath(path);
    if (parsed) resources[parsed.namespace] = messages;
  }
  return resources;
}

/** Namespaces present in the source catalog; used to seed i18next. */
export function knownNamespaces(): string[] {
  const names = new Set<string>([DEFAULT_NAMESPACE]);
  for (const path of [...Object.keys(eagerEnCatalogs), ...Object.keys(lazyCatalogs)]) {
    const parsed = parseCatalogPath(path);
    if (parsed) names.add(parsed.namespace);
  }
  return [...names];
}

/** Locales whose catalogs are registered on the shared instance. */
const loadedLocales = new Set<SupportedLocale>([DEFAULT_LOCALE]);
/** In-flight loads, so two switches in a row share one fetch. */
const pendingLoads = new Map<SupportedLocale, Promise<void>>();

async function fetchLocale(locale: SupportedLocale): Promise<void> {
  const loaders = Object.entries(lazyCatalogs).flatMap(([path, load]) => {
    const parsed = parseCatalogPath(path);
    return parsed?.locale === locale ? [[parsed.namespace, load] as const] : [];
  });
  const bundles = await Promise.all(
    loaders.map(async ([namespace, load]) => [namespace, await load()] as const),
  );
  for (const [namespace, messages] of bundles) {
    i18next.addResourceBundle(locale, namespace, messages, true, true);
  }
}

/**
 * Fetch and register `locale`'s catalogs. Idempotent, and concurrent callers
 * share one load. `en` is bundled, so it resolves without touching the network.
 *
 * Rejects if a chunk fails to load; `loadLocaleOrFallback` is the boot-safe
 * wrapper every caller in this module actually uses.
 */
export async function loadLocale(locale: SupportedLocale): Promise<void> {
  if (loadedLocales.has(locale)) return;
  const inflight = pendingLoads.get(locale);
  if (inflight) return inflight;
  const load = fetchLocale(locale)
    .then(() => {
      loadedLocales.add(locale);
    })
    .finally(() => {
      pendingLoads.delete(locale);
    });
  pendingLoads.set(locale, load);
  return load;
}

/**
 * `loadLocale` that never rejects.
 *
 * A catalog chunk that 404s or times out must not blank the app or produce an
 * unhandled rejection from the switcher's fire-and-forget handler. `en` is
 * bundled, so the failure mode is English copy under the requested `<html lang>`
 * — degraded, but readable, and self-healing: `installVitePreloadRecovery`
 * reloads once on `vite:preloadError`, and the next boot retries the fetch.
 */
async function loadLocaleOrFallback(locale: SupportedLocale): Promise<void> {
  try {
    await loadLocale(locale);
  } catch (error) {
    console.error(
      `[i18n] could not load the "${locale}" catalogs; falling back to ${DEFAULT_LOCALE}`,
      error,
    );
  }
}

// Safety net for anything that drives the shared instance directly rather than
// through `activateLocale`. It repairs the locale a tick late (English until the
// chunk lands, then a re-render from `addResourceBundle`), which is why it is a
// net and not the mechanism — see `activateLocale`.
i18next.on("languageChanged", (next: string) => {
  if (!isSupportedLocale(next)) return;
  void loadLocaleOrFallback(normalizeLocale(next));
});

let initialized = false;

function ensureInitialized(locale: SupportedLocale) {
  if (initialized) return;
  void i18next.use(initReactI18next).init({
    lng: locale,
    lowerCaseLng: true,
    fallbackLng: DEFAULT_LOCALE,
    defaultNS: DEFAULT_NAMESPACE,
    ns: knownNamespaces(),
    // `en` only. Inline resources with no backend mean init completes
    // synchronously and `hasLoadedNamespace` is always true, so `useTranslation`
    // never suspends — the contract init-sync.test.tsx pins. Other locales
    // arrive through `addResourceBundle`, which does not change that.
    resources: { [DEFAULT_LOCALE]: enResources() },
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
 * Activate a locale: fetch its catalogs, switch i18next, reflect it on
 * `<html lang>`, and persist the cookie. Unknown locales coerce to `en`.
 * Returns the locale activated.
 *
 * The load happens BEFORE `changeLanguage`, not on the `languageChanged` event.
 * `changeLanguage` re-renders the whole tree, so registering the catalog after
 * it would paint one frame of English and then flip — the switch is atomic this
 * way. The switcher's handler is fire-and-forget, so this never rejects.
 *
 * Catalogs are fetched, so two rapid selections race: pick `zh-cn` then `pt-pt`
 * and the `zh-cn` fetch can settle last, switching the app back and persisting a
 * cookie the user never chose. Eager bundling had no such window. Each call
 * claims a sequence number and abandons everything after the await if a newer
 * one has started — the catalog it already registered stays, which is harmless.
 *
 * A stale call reports the locale that ACTUALLY won, not the one it was asked
 * for. The switcher does `setLocale(await activateLocale(value))`, so returning
 * the abandoned locale would leave the dropdown reading "简体中文" over a
 * Portuguese app — the same race, surviving in the one place the user looks to
 * confirm it did not happen. Both orderings converge on the winner: if the newer
 * call already finished, this returns its locale; if it has not, this returns the
 * pre-switch locale and the newer call's own `setLocale` lands after.
 */
let latestActivation = 0;

export async function activateLocale(locale: string): Promise<SupportedLocale> {
  const normalized = normalizeLocale(locale);
  const activation = ++latestActivation;
  ensureInitialized(normalized);
  await loadLocaleOrFallback(normalized);
  if (activation !== latestActivation) return normalizeLocale(i18next.language);
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
 * Fetch the boot locale's catalogs and make it the active language.
 *
 * `main.tsx` awaits this before `createRoot().render()`, which is the whole
 * first-paint story: the Go shell resolves the locale into the boot payload, so
 * exactly ONE locale is fetched, and React does not mount until its messages are
 * registered. Nothing ever renders against a half-loaded catalog, so there is no
 * flash of English and — the failure `returnNull: false` would otherwise make
 * silent — no frame of raw `namespace:key` strings.
 *
 * For `en` this is a resolved promise: the catalogs are already in the entry
 * chunk, so the boot is byte-for-byte what it was before.
 */
export async function preloadLocale(locale: SupportedLocale): Promise<void> {
  ensureInitialized(locale);
  await loadLocaleOrFallback(locale);
  if (i18next.language !== locale) {
    await i18next.changeLanguage(locale);
  }
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
 * Load EVERY locale up front, for unit tests only.
 *
 * Suites drive the shared instance with a bare `i18n.changeLanguage("pseudo")`
 * and assert on the next line — a synchronous expectation that lazy catalogs
 * would break in ~20 files. `vitest.setup.ts` awaits this once, restoring the
 * "all catalogs in memory" world those tests were written against without
 * putting it back in the browser bundle.
 */
export async function loadAllLocalesForTests(): Promise<void> {
  await Promise.all(SUPPORTED_LOCALES.map((locale) => loadLocale(locale)));
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
