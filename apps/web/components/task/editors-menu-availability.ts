import type { EditorOption } from "@/lib/types/http";

const EMBEDDED_VSCODE_KIND = "internal_vscode";

export function getAvailableTaskTopbarEditors(
  editors: EditorOption[],
  embeddedVscodeSupported = false,
): EditorOption[] {
  return editors.filter((editor) => {
    if (!editor.enabled) return false;
    if (editor.kind === "built_in" && !editor.installed) return false;
    return editor.kind !== EMBEDDED_VSCODE_KIND || embeddedVscodeSupported;
  });
}

export function resolveTaskTopbarEditorId(
  defaultEditorId: string | null | undefined,
  editors: EditorOption[],
): string {
  if (defaultEditorId && editors.some((editor) => editor.id === defaultEditorId)) {
    return defaultEditorId;
  }
  return editors[0]?.id ?? "";
}
