import { useSyncExternalStore } from "react";
import { formatDistanceToNow, type Locale } from "date-fns";
import { enUS } from "date-fns/locale/en-US";

import { DEFAULT_LOCALE, i18n, normalizeLocale, type SupportedLocale } from "./index";

/**
 * date-fns locale resolution.
 *
 * `./formats` already covers everything `Intl` can do — numbers, dates, times,
 * and the hand-bucketed relative strings that route through the catalog. What
 * `Intl` does not give us is date-fns' fuzzy distance phrasing ("about 1 hour
 * ago", "3 days ago"). Those words live in date-fns' own locale objects, so
 * they translate only if the formatter is handed a `Locale`; with none it
 * silently resolves `defaultLocale` and renders English — which is how a
 * pt-PT user ended up reading "about 1 hour ago" inside Portuguese prose.
 *
 * Kept out of `./index` on purpose. `./index` is on the critical boot path and
 * is edited often; this module is imported only by the handful of surfaces
 * that render a distance, so date-fns' locale chunks stay off that path.
 */

/**
 * Our locale ids to date-fns locales.
 *
 * `en` and `pseudo` resolve to `enUS` statically, which costs no bundle bytes:
 * date-fns ships `enUS` as its own `defaultLocale`, so it is already present in
 * any chunk that calls a formatter. The other two are loaded on demand — they
 * are not tiny, and eagerly importing them would undo the initial-bundle work
 * happening in parallel on the catalog loader.
 *
 * `pseudo` mapping to `enUS` is a decision, not a fallthrough. There is no
 * pseudo date locale; leaving it out would land on `enUS` anyway, but via
 * date-fns' default rather than our intent. It does mean distances are the one
 * part of the UI the pseudo-locale cannot prove, because date-fns owns those
 * strings rather than our catalog.
 */
const LAZY_LOCALES: Partial<Record<SupportedLocale, () => Promise<Locale>>> = {
  "pt-pt": () => import("date-fns/locale/pt").then((m) => m.pt),
  "zh-cn": () => import("date-fns/locale/zh-CN").then((m) => m.zhCN),
  "zh-tw": () => import("date-fns/locale/zh-TW").then((m) => m.zhTW),
  "zh-hk": () => import("date-fns/locale/zh-HK").then((m) => m.zhHK),
};

const loaded = new Map<SupportedLocale, Locale>([
  ["en", enUS],
  ["pseudo", enUS],
]);

const inFlight = new Map<SupportedLocale, Promise<Locale>>();
const listeners = new Set<() => void>();

function activeLocale(): SupportedLocale {
  return normalizeLocale(i18n.language || DEFAULT_LOCALE);
}

/**
 * The date-fns locale for `locale`, loading it if needed. Resolves to `enUS`
 * for any locale with no lazy loader, so a caller never has to special-case
 * the statically-bundled ones.
 */
export function resolveDateLocale(locale: SupportedLocale): Promise<Locale> {
  const cached = loaded.get(locale);
  if (cached) return Promise.resolve(cached);

  const pending = inFlight.get(locale);
  if (pending) return pending;

  const load = LAZY_LOCALES[locale];
  if (!load) return Promise.resolve(enUS);

  const request = load()
    .then((resolved) => {
      loaded.set(locale, resolved);
      return resolved;
    })
    .catch(() => {
      // A failed chunk fetch (offline, stale deploy) must not break the
      // surface — cache the English fallback so we stop retrying on every
      // render and the timestamp still shows, just unlocalized.
      loaded.set(locale, enUS);
      return enUS;
    })
    .finally(() => {
      inFlight.delete(locale);
    });

  inFlight.set(locale, request);
  return request;
}

/**
 * The active date-fns locale, synchronously. Returns `enUS` while a lazily
 * loaded locale is still in flight — components re-render via `useDateLocale`
 * once it lands.
 */
export function dateLocale(): Locale {
  return loaded.get(activeLocale()) ?? enUS;
}

function notify(): void {
  for (const listener of listeners) listener();
}

function subscribe(onStoreChange: () => void): () => void {
  listeners.add(onStoreChange);
  return () => {
    listeners.delete(onStoreChange);
  };
}

/**
 * The active date-fns locale, re-rendering the caller when the language
 * changes or a lazily loaded locale arrives.
 *
 * `dateLocale` is a stable snapshot: it hands back the same cached `Locale`
 * object until one of those two things happens, which is what
 * `useSyncExternalStore` requires.
 */
export function useDateLocale(): Locale {
  return useSyncExternalStore(subscribe, dateLocale, dateLocale);
}

/** Load `locale`'s date-fns locale and wake subscribed components. */
export function primeDateLocale(locale: SupportedLocale): Promise<void> {
  return resolveDateLocale(locale).then(() => {
    notify();
  });
}

/**
 * Format `value` as a distance from now ("about 1 hour ago") in the active
 * locale. Returns "" for missing or unparseable input.
 *
 * Under `en` this is byte-identical to a bare `formatDistanceToNow(d, {
 * addSuffix: true })`: date-fns' `defaultLocale` *is* `enUS`, so naming it
 * changes nothing. That matters because e2e specs query these timestamps by
 * accessible name.
 */
export function formatTimeDistance(
  value: string | number | Date | null | undefined,
  locale: Locale = dateLocale(),
): string {
  if (value === null || value === undefined || value === "") return "";
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return formatDistanceToNow(date, { addSuffix: true, locale });
}

// Track the active language for the whole app lifetime. Registered at import
// rather than from `activateLocale` so this stays out of `./index`; the load
// starts at app boot, well before any distance-rendering surface mounts.
i18n.on("languageChanged", (next: string) => {
  // Notify unconditionally: switching back to an already-cached locale still
  // has to re-render, and `primeDateLocale` only fires after an await.
  notify();
  void primeDateLocale(normalizeLocale(next));
});

void primeDateLocale(activeLocale());
