"use client";

import { IconCommand, IconFiles, IconFileSearch } from "@tabler/icons-react";
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
  Icon: typeof IconCommand;
};

const SCOPE_OPTIONS: ScopeOption[] = [
  { mode: "commands", label: "Commands", shortcutId: "SEARCH", Icon: IconCommand },
  { mode: "search-files", label: "Files", shortcutId: "FILE_SEARCH", Icon: IconFiles },
  {
    mode: "search-content",
    label: "Contents",
    shortcutId: "CONTENT_SEARCH",
    Icon: IconFileSearch,
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
  const activeIndex = SCOPE_OPTIONS.findIndex((scope) => scope.mode === mode);

  return (
    <div
      role="tablist"
      aria-label="Command palette mode"
      className="relative isolate mx-1 mb-1 mt-0.5 grid grid-cols-3 rounded-xl bg-muted/60 p-1"
    >
      <div
        aria-hidden="true"
        data-testid="command-panel-scope-indicator"
        data-scope={mode}
        className="pointer-events-none absolute inset-y-1 left-1 z-0 w-[calc((100%-0.5rem)/3)] rounded-lg bg-background shadow-[0_1px_2px_rgb(0_0_0_/_0.06),0_0_0_1px_rgb(0_0_0_/_0.05)] transition-transform duration-150 ease-out dark:shadow-[0_0_0_1px_rgb(255_255_255_/_0.08)]"
        style={{ transform: `translateX(${activeIndex * 100}%)` }}
      />
      {SCOPE_OPTIONS.map((scope) => {
        const active = mode === scope.mode;
        const shortcut = formatShortcut(getShortcut(scope.shortcutId, keyboardShortcuts));
        const ScopeIcon = scope.Icon;
        return (
          <button
            key={scope.mode}
            type="button"
            role="tab"
            aria-label={scope.label}
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            title={`${scope.label} (${shortcut})`}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => onScopeChange(scope.mode)}
            className={cn(
              "relative z-10 flex min-h-10 cursor-pointer items-center justify-center gap-1.5 rounded-lg px-3 text-xs font-medium text-muted-foreground outline-none transition-[color,scale] duration-150 ease-out hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 active:scale-[0.96]",
              active && "text-foreground",
            )}
          >
            <ScopeIcon className="size-3.5 shrink-0" stroke={1.75} />
            <span>{scope.label}</span>
          </button>
        );
      })}
    </div>
  );
}
