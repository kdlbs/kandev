"use client";

import { useState } from "react";
import { IconCode } from "@tabler/icons-react";
import { Trans, useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import type { JiraAuthMethod } from "@/lib/types/jira";

// Session cookies are HttpOnly so document.cookie can't read them, but
// DevTools → Application → Cookies surfaces them in plain text. Users copy
// the Value cell of a single row; the backend wraps it under both
// cloud.session.token and tenant.session.token so a single paste works for
// password accounts and SSO tenants.
//
// The instructions themselves are copy (`jira:cookieInstructions`, resolved at
// render), but every token below names something the user has to match verbatim
// in DevTools — cookie rows, a host, a keyboard shortcut. They are interpolated
// rather than written into the catalog so no locale, pseudo included, can turn
// one into a name that does not exist.
const SESSION_COOKIE_NAME = "cloud.session.token";
const SSO_SESSION_COOKIE_NAME = "tenant.session.token";
const ATLASSIAN_COOKIE_HOST = "https://*.atlassian.net";
// i18n-exempt: keyboard shortcut, interpolated so no locale rewrites it. See the comment above.
const DEVTOOLS_SHORTCUT = "Cmd+Opt+I / Ctrl+Shift+I";
// Atlassian's API-token page. The visible link text is the URL itself, so it is
// an identifier in both roles and never reaches the catalog.
const API_TOKEN_URL = "https://id.atlassian.com/manage-profile/security/api-tokens";
const API_TOKEN_URL_LABEL = "id.atlassian.com/manage-profile/security/api-tokens";
// Symbol-only stand-in for a stored secret — nothing to translate.
const MASKED_SECRET = "••••••••";

/**
 * Field label and empty-state placeholder per auth method.
 *
 * The record keys are the wire `JiraAuthMethod` values and must never be
 * translated; only the label and placeholder are copy. Keyed by JiraAuthMethod
 * so adding a new method causes the type system to flag the missing entry.
 * Built from `t` at render — a module-scope table would freeze at the boot
 * locale and the pseudo-locale could not see it.
 */
export function secretCopy(
  t: TFunction,
): Record<JiraAuthMethod, { label: string; placeholder: string }> {
  return {
    api_token: { label: t("jira:apiToken"), placeholder: t("jira:pasteApiTokenHere") },
    pat: {
      label: t("jira:personalAccessToken"),
      placeholder: t("jira:pastePersonalAccessTokenHere"),
    },
    session_cookie: {
      label: t("jira:sessionTokenValue"),
      placeholder: t("jira:pasteSessionTokenValue", { cookieName: SESSION_COOKIE_NAME }),
    },
    // OAuth has no paste field (OAuthFields renders the connect button instead);
    // this entry only satisfies the exhaustive Record<JiraAuthMethod, ...> type.
    oauth: { label: t("jira:oauth2"), placeholder: "" },
  };
}

export function secretPlaceholder(
  t: TFunction,
  method: JiraAuthMethod,
  hasSavedSecret: boolean,
): string {
  return hasSavedSecret ? MASKED_SECRET : secretCopy(t)[method].placeholder;
}

function SessionSnippet() {
  const { t } = useTranslation();
  const [show, setShow] = useState(false);
  return (
    <div className="text-xs text-muted-foreground space-y-2">
      <button
        type="button"
        onClick={() => setShow((v) => !v)}
        className="inline-flex items-center gap-1 underline cursor-pointer"
      >
        <IconCode className="h-3 w-3" />
        {/* Two whole sentences rather than "Show"/"Hide" spliced onto a shared
            tail: which word varies, and where it sits, is the translator's. */}
        {show ? t("jira:hideSessionTokenHelp") : t("jira:showSessionTokenHelp")}
      </button>
      {show && (
        <pre className="bg-muted rounded p-3 text-[11px] overflow-x-auto whitespace-pre-wrap">
          <code>
            {t("jira:cookieInstructions", {
              shortcut: DEVTOOLS_SHORTCUT,
              host: ATLASSIAN_COOKIE_HOST,
              cookieName: SESSION_COOKIE_NAME,
              ssoCookieName: SSO_SESSION_COOKIE_NAME,
            })}
          </code>
        </pre>
      )}
    </div>
  );
}

// `t` is threaded in rather than read from a hook: this is a plain function, and
// the guard only inspects JSX, so a literal returned from here would never be
// reported. Exported for its unit test.
export function formatExpiry(
  t: TFunction,
  expiresAt: string,
): { label: string; tone: "ok" | "warn" | "danger" } {
  const diffMs = new Date(expiresAt).getTime() - Date.now();
  if (Number.isNaN(diffMs)) return { label: t("jira:expiryUnknown"), tone: "warn" };
  if (diffMs <= 0) return { label: t("jira:cookieExpired"), tone: "danger" };
  const hours = diffMs / (60 * 60 * 1000);
  if (hours < 24) {
    const h = Math.max(1, Math.round(hours));
    return { label: t("jira:cookieExpiresInHours", { count: h }), tone: "danger" };
  }
  const days = Math.round(hours / 24);
  return {
    label: t("jira:cookieExpiresInDays", { count: days }),
    tone: days < 7 ? "warn" : "ok",
  };
}

const TONE_CLASSES: Record<"ok" | "warn" | "danger", string> = {
  ok: "text-muted-foreground",
  warn: "text-amber-600 dark:text-amber-400",
  danger: "text-destructive",
};

export function CookieExpiry({ expiresAt }: { expiresAt: string }) {
  const { t } = useTranslation();
  const { label, tone } = formatExpiry(t, expiresAt);
  // `formatDateTime` follows the active app locale; the previous bare
  // `toLocaleString()` followed the browser's, so the tooltip could disagree
  // with the rest of the page after a language switch.
  //
  // Guarded because the two disagree on a malformed date, which a legacy or
  // corrupted `secretExpiresAt` really can be — `formatExpiry` has a branch for
  // exactly that. `toLocaleString()` returned the string "Invalid Date";
  // `Intl.DateTimeFormat` throws `RangeError`, which would take the whole Jira
  // settings card down with it. No tooltip is the right fallback: the label
  // already says the expiry is unknown.
  const parsed = new Date(expiresAt);
  const absolute = Number.isNaN(parsed.getTime()) ? undefined : formatDateTime(parsed);
  return (
    <p className={`text-xs ${TONE_CLASSES[tone]}`} title={absolute}>
      {label}
    </p>
  );
}

/**
 * Per-method help under the secret input. Each branch is one whole message
 * rather than a stem spliced around a conditional link — where a link sits in a
 * sentence is the translator's call, not English word order's.
 */
export function SecretHelp({ method, patHref }: { method: JiraAuthMethod; patHref?: string }) {
  const { t } = useTranslation();
  if (method === "session_cookie") return <SessionSnippet />;
  if (method === "api_token") {
    return (
      <p className="text-xs text-muted-foreground">
        {/* The link's visible text is the token page's own URL — an identifier in
            both roles, so it travels through `values` rather than the catalog,
            where the pseudo-locale would transliterate it. */}
        <Trans i18nKey="jira:createATokenAt" values={{ url: API_TOKEN_URL_LABEL }}>
          Create a token at{" "}
          <a
            className="underline cursor-pointer"
            href={API_TOKEN_URL}
            target="_blank"
            rel="noreferrer"
          >
            {API_TOKEN_URL_LABEL}
          </a>
        </Trans>
      </p>
    );
  }
  if (method !== "pat") return null;
  if (!patHref) {
    return <p className="text-xs text-muted-foreground">{t("jira:createPatNoProfileLink")}</p>;
  }
  return (
    <p className="text-xs text-muted-foreground">
      {/* The profile URL is derived from the site the user configured, so it is
          data and is interpolated. */}
      <Trans i18nKey="jira:createPatWithProfileLink" values={{ url: patHref }}>
        Create a Personal Access Token from your Jira profile (
        <a className="underline cursor-pointer" href={patHref} target="_blank" rel="noreferrer">
          {patHref}
        </a>
        ) → Personal Access Tokens. Required scopes: read & write.
      </Trans>
    </p>
  );
}
