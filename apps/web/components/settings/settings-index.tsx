"use client";

import { useEffect, useRef, useState } from "react";
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
 * Both decisions are made once, at mount. Resizing a window is not a reason to
 * navigate, and `useResponsiveBreakpoint` reads the real viewport on the first
 * client render, so there is no desktop-first flash to guard against.
 */
export function SettingsIndex({ restoreTo }: { restoreTo: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { isMobile } = useResponsiveBreakpoint();
  const [showIndex] = useState(isMobile);
  // Captured so a re-render cannot retarget an in-flight handoff.
  const restoreTarget = useRef(restoreTo);

  useEffect(() => {
    if (showIndex) return;
    // `replace`, not `push`: with a history entry for `/settings` still in
    // place, Back would land here and immediately redirect again.
    router.replace(restoreTarget.current);
  }, [router, showIndex]);

  if (!showIndex) return null;

  return (
    <nav data-testid="settings-index" aria-label={t("common:settings")}>
      <SettingsPageNav pathname={SETTINGS_INDEX_PATH} defaultOpenGroup="general" />
    </nav>
  );
}
