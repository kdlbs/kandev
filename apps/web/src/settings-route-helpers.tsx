"use client";

import { useEffect } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { rememberSettingsPath } from "@/lib/settings/last-settings-page";

/**
 * A settings path that only forwards to another one — `/settings/general` to its
 * first page, `/settings/general/shell` to Terminal, and the handful of others
 * left behind when pages moved. `replace`, so Back skips the stub.
 */
export function SettingsRedirect({ to }: { to: string }) {
  const router = useRouter();

  useEffect(() => {
    router.replace(to);
  }, [router, to]);

  return null;
}

/**
 * Records the settings page bare `/settings` should return to.
 *
 * Takes the route table's own set of static paths: the settings shell renders —
 * and would therefore record — any `/settings/*` path, including ones that fall
 * through to the not-ported fallback, and the dynamic routes resolve against
 * workspaces, agents and plugins that can be deleted.
 */
export function useRememberSettingsPath(pathname: string, knownPaths: ReadonlySet<string>): void {
  useEffect(() => {
    rememberSettingsPath(pathname, knownPaths);
  }, [pathname, knownPaths]);
}
