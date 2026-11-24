import { act } from "@testing-library/react";
import type { Editor } from "@tiptap/core";
import { buildTaskPromptDocument } from "@/lib/prompts/task-prompt-document";

export function setMoveInstructions(input: HTMLElement, value: string) {
  const { editor } = input as HTMLElement & { editor: Editor };
  act(() => {
    editor.commands.setContent(buildTaskPromptDocument(value, []));
    editor.commands.setTextSelection(editor.state.doc.content.size - 1);
  });
}

export function readMoveInstructions(input: HTMLElement) {
  return input.getAttribute("data-task-description-value");
}
