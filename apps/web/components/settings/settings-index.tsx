"use client";

import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { SettingsPageNav } from "@/components/settings/settings-page-nav";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useRouter } from "@/lib/routing/client-router";

const SETTINGS_INDEX_PATH = "/settings";

/**
 * Bare `/settings`.
 *
 * Below `md` it *is* the settings index: the same tree the nav sheet shows,
 * rendered as a real route. There is no sidebar there, so the list is the
 * content — and the breadcrumb's "Settings" crumb (which every settings leaf
 * emits) finally points at a page instead of a duplicate.
 *
 * From `md` up the sidebar already shows that tree, so a page repeating it would
 * be the duplication this route used to be: it hands off to `restoreTo`, the
 * settings page this device was last on.
 *
 * `restoreTo` is resolved by the route table, which owns the list of static
 * settings paths that a stored value has to still be a member of — see
 * `readLastSettingsPath`.
 *
 * The handoff follows the live breakpoint rather than the one at mount: growing
 * a window past `md` reveals the sidebar's copy of this tree, and two identical
 * menus side by side is the duplication this route exists to remove.
 * `useResponsiveBreakpoint` reads the real viewport on the first client render,
 * so there is no desktop-first flash to guard against either way.
 */
export function SettingsIndex({ restoreTo }: { restoreTo: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { isMobile } = useResponsiveBreakpoint();
  // Captured so a re-render cannot retarget an in-flight handoff. Nothing
  // records `/settings`, so the stored value cannot change while we sit here.
  const restoreTarget = useRef(restoreTo);

  useEffect(() => {
    if (isMobile) return;
    // The sidebar is interactive while this route's chunk is still loading, so a
    // tap can navigate away before this effect flushes. Read the live location
    // rather than trusting mount order: handing off after the user has left
    // would drag them back into Settings.
    if (window.location.pathname !== SETTINGS_INDEX_PATH) return;
    // `replace`, not `push`: with a history entry for `/settings` still in
    // place, Back would land here and immediately redirect again.
    router.replace(restoreTarget.current);
  }, [router, isMobile]);

  if (!isMobile) return null;

  return (
    <nav data-testid="settings-index" aria-label={t("common:settings")}>
      <SettingsPageNav pathname={SETTINGS_INDEX_PATH} />
    </nav>
  );
}
