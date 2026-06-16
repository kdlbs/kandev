"use client";

import { useMemo, useSyncExternalStore } from "react";

import { LOCATION_CHANGE_EVENT } from "./navigation-event";

type NavigateOptions = {
  scroll?: boolean;
};

export type AppRouter = {
  push: (href: string, options?: NavigateOptions) => void;
  replace: (href: string, options?: NavigateOptions) => void;
  refresh: () => void;
  back: () => void;
  forward: () => void;
  prefetch: (_href: string) => void;
};

export function useRouter(): AppRouter {
  return useMemo(
    () => ({
      push: (href, options) => navigate("push", href, options),
      replace: (href, options) => navigate("replace", href, options),
      refresh: () => window.location.reload(),
      back: () => window.history.back(),
      forward: () => window.history.forward(),
      prefetch: () => undefined,
    }),
    [],
  );
}

export function usePathname(): string {
  return parseLocationKey(useLocationKey()).pathname;
}

export function useSearchParams(): URLSearchParams {
  const { search } = parseLocationKey(useLocationKey());
  return useMemo(() => new URLSearchParams(search), [search]);
}

export function useParams(): Record<string, string> {
  const { pathname } = parseLocationKey(useLocationKey());
  return useMemo(() => paramsForPath(pathname), [pathname]);
}

function navigate(mode: "push" | "replace", href: string, options?: NavigateOptions): void {
  if (mode === "push") {
    window.history.pushState({}, "", href);
  } else {
    window.history.replaceState({}, "", href);
  }
  if (options?.scroll !== false) {
    window.scrollTo(0, 0);
  }
  dispatchLocationChange();
}

function useLocationKey(): string {
  return useSyncExternalStore(subscribeLocation, getLocationKey, getServerLocationKey);
}

type LocationSnapshot = {
  pathname: string;
  search: string;
};

function subscribeLocation(callback: () => void): () => void {
  window.addEventListener("popstate", callback);
  window.addEventListener(LOCATION_CHANGE_EVENT, callback);
  return () => {
    window.removeEventListener("popstate", callback);
    window.removeEventListener(LOCATION_CHANGE_EVENT, callback);
  };
}

function getLocationKey(): string {
  return `${window.location.pathname}\n${window.location.search}`;
}

function getServerLocationKey(): string {
  return "/\n";
}

function parseLocationKey(key: string): LocationSnapshot {
  const [pathname, search = ""] = key.split("\n", 2);
  return { pathname, search };
}

function dispatchLocationChange(): void {
  window.dispatchEvent(new Event(LOCATION_CHANGE_EVENT));
}

function paramsForPath(pathname: string): Record<string, string> {
  const segments = pathname.split("/").filter(Boolean).map(decodeURIComponent);
  const params: Record<string, string> = {};

  assignSingle(params, "taskId", segments, ["t"]);
  assignSingle(params, "id", segments, ["office", "tasks"]);
  assignSingle(params, "id", segments, ["office", "projects"]);
  assignSingle(params, "id", segments, ["office", "routines"]);
  assignSingle(params, "id", segments, ["office", "agents"]);
  assignSingle(params, "runId", segments, ["office", "agents", "*", "runs"]);
  assignSingle(params, "agentId", segments, ["settings", "agents"]);
  assignSingle(params, "profileId", segments, ["settings", "agents", "*", "profiles"]);
  assignSingle(params, "profileId", segments, ["settings", "executors"]);
  assignSingle(params, "executorId", segments, ["settings", "executors", "ssh"]);
  assignSingle(params, "type", segments, ["settings", "executors", "new"]);
  assignSingle(params, "id", segments, ["settings", "workspace"]);
  assignSingle(params, "automationId", segments, ["settings", "workspace", "*", "automations"]);

  return params;
}

function assignSingle(
  params: Record<string, string>,
  key: string,
  segments: string[],
  pattern: string[],
): void {
  const value = matchSingleParam(segments, pattern);
  if (value) params[key] = value;
}

function matchSingleParam(segments: string[], pattern: string[]): string | null {
  if (segments.length <= pattern.length) return null;
  for (let i = 0; i < pattern.length; i++) {
    const expected = pattern[i];
    if (expected !== "*" && segments[i] !== expected) return null;
  }
  return segments[pattern.length] ?? null;
}
