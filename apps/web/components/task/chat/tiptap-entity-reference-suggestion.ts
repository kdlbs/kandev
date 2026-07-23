import { PluginKey } from "@tiptap/pm/state";
import type {
  SuggestionKeyDownProps,
  SuggestionOptions,
  SuggestionProps,
} from "@tiptap/suggestion";
import type { EntityReference } from "@/lib/types/entity-reference";
import { isEntityReferenceTriggerAllowed } from "./tiptap-helpers";
import type { MenuState } from "./tiptap-suggestion";

const EMPTY_ENTITY_REFERENCE_STATE: MenuState<EntityReference> = {
  isOpen: false,
  items: [],
  query: "",
  clientRect: null,
  command: null,
};

export const EntityReferenceSuggestionPluginKey = new PluginKey("entityReferenceSuggestion");

export function handleEntityReferenceMenuKeyDown(args: {
  event: KeyboardEvent;
  items: readonly EntityReference[];
  selectedIndex: number;
  setSelectedIndex: (index: number) => void;
  onSelect: (reference: EntityReference) => void;
}): boolean {
  if (args.event.key === "ArrowDown" || args.event.key === "ArrowUp") {
    if (args.items.length > 0) {
      const delta = args.event.key === "ArrowDown" ? 1 : -1;
      const next = Math.min(Math.max(args.selectedIndex + delta, 0), args.items.length - 1);
      args.setSelectedIndex(next);
    }
    return true;
  }
  if (args.event.key !== "Enter" && args.event.key !== "Tab") return false;
  const item = args.items[args.selectedIndex];
  if (!item) return false;
  args.onSelect(item);
  return true;
}

export function createEntityReferenceSuggestion(
  setMenuState: (state: MenuState<EntityReference>) => void,
  onKeyDown: (event: KeyboardEvent) => boolean,
): Partial<SuggestionOptions<EntityReference>> {
  return {
    char: "#",
    pluginKey: EntityReferenceSuggestionPluginKey,
    allowSpaces: true,
    allowedPrefixes: null,
    allow: ({ state, range }) => {
      const $from = state.doc.resolve(range.from);
      const triggerOffset = range.from - $from.start();
      return isEntityReferenceTriggerAllowed({
        textBeforeTrigger: $from.parent.textBetween(0, triggerOffset, "\0", "\0"),
        parentType: $from.parent.type.name,
        hasCodeMark: $from.marks().some((mark) => mark.type.name === "code"),
      });
    },
    items: () => [],
    command: ({ editor, range, props }) => {
      editor
        .chain()
        .focus()
        .insertContentAt(range, [
          { type: "entityReference", attrs: props },
          { type: "text", text: " " },
        ])
        .run();
    },
    render: () => {
      const updateMenu = (props: SuggestionProps<EntityReference>) => {
        setMenuState({
          isOpen: true,
          items: [],
          query: props.query,
          clientRect: props.clientRect ?? null,
          command: (reference) => props.command(reference),
        });
      };
      return {
        onStart: updateMenu,
        onUpdate: updateMenu,
        onKeyDown(keyDown: SuggestionKeyDownProps) {
          if (keyDown.event.key === "Escape") {
            setMenuState(EMPTY_ENTITY_REFERENCE_STATE);
            return true;
          }
          return onKeyDown(keyDown.event);
        },
        onExit() {
          setMenuState(EMPTY_ENTITY_REFERENCE_STATE);
        },
      };
    },
  };
}
