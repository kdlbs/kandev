import { Extension, type Editor } from "@tiptap/core";

type PromptSuggestionStorage = { promptSuggestion?: { text: string } };

function suggestionStorage(editor: Editor): PromptSuggestionStorage {
  return editor.storage as unknown as PromptSuggestionStorage;
}

/** Holds the composer's next-prompt suggestion. The ghost text is a placeholder
 *  decoration, never document content, so it cannot leak into the draft. */
export const PromptSuggestion = Extension.create({
  name: "promptSuggestion",

  addStorage() {
    return { text: "" };
  },

  addKeyboardShortcuts() {
    return { Tab: () => acceptPromptSuggestion(this.editor) };
  },
});

/** The stored suggestion text, regardless of the draft. Read by the placeholder decoration. */
export function storedPromptSuggestion(editor: Editor): string {
  return suggestionStorage(editor).promptSuggestion?.text ?? "";
}

/** The suggestion when it is visible: editable editor, empty draft. */
export function visiblePromptSuggestion(editor: Editor | null): string | null {
  if (!editor || !editor.isEditable || !editor.isEmpty) return null;
  return storedPromptSuggestion(editor) || null;
}

export function updatePromptSuggestion(editor: Editor | null, text: string | null) {
  const storage = editor ? suggestionStorage(editor).promptSuggestion : undefined;
  if (!editor || !storage) return;
  const next = text?.trim() ?? "";
  if (storage.text === next) return;
  storage.text = next;
  editor.view.dispatch(editor.state.tr);
}

/** Tab: copy the visible suggestion into the draft as editable text. */
export function acceptPromptSuggestion(editor: Editor): boolean {
  const text = visiblePromptSuggestion(editor);
  if (!text) return false;
  editor.chain().setContent(text, { emitUpdate: true }).focus("end").run();
  return true;
}

/** Submit shortcut: put the visible suggestion in the draft so the normal submit path sends it. */
export function fillPromptSuggestionForSubmit(editor: Editor): boolean {
  const text = visiblePromptSuggestion(editor);
  if (!text) return false;
  editor.commands.setContent(text, { emitUpdate: true });
  return true;
}

/** Every submit control: an empty draft showing a suggestion sends that suggestion. */
export function submitWithPromptSuggestion(editor: Editor | null, submit: () => void): void {
  if (editor) fillPromptSuggestionForSubmit(editor);
  submit();
}
