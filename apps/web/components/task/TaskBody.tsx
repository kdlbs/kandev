"use client";

/**
 * TaskBody — simple | advanced switcher.
 *
 *   simple   -> <OfficeSimplePane>   (Linear-style: comments, properties)
 *   advanced -> <DockviewLayout>     (panels: chat, files, terminal)
 *
 * The switcher itself takes already-rendered slots. Each shell
 * (kanban / office) does its own data plumbing — TaskBody only picks
 * which slot to mount. This keeps the unified surface tiny and lets
 * each shell stay in charge of its hydration / WS subscriptions.
 *
 * Defaults differ by route:
 *   /t/:id            -> advanced (kanban shell)
 *   /office/tasks/:id -> simple   (office shell)
 *
 * URL search params override:
 *   /t/:id?simple            -> simple
 *   /office/tasks/:id?advanced -> advanced
 */

import type { ReactNode } from "react";

export type TaskBodyMode = "simple" | "advanced";

export type TaskBodyProps = {
  mode: TaskBodyMode;
  simpleSlot?: ReactNode;
  advancedSlot?: ReactNode;
};

/**
 * Pure URL-mode resolver. Exported for tests.
 *
 * @param searchParams plain object from Next's useSearchParams or page-prop
 * @param defaultMode  shell-default mode (kanban=advanced, office=simple)
 */
export function resolveTaskBodyMode(
  searchParams: { simple?: unknown; advanced?: unknown; mode?: unknown } | URLSearchParams | null,
  defaultMode: TaskBodyMode,
): TaskBodyMode {
  if (!searchParams) return defaultMode;
  const has = (key: string): boolean => {
    if (searchParams instanceof URLSearchParams) return searchParams.has(key);
    return key in searchParams && searchParams[key as keyof typeof searchParams] !== undefined;
  };
  const get = (key: string): string | null => {
    if (searchParams instanceof URLSearchParams) return searchParams.get(key);
    const value = searchParams[key as keyof typeof searchParams];
    return typeof value === "string" ? value : null;
  };

  if (has("simple")) return "simple";
  if (has("advanced")) return "advanced";
  const explicit = get("mode");
  if (explicit === "simple" || explicit === "advanced") return explicit;
  return defaultMode;
}

export function TaskBody({ mode, simpleSlot, advancedSlot }: TaskBodyProps) {
  if (mode === "advanced") return <>{advancedSlot ?? null}</>;
  return <>{simpleSlot ?? null}</>;
}
