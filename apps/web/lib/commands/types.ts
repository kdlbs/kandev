import type { ReactNode } from "react";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";

export type CommandPanelMode =
  | "commands"
  | "search-tasks"
  | "search-files"
  | "search-content"
  | "input";

export type CommandItem = {
  id: string;
  label: string;
  group: string;
  icon?: ReactNode;
  shortcut?: KeyboardShortcut;
  keywords?: string[];
  /** Secondary display text. Deliberately excluded from command search terms. */
  context?: string;
  /** Hidden from the idle palette; becomes eligible after the user types. */
  searchOnly?: boolean;
  /** For level-2 transitions: set the mode instead of running an action */
  enterMode?: CommandPanelMode;
  /** Standard action — close panel and execute */
  action?: () => void;
  /** Keep the palette mounted while an action-owned confirmation is shown. */
  keepOpen?: boolean;
  /** Confirmation content rendered inside the open palette. */
  confirmation?: ReactNode;
  /** Clears action-owned confirmation state when the palette itself closes. */
  onConfirmationDismiss?: () => void;
  /** For 'input' mode: placeholder text for the input field */
  inputPlaceholder?: string;
  /** For 'input' mode: called with the input value when Enter is pressed */
  onInputSubmit?: (value: string) => void | Promise<void>;
  /** Lower values appear first. Page-specific = 0, global = 100. Default: 100 */
  priority?: number;
};
