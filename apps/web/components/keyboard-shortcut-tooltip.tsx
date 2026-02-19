/**
 * Reusable tooltip component for displaying keyboard shortcuts
 */

import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Kbd, KbdGroup } from "@kandev/ui/kbd";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import { formatShortcut, detectPlatform } from "@/lib/keyboard/utils";

type KeyboardShortcutTooltipProps = {
  shortcut: KeyboardShortcut;
  children: React.ReactNode;
  /** Additional description text to show alongside the shortcut */
  description?: string;
  /** Whether to show the tooltip (default: true) */
  enabled?: boolean;
};

/**
 * Tooltip that displays a keyboard shortcut hint
 *
 * @example
 * ```tsx
 * <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT}>
 *   <Button type="submit">Submit</Button>
 * </KeyboardShortcutTooltip>
 * ```
 */
export function KeyboardShortcutTooltip({
  shortcut,
  children,
  description,
  enabled = true,
}: KeyboardShortcutTooltipProps) {
  if (!enabled) {
    return <>{children}</>;
  }

  const platform = detectPlatform();
  const formattedShortcut = formatShortcut(shortcut, platform);
  const parts = formattedShortcut.split("+");

  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent>
        <div className="flex items-center gap-2">
          {description && <span>{description}</span>}
          <KbdGroup>
            {parts.map((part, index) => (
              <Kbd key={index}>{part}</Kbd>
            ))}
          </KbdGroup>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * Simple text-only version that just shows the formatted shortcut
 * Useful for inline display without tooltip
 *
 * @example
 * ```tsx
 * <KeyboardShortcutText shortcut={SHORTCUTS.SUBMIT} />
 * // Renders: Cmd+Enter (on Mac) or Ctrl+Enter (on Windows/Linux)
 * ```
 */
export function KeyboardShortcutText({ shortcut }: { shortcut: KeyboardShortcut }) {
  const platform = detectPlatform();
  return <span>{formatShortcut(shortcut, platform)}</span>;
}

/**
 * Kbd group version for displaying shortcuts with proper styling
 *
 * @example
 * ```tsx
 * <KeyboardShortcutKbd shortcut={SHORTCUTS.SUBMIT} />
 * ```
 */
export function KeyboardShortcutKbd({ shortcut }: { shortcut: KeyboardShortcut }) {
  const platform = detectPlatform();
  const formattedShortcut = formatShortcut(shortcut, platform);
  const parts = formattedShortcut.split("+");

  return (
    <KbdGroup>
      {parts.map((part, index) => (
        <Kbd key={index}>{part}</Kbd>
      ))}
    </KbdGroup>
  );
}
