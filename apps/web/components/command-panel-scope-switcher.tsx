"use client";

import { useAppStore } from "@/components/state-provider";
import type { CommandPanelMode } from "@/lib/commands/types";
import type { ConfigurableShortcutId } from "@/lib/keyboard/shortcut-overrides";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { formatShortcut } from "@/lib/keyboard/utils";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export type CommandPanelScopeMode = "commands" | "search-tasks" | "search-files" | "search-content";

type ScopeOption = {
  mode: CommandPanelScopeMode;
  /**
   * Catalog key, resolved at render. `mode` stays the untranslated
   * discriminant — it is compared with `===` in `isCommandPanelScopeMode` and
   * threaded through the command-panel state.
   */
  labelKey: string;
  /** Omitted for scopes that are reachable by click and Tab but unbound. */
  shortcutId?: ConfigurableShortcutId;
  /** Scopes that read a checked-out worktree, so they need an open session. */
  requiresWorkspace?: boolean;
};

const SCOPE_OPTIONS: ScopeOption[] = [
  { mode: "commands", labelKey: "common:scopeCommands", shortcutId: "SEARCH" },
  { mode: "search-tasks", labelKey: "common:scopeTasks" },
  {
    mode: "search-files",
    labelKey: "common:scopeFiles",
    shortcutId: "FILE_SEARCH",
    requiresWorkspace: true,
  },
  {
    mode: "search-content",
    labelKey: "common:scopeContents",
    shortcutId: "CONTENT_SEARCH",
    requiresWorkspace: true,
  },
];

export function isCommandPanelScopeMode(mode: CommandPanelMode): mode is CommandPanelScopeMode {
  return SCOPE_OPTIONS.some((scope) => scope.mode === mode);
}

function availableScopes(workspaceSearchAvailable: boolean): ScopeOption[] {
  return SCOPE_OPTIONS.filter((scope) => workspaceSearchAvailable || !scope.requiresWorkspace);
}

/** Commands and Tasks are always reachable, so the switcher always has tabs. */
export function getAvailableCommandPanelScopes(
  workspaceSearchAvailable: boolean,
): CommandPanelScopeMode[] {
  return availableScopes(workspaceSearchAvailable).map((scope) => scope.mode);
}

export function getAdjacentCommandPanelScope(
  mode: CommandPanelScopeMode,
  reverse = false,
  workspaceSearchAvailable = true,
): CommandPanelScopeMode {
  const scopes = availableScopes(workspaceSearchAvailable);
  const currentIndex = scopes.findIndex((scope) => scope.mode === mode);
  const offset = reverse ? -1 : 1;
  const nextIndex = (currentIndex + offset + scopes.length) % scopes.length;
  return scopes[nextIndex].mode;
}

export function CommandPanelScopeSwitcher({
  mode,
  onScopeChange,
  workspaceSearchAvailable,
}: {
  mode: CommandPanelScopeMode;
  onScopeChange: (mode: CommandPanelScopeMode) => void;
  workspaceSearchAvailable: boolean;
}) {
  const { t } = useTranslation();
  const keyboardShortcuts = useAppStore((state) => state.userSettings.keyboardShortcuts);

  return (
    <div
      role="tablist"
      aria-label={t("common:commandPaletteMode")}
      className="mr-1 flex h-10 max-w-full shrink-0 items-stretch gap-0.5 overflow-x-auto"
    >
      {availableScopes(workspaceSearchAvailable).map((scope) => {
        const active = mode === scope.mode;
        const shortcut = scope.shortcutId
          ? formatShortcut(getShortcut(scope.shortcutId, keyboardShortcuts))
          : null;
        const label = t(scope.labelKey);
        return (
          <button
            key={scope.mode}
            type="button"
            role="tab"
            aria-label={label}
            aria-selected={active}
            tabIndex={-1}
            title={shortcut ? t("common:scopeTitleWithShortcut", { label, shortcut }) : label}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => onScopeChange(scope.mode)}
            className={cn(
              "relative flex h-10 cursor-pointer items-center px-2 text-[0.6875rem] font-medium text-muted-foreground outline-none after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:origin-center after:rounded-full after:bg-foreground/70 after:transition-[opacity,scale] after:duration-150 after:ease-out transition-[color,scale] duration-150 ease-out hover:text-foreground focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50 active:scale-[0.96]",
              active
                ? "text-foreground after:scale-100 after:opacity-100"
                : "after:scale-x-75 after:opacity-0",
            )}
          >
            <span>{label}</span>
          </button>
        );
      })}
    </div>
  );
}
