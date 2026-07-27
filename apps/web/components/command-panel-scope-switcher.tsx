"use client";

import { Kbd } from "@kandev/ui/kbd";
import { useAppStore } from "@/components/state-provider";
import type { CommandPanelMode } from "@/lib/commands/types";
import type { ConfigurableShortcutId } from "@/lib/keyboard/shortcut-overrides";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { formatShortcut } from "@/lib/keyboard/utils";
import { cn } from "@/lib/utils";

export type CommandPanelScopeMode = "commands" | "search-files" | "search-content";

type ScopeOption = {
  mode: CommandPanelScopeMode;
  label: string;
  shortcutId: ConfigurableShortcutId;
};

const SCOPE_OPTIONS: ScopeOption[] = [
  { mode: "commands", label: "Commands", shortcutId: "SEARCH" },
  { mode: "search-files", label: "Files", shortcutId: "FILE_SEARCH" },
  { mode: "search-content", label: "Contents", shortcutId: "CONTENT_SEARCH" },
];

export function isCommandPanelScopeMode(mode: CommandPanelMode): mode is CommandPanelScopeMode {
  return SCOPE_OPTIONS.some((scope) => scope.mode === mode);
}

export function getAdjacentCommandPanelScope(
  mode: CommandPanelScopeMode,
  reverse = false,
): CommandPanelScopeMode {
  const currentIndex = SCOPE_OPTIONS.findIndex((scope) => scope.mode === mode);
  const offset = reverse ? -1 : 1;
  const nextIndex = (currentIndex + offset + SCOPE_OPTIONS.length) % SCOPE_OPTIONS.length;
  return SCOPE_OPTIONS[nextIndex].mode;
}

export function CommandPanelScopeSwitcher({
  mode,
  onScopeChange,
}: {
  mode: CommandPanelScopeMode;
  onScopeChange: (mode: CommandPanelScopeMode) => void;
}) {
  const keyboardShortcuts = useAppStore((state) => state.userSettings.keyboardShortcuts);

  return (
    <div
      role="tablist"
      aria-label="Command palette mode"
      className="flex min-h-10 items-stretch gap-1 px-2"
    >
      {SCOPE_OPTIONS.map((scope) => {
        const active = mode === scope.mode;
        const shortcut = formatShortcut(getShortcut(scope.shortcutId, keyboardShortcuts));
        return (
          <button
            key={scope.mode}
            type="button"
            role="tab"
            aria-label={scope.label}
            aria-selected={active}
            tabIndex={-1}
            title={`${scope.label} (${shortcut})`}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => onScopeChange(scope.mode)}
            className={cn(
              "relative flex min-h-10 cursor-pointer items-center gap-2 px-3 text-xs font-medium text-muted-foreground outline-none transition-colors hover:text-foreground",
              active &&
                "text-foreground after:absolute after:inset-x-3 after:bottom-0 after:h-0.5 after:rounded-full after:bg-primary",
            )}
          >
            <span>{scope.label}</span>
            <Kbd className={cn("h-4 min-w-4 text-[0.55rem]", active && "text-foreground")}>
              {shortcut}
            </Kbd>
          </button>
        );
      })}
    </div>
  );
}
