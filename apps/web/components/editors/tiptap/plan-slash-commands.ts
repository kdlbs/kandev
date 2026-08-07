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
import type { TFunction } from "i18next";

// ── Types ────────────────────────────────────────────────────────────

/**
 * Stable grouping id, not display copy: it is the `Map` key the menu groups by.
 * `plan-slash-menu.tsx` resolves it to a translated heading at render.
 */
export type PlanSlashCategory = "text" | "lists" | "blocks";

export type PlanSlashCommand = {
  id: string;
  label: string;
  description: string;
  icon: Icon;
  category: PlanSlashCategory;
  action: (editor: Editor, range: Range) => void;
};

// ── Command definitions ──────────────────────────────────────────────

const textCommands = (t: TFunction): PlanSlashCommand[] => [
  {
    id: "heading1",
    label: t("editors:slashHeading1"),
    description: t("editors:slashHeading1Description"),
    icon: IconH1,
    category: "text",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleHeading({ level: 1 }).run();
    },
  },
  {
    id: "heading2",
    label: t("editors:slashHeading2"),
    description: t("editors:slashHeading2Description"),
    icon: IconH2,
    category: "text",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleHeading({ level: 2 }).run();
    },
  },
  {
    id: "heading3",
    label: t("editors:slashHeading3"),
    description: t("editors:slashHeading3Description"),
    icon: IconH3,
    category: "text",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleHeading({ level: 3 }).run();
    },
  },
];

const listCommands = (t: TFunction): PlanSlashCommand[] => [
  {
    id: "bulletList",
    label: t("editors:slashBulletList"),
    description: t("editors:slashBulletListDescription"),
    icon: IconList,
    category: "lists",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleBulletList().run();
    },
  },
  {
    id: "numberedList",
    label: t("editors:slashNumberedList"),
    description: t("editors:slashNumberedListDescription"),
    icon: IconListNumbers,
    category: "lists",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleOrderedList().run();
    },
  },
  {
    id: "taskList",
    label: t("editors:slashTaskList"),
    description: t("editors:slashTaskListDescription"),
    icon: IconListCheck,
    category: "lists",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleTaskList().run();
    },
  },
];

const blockCommands = (t: TFunction): PlanSlashCommand[] => [
  {
    id: "codeBlock",
    label: t("editors:slashCodeBlock"),
    description: t("editors:slashCodeBlockDescription"),
    icon: IconCode,
    category: "blocks",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleCodeBlock().run();
    },
  },
  {
    id: "blockquote",
    label: t("editors:slashBlockquote"),
    description: t("editors:slashBlockquoteDescription"),
    icon: IconQuote,
    category: "blocks",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).toggleBlockquote().run();
    },
  },
  {
    id: "table",
    label: t("editors:slashTable"),
    description: t("editors:slashTableDescription"),
    icon: IconTable,
    category: "blocks",
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
    label: t("editors:slashDivider"),
    description: t("editors:slashDividerDescription"),
    icon: IconMinus,
    category: "blocks",
    action: (editor, range) => {
      editor.chain().focus().deleteRange(range).setHorizontalRule().run();
    },
  },
];

/** Exported for unit testing — see `plan-slash-commands.test.ts`. */
export const buildPlanSlashCommands = (t: TFunction): PlanSlashCommand[] => [
  ...textCommands(t),
  ...listCommands(t),
  ...blockCommands(t),
];

/**
 * Matches on the translated label and on the stable `id`, so the English command
 * name keeps working as a query under a non-English locale.
 *
 * Exported for unit testing — see `plan-slash-commands.test.ts`.
 */
export function filterPlanSlashCommands(commands: PlanSlashCommand[], query: string) {
  if (!query) return commands;
  const lq = query.toLowerCase();
  return commands.filter(
    (cmd) => cmd.label.toLowerCase().includes(lq) || cmd.id.toLowerCase().includes(lq),
  );
}

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
  getT: () => TFunction,
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

          items: ({ query }) =>
            // Resolved per call, so the menu follows a locale switch without the
            // extension itself having to be rebuilt.
            filterPlanSlashCommands(buildPlanSlashCommands(getT()), query),

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
