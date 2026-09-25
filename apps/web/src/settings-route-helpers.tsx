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
    router.replace(resolveSettingsRedirect(to));
  }, [router, to]);

  return null;
}

export function resolveSettingsRedirect(to: string): string {
  if (typeof window === "undefined") return to;
  const destination = new URL(to, window.location.href);
  const current = new URL(window.location.href);
  const query = new URLSearchParams(current.search);
  for (const key of new Set(destination.searchParams.keys())) query.delete(key);
  for (const [key, value] of destination.searchParams) query.append(key, value);
  const search = query.toString();
  const hash = destination.hash || (destination.searchParams.has("tab") ? "" : current.hash);
  return `${destination.pathname}${search ? `?${search}` : ""}${hash}`;
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
