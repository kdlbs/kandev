import type { ReactNode } from "react";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";

export type CommandPanelMode = "commands" | "search-tasks" | "input";

export type CommandItem = {
  id: string;
  label: string;
  group: string;
  icon?: ReactNode;
  shortcut?: KeyboardShortcut;
  keywords?: string[];
  /** For level-2 transitions: set the mode instead of running an action */
  enterMode?: CommandPanelMode;
  /** Standard action — close panel and execute */
  action?: () => void;
  /** For 'input' mode: placeholder text for the input field */
  inputPlaceholder?: string;
  /** For 'input' mode: called with the input value when Enter is pressed */
  onInputSubmit?: (value: string) => void | Promise<void>;
  /** Lower values appear first. Page-specific = 0, global = 100. Default: 100 */
  priority?: number;
};
