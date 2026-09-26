"use client";

import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";

// Shared so the three cannot drift apart in size or shape; only the colour
// differs between them.
const BADGE_BASE = "shrink-0 border px-1 py-0.5 text-[10px] font-medium leading-none";
const BADGE_INFO = "rounded border-primary/35 bg-primary/10 text-primary";
const BADGE_WARN = "rounded border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400";
/**
 * Exported because the integrations index sizes its own copy for a card while
 * the menu sizes one for a row. Only the colour is shared — that is the part
 * that must not drift, since the two badges claim the same thing.
 */
export const BADGE_OK_COLORS =
  "border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400";

/**
 * The badges that qualify a record wherever it is listed.
 *
 * Each of these says something the row itself cannot: which workspace commands
 * ended up acting on, and which profiles a picker will refuse to offer. Both
 * facts matter in the settings menu and on the record's own page, so both live
 * here rather than being redrawn per surface — a badge that reads differently
 * in the menu than on the page it opens is worse than no badge.
 */

/** The workspace new tasks and commands act on. */
export function ActiveWorkspaceBadge() {
  const { t } = useTranslation();
  return (
    <span className={cn(BADGE_BASE, BADGE_INFO, "rounded-full px-1.5")}>
      {t("sidebar:activeWorkspaceBadge")}
    </span>
  );
}

/**
 * An agent whose CLI is not on this machine. Its profiles stay listed and
 * editable — you may be about to install it — but nothing can run against it.
 *
 * Amber, like the disabled badge and like the panel the profile page shows for
 * this same fact: both mark a record you cannot currently run, whatever the
 * reason. Blue is left to the badge that simply says which record is current.
 */
export function NotInstalledBadge() {
  const { t } = useTranslation();
  return <span className={cn(BADGE_BASE, BADGE_WARN)}>{t("agents:notInstalled")}</span>;
}

/**
 * An integration that is connected — credentials stored and the last health
 * check passed. The settings tree showed this before it became a menu; this is
 * the same pill at menu scale.
 */
export function IntegrationEnabledBadge() {
  const { t } = useTranslation();
  return (
    <span className={cn(BADGE_BASE, "rounded", BADGE_OK_COLORS)}>{t("sidebar:enabledBadge")}</span>
  );
}

/**
 * A profile switched off. It is hidden from every task and session picker but
 * stays listed so it can be edited and switched back on — which is exactly why
 * it has to be marked, or it reads as an ordinary profile that pickers have
 * inexplicably lost.
 */
export function DisabledBadge() {
  const { t } = useTranslation();
  return <span className={cn(BADGE_BASE, BADGE_WARN)}>{t("sidebar:disabledBadge")}</span>;
}
