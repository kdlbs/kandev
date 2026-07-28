"use client";

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
  {
    mode: "search-content",
    label: "Contents",
    shortcutId: "CONTENT_SEARCH",
  },
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
      className="mr-1 flex h-10 shrink-0 items-stretch gap-0.5"
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
              "relative flex h-10 cursor-pointer items-center px-2 text-[0.6875rem] font-medium text-muted-foreground outline-none after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:origin-center after:rounded-full after:bg-foreground/70 after:transition-[opacity,scale] after:duration-150 after:ease-out transition-[color,scale] duration-150 ease-out hover:text-foreground focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50 active:scale-[0.96]",
              active
                ? "text-foreground after:scale-100 after:opacity-100"
                : "after:scale-x-75 after:opacity-0",
            )}
          >
            <span>{scope.label}</span>
          </button>
        );
      })}
    </div>
  );
}
