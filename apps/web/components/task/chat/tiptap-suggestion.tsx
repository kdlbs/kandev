"use client";

import { PluginKey } from "@tiptap/pm/state";
import type {
  SuggestionOptions,
  SuggestionProps,
  SuggestionKeyDownProps,
} from "@tiptap/suggestion";
import type { MentionItem } from "@/hooks/use-inline-mention";
import type { SlashCommand } from "@/hooks/use-inline-slash";

import { getFileName } from "@/lib/utils/file-path";
import type { MentionKind } from "./tiptap-mention-extension";

// ── Shared types ────────────────────────────────────────────────────

export type MenuState<T> = {
  isOpen: boolean;
  items: T[];
  query: string;
  clientRect: (() => DOMRect | null) | null;
  command: ((item: T) => void) | null;
};

const EMPTY_MENTION_STATE: MenuState<MentionItem> = {
  isOpen: false,
  items: [],
  query: "",
  clientRect: null,
  command: null,
};

const EMPTY_SLASH_STATE: MenuState<SlashCommand> = {
  isOpen: false,
  items: [],
  query: "",
  clientRect: null,
  command: null,
};

// ── Mention suggestion ──────────────────────────────────────────────

export type MentionSuggestionCallbacks = {
  getItems: (query: string) => Promise<MentionItem[]>;
};

export const MentionSuggestionPluginKey = new PluginKey("mentionSuggestion");

/**
 * Creates a mention suggestion config for TipTap.
 * `setMenuState` drives the React rendering of MentionMenu.
 * `onKeyDown` is a stable callback the parent uses to delegate keystrokes.
 */
export function createMentionSuggestion(
  callbacks: MentionSuggestionCallbacks,
  setMenuState: (state: MenuState<MentionItem>) => void,
  onKeyDown: (event: KeyboardEvent) => boolean,
): Partial<
  SuggestionOptions<MentionItem, { id: string; label: string; kind: MentionKind; path: string }>
> {
  return {
    char: "@",
    pluginKey: MentionSuggestionPluginKey,
    allowSpaces: false,

    items: async ({ query }) => {
      return callbacks.getItems(query);
    },

    command: ({ editor, range, props: mentionAttrs }) => {
      editor
        .chain()
        .focus()
        .insertContentAt(range, [
          { type: "contextMention", attrs: mentionAttrs },
          { type: "text", text: " " },
        ])
        .run();
    },

    render: () => {
      return {
        onStart(props: SuggestionProps<MentionItem>) {
          setMenuState({
            isOpen: true,
            items: props.items,
            query: props.query,
            clientRect: props.clientRect ?? null,
            command: (item: MentionItem) => {
              props.command(mentionItemToAttrs(item));
            },
          });
        },

        onUpdate(props: SuggestionProps<MentionItem>) {
          setMenuState({
            isOpen: true,
            items: props.items,
            query: props.query,
            clientRect: props.clientRect ?? null,
            command: (item: MentionItem) => {
              props.command(mentionItemToAttrs(item));
            },
          });
        },

        onKeyDown(kd: SuggestionKeyDownProps) {
          if (kd.event.key === "Escape") {
            setMenuState(EMPTY_MENTION_STATE);
            return true;
          }
          return onKeyDown(kd.event);
        },

        onExit() {
          setMenuState(EMPTY_MENTION_STATE);
        },
      };
    },
  };
}

function mentionItemToAttrs(item: MentionItem): {
  id: string;
  label: string;
  kind: MentionKind;
  path: string;
} {
  const name = item.kind === "file" ? getFileName(item.label) : item.label;

  return {
    id: item.id,
    label: name,
    kind: item.kind,
    path: (() => {
      if (item.kind === "file") return item.label;
      if (item.kind === "prompt") return `prompt:${item.id}`;
      return "plan:context";
    })(),
  };
}

// ── Slash command suggestion ────────────────────────────────────────

export type SlashSuggestionCallbacks = {
  getCommands: () => SlashCommand[];
  onAgentCommand: (commandName: string) => void;
};

export const SlashSuggestionPluginKey = new PluginKey("slashSuggestion");

export function createSlashSuggestion(
  callbacks: SlashSuggestionCallbacks,
  setMenuState: (state: MenuState<SlashCommand>) => void,
  onKeyDown: (event: KeyboardEvent) => boolean,
): Partial<SuggestionOptions<SlashCommand>> {
  return {
    char: "/",
    pluginKey: SlashSuggestionPluginKey,
    allowSpaces: false,

    items: ({ query }) => {
      const allCommands = callbacks.getCommands();
      if (!query) return allCommands;

      const lq = query.toLowerCase();
      return allCommands
        .filter((cmd) => {
          const name = cmd.agentCommandName?.toLowerCase();
          return name?.startsWith(lq) || cmd.label.toLowerCase().includes(lq);
        })
        .sort((a, b) => {
          const an = a.agentCommandName?.toLowerCase();
          const bn = b.agentCommandName?.toLowerCase();
          const aPre = an?.startsWith(lq) ?? false;
          const bPre = bn?.startsWith(lq) ?? false;
          if (aPre && !bPre) return -1;
          if (!aPre && bPre) return 1;
          return 0;
        });
    },

    command: ({ editor, range, props: cmd }) => {
      editor.chain().focus().deleteRange(range).run();
      if (cmd.agentCommandName) {
        callbacks.onAgentCommand(cmd.agentCommandName);
      }
    },

    render: () => {
      return {
        onStart(props: SuggestionProps<SlashCommand>) {
          if (props.items.length === 0) return;
          setMenuState({
            isOpen: true,
            items: props.items,
            query: props.query,
            clientRect: props.clientRect ?? null,
            command: (cmd: SlashCommand) => props.command(cmd),
          });
        },

        onUpdate(props: SuggestionProps<SlashCommand>) {
          if (props.items.length === 0) {
            setMenuState(EMPTY_SLASH_STATE);
            return;
          }
          setMenuState({
            isOpen: true,
            items: props.items,
            query: props.query,
            clientRect: props.clientRect ?? null,
            command: (cmd: SlashCommand) => props.command(cmd),
          });
        },

        onKeyDown(kd: SuggestionKeyDownProps) {
          if (kd.event.key === "Escape") {
            setMenuState(EMPTY_SLASH_STATE);
            return true;
          }
          return onKeyDown(kd.event);
        },

        onExit() {
          setMenuState(EMPTY_SLASH_STATE);
        },
      };
    },
  };
}
