/**
 * The `kandev_locale` cookie is the source of truth for the active UI locale.
 * The Go shell reads it to set `<html lang>` and the boot payload on first
 * paint; the SPA writes it when the user picks a language. Kept in sync with
 * the backend constant `localeCookieName` in
 * apps/backend/internal/backendapp/helpers.go.
 */
export const LOCALE_COOKIE = "kandev_locale";

const ONE_YEAR_SECONDS = 60 * 60 * 24 * 365;

export function readLocaleCookie(doc: Document = document): string | undefined {
  const cookie = doc.cookie ?? "";
  for (const part of cookie.split(";")) {
    const [rawName, ...rawValue] = part.split("=");
    if (rawName?.trim() === LOCALE_COOKIE) {
      try {
        const value = decodeURIComponent(rawValue.join("=").trim());
        // An empty value (e.g. a cleared cookie) reads as "not set".
        return value || undefined;
      } catch {
        // `decodeURIComponent` throws on a malformed percent-escape. The cookie is
        // external input — it can be truncated or hand-edited — and this runs at
        // module scope via resolveInitialLocale, before React mounts. An escaping
        // throw there is unrecoverable and renders a blank page with no error of
        // any kind (see provider.tsx). Treat it exactly like an unset cookie.
        return undefined;
      }
    }
  }
  return undefined;
}

export function writeLocaleCookie(locale: string, doc: Document = document): void {
  doc.cookie = `${LOCALE_COOKIE}=${encodeURIComponent(locale)}; path=/; max-age=${ONE_YEAR_SECONDS}; SameSite=Lax`;
}
