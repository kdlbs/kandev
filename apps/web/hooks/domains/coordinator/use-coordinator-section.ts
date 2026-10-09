"use client";

import { useCallback, useMemo } from "react";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { resolveSettingsTab } from "@/hooks/domains/settings/use-settings-tab";

const SECTION_PARAM = "section";
export const DEFAULT_SECTION = "identity";

/**
 * The coordinator page's selected section, kept in `?section=` and replaced
 * (never pushed) on change, as `?tab=` is for other settings pages. A missing,
 * unknown or unregistered value selects the default section.
 */
export function useCoordinatorSection(slugs: readonly string[]) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const router = useRouter();
  const value = useMemo(
    () => resolveSettingsTab(searchParams.get(SECTION_PARAM), slugs, DEFAULT_SECTION),
    [searchParams, slugs],
  );

  const selectSection = useCallback(
    (next: string) => {
      if (!slugs.includes(next) || pathname !== window.location.pathname) return;
      const params = new URLSearchParams(window.location.search);
      params.set(SECTION_PARAM, next);
      const href = `${pathname}?${params.toString()}`;
      runWithNavigationBlockerBypassed(() => router.replace(href, { scroll: false }));
    },
    [pathname, router, slugs],
  );

  return { value, selectSection };
}
