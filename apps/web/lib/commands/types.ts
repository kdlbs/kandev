import type { ReactNode } from 'react';
import type { KeyboardShortcut } from '@/lib/keyboard/constants';

export type CommandPanelMode = 'commands' | 'search-tasks';

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
  /** Lower values appear first. Page-specific = 0, global = 100. Default: 100 */
  priority?: number;
};
