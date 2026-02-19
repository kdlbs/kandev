import { Extension } from "@tiptap/core";
import { PluginKey } from "@tiptap/pm/state";
import Suggestion from "@tiptap/suggestion";
import type { SuggestionProps, SuggestionKeyDownProps } from "@tiptap/suggestion";
import type { Editor, Range } from "@tiptap/core";
import type { Icon } from "@tabler/icons-react";
import {
  IconH1,
  IconH2,
  IconH3,
  IconList,
  IconListNumbers,
  IconListCheck,
  IconCode,
  IconQuote,
  IconTable,
  IconMinus,
} from "@tabler/icons-react";
import type { MenuState } from "@/components/task/chat/tiptap-suggestion";

// ── Types ────────────────────────────────────────────────────────────

export type PlanSlashCommand = {
  id: string;
  label: string;
  description: string;
  icon: Icon;
  category: string;
  action: (editor: Editor, range: Range) => void;
};

// ── Command definitions ──────────────────────────────────────────────

const PLAN_SLASH_COMMANDS: PlanSlashCommand[] = [
  // Text
  {
    id: "heading1",
    label: "Heading 1",
    description: "Large heading",
    icon: IconH1,
    category: "Text",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleHeading({ level: 1 }).run();
    },
  },
  {
    id: "heading2",
    label: "Heading 2",
    description: "Medium heading",
    icon: IconH2,
    category: "Text",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleHeading({ level: 2 }).run();
    },
  },
  {
    id: "heading3",
    label: "Heading 3",
    description: "Small heading",
    icon: IconH3,
    category: "Text",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleHeading({ level: 3 }).run();
    },
  },
  // Lists
  {
    id: "bulletList",
    label: "Bullet List",
    description: "Unordered list",
    icon: IconList,
    category: "Lists",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleBulletList().run();
    },
  },
  {
    id: "numberedList",
    label: "Numbered List",
    description: "Ordered list",
    icon: IconListNumbers,
    category: "Lists",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleOrderedList().run();
    },
  },
  {
    id: "taskList",
    label: "Task List",
    description: "Checklist items",
    icon: IconListCheck,
    category: "Lists",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleTaskList().run();
    },
  },
  // Blocks
  {
    id: "codeBlock",
    label: "Code Block",
    description: "Fenced code",
    icon: IconCode,
    category: "Blocks",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleCodeBlock().run();
    },
  },
  {
    id: "blockquote",
    label: "Blockquote",
    description: "Quote text",
    icon: IconQuote,
    category: "Blocks",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleBlockquote().run();
    },
  },
  {
    id: "table",
    label: "Table",
    description: "3x3 table",
    icon: IconTable,
    category: "Blocks",
    action: (editor, range) => {
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertTable({ rows: 3, cols: 3, withHeaderRow: true })
        .run();
    },
  },
  {
    id: "horizontalRule",
    label: "Divider",
    description: "Horizontal line",
    icon: IconMinus,
    category: "Blocks",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).setHorizontalRule().run();
    },
  },
];

// ── Empty state ──────────────────────────────────────────────────────

const EMPTY_STATE: MenuState<PlanSlashCommand> = {
  isOpen: false,
  items: [],
  query: "",
  clientRect: null,
  command: null,
};

// ── Extension ────────────────────────────────────────────────────────

const PlanSlashPluginKey = new PluginKey("planSlashCommands");

export function createPlanSlashExtension(
  setMenuState: (state: MenuState<PlanSlashCommand>) => void,
  onKeyDown: (event: KeyboardEvent) => boolean,
) {
  return Extension.create({
    name: "planSlashCommands",

    addProseMirrorPlugins() {
      return [
        Suggestion({
          editor: this.editor,
          char: "/",
          pluginKey: PlanSlashPluginKey,
          allowSpaces: false,
          startOfLine: true,

          items: ({ query }) => {
            if (!query) return PLAN_SLASH_COMMANDS;
            const lq = query.toLowerCase();
            return PLAN_SLASH_COMMANDS.filter(
              (cmd) => cmd.label.toLowerCase().includes(lq) || cmd.id.toLowerCase().includes(lq),
            );
          },

          command: ({ editor, range, props: cmd }) => {
            cmd.action(editor, range);
          },

          render: () => ({
            onStart(props: SuggestionProps<PlanSlashCommand>) {
              if (props.items.length === 0) return;
              setMenuState({
                isOpen: true,
                items: props.items,
                query: props.query,
                clientRect: props.clientRect ?? null,
                command: (cmd) => props.command(cmd),
              });
            },

            onUpdate(props: SuggestionProps<PlanSlashCommand>) {
              if (props.items.length === 0) {
                setMenuState(EMPTY_STATE);
                return;
              }
              setMenuState({
                isOpen: true,
                items: props.items,
                query: props.query,
                clientRect: props.clientRect ?? null,
                command: (cmd) => props.command(cmd),
              });
            },

            onKeyDown(kd: SuggestionKeyDownProps) {
              if (kd.event.key === "Escape") {
                setMenuState(EMPTY_STATE);
                return true;
              }
              return onKeyDown(kd.event);
            },

            onExit() {
              setMenuState(EMPTY_STATE);
            },
          }),
        }),
      ];
    },
  });
}
