import { PluginKey } from "@tiptap/pm/state";
import { exitSuggestion } from "@tiptap/suggestion";
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

type EntityReferenceSuggestionRange = {
  from: number;
  to: number;
};

type EntityReferenceSuggestionTransaction = {
  docChanged?: boolean;
  getMeta: (key: string) => unknown;
};

type PendingEntityReferenceInput = {
  from: number;
  text: string;
};

export type EntityReferenceInputGate = {
  recordTextInput: (from: number, to: number, text: string) => void;
  recordDeletion: () => void;
  shouldShow: (
    range: EntityReferenceSuggestionRange,
    transaction: EntityReferenceSuggestionTransaction,
  ) => boolean;
  reset: () => void;
};

function isExternalEntityReferenceTransaction(
  transaction: EntityReferenceSuggestionTransaction,
): boolean {
  const uiEvent = transaction.getMeta("uiEvent");
  return (
    uiEvent === "paste" ||
    uiEvent === "drop" ||
    transaction.getMeta("paste") === true ||
    transaction.getMeta("drop") === true
  );
}

function isActiveEntityReferenceRange(
  range: EntityReferenceSuggestionRange,
  activeRange: EntityReferenceSuggestionRange | null,
): boolean {
  return activeRange !== null && range.from === activeRange.from && range.to >= range.from + 1;
}

function matchesDirectEntityReferenceInput(
  range: EntityReferenceSuggestionRange,
  input: PendingEntityReferenceInput,
  activeRange: EntityReferenceSuggestionRange | null,
): boolean {
  const hashOffset = input.text.lastIndexOf("#");
  const triggerFrom = hashOffset >= 0 ? input.from + hashOffset : null;
  const isDirectNewTrigger =
    triggerFrom !== null && range.from === triggerFrom && range.to >= triggerFrom + 1;
  const isDirectContinuation = isActiveEntityReferenceRange(range, activeRange);
  return isDirectNewTrigger || isDirectContinuation;
}

/**
 * Tracks the direct text input that can create or extend a # suggestion.
 * ProseMirror does not mark every programmatic transaction with an origin, so
 * requiring a matching handleTextInput callback keeps restored and pasted
 * content literal without changing the editor's normal insertion behavior.
 */
export function createEntityReferenceInputGate(): EntityReferenceInputGate {
  let activeRange: EntityReferenceSuggestionRange | null = null;
  let pendingInput: PendingEntityReferenceInput | null = null;
  let pendingDeletion = false;

  const reset = () => {
    activeRange = null;
    pendingInput = null;
    pendingDeletion = false;
  };

  const recordTextInput = (from: number, _to: number, text: string) => {
    pendingInput = { from, text };
    pendingDeletion = false;
  };

  const recordDeletion = () => {
    pendingInput = null;
    pendingDeletion = true;
  };

  const shouldShow = (
    range: EntityReferenceSuggestionRange,
    transaction: EntityReferenceSuggestionTransaction,
  ) => {
    if (isExternalEntityReferenceTransaction(transaction)) {
      reset();
      return false;
    }

    const input = pendingInput;
    const deletion = pendingDeletion;
    pendingInput = null;
    pendingDeletion = false;

    // A document-changing transaction without a matching text-input callback
    // is a paste, draft restore, history recall, or programmatic update.
    if (!input && !deletion && transaction.docChanged) {
      reset();
      return false;
    }

    const isActiveRange = isActiveEntityReferenceRange(range, activeRange);
    if (deletion) {
      if (!isActiveRange) {
        reset();
        return false;
      }
      activeRange = { ...range };
      return true;
    }
    if (!input) {
      return Boolean(isActiveRange);
    }

    if (!matchesDirectEntityReferenceInput(range, input, activeRange)) {
      reset();
      return false;
    }

    activeRange = { ...range };
    return true;
  };

  return { recordTextInput, recordDeletion, shouldShow, reset };
}

export function isEntityReferenceQueryAllowed(query: string): boolean {
  return query.length === 0 || !/^\s/u.test(query);
}

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
  inputGate: EntityReferenceInputGate = createEntityReferenceInputGate(),
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
    shouldShow: ({ range, query, transaction }) => {
      if (!isEntityReferenceQueryAllowed(query)) {
        inputGate.reset();
        return false;
      }
      return inputGate.shouldShow(range, transaction);
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
            // Claim the key here: once the entity-reference popup is closed,
            // nothing further up the tree (e.g. a clarification panel's own
            // Escape-collapses handler) should also react to the same
            // keypress.
            keyDown.event.stopPropagation();
            setMenuState(EMPTY_ENTITY_REFERENCE_STATE);
            inputGate.reset();
            exitSuggestion(keyDown.view, EntityReferenceSuggestionPluginKey);
            return true;
          }
          return onKeyDown(keyDown.event);
        },
        onExit() {
          inputGate.reset();
          setMenuState(EMPTY_ENTITY_REFERENCE_STATE);
        },
      };
    },
  };
}
