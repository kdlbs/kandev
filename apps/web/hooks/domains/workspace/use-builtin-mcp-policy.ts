import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useBuiltinMCPManagement } from "./use-builtin-mcp-management";

export function useBuiltinMCPPolicy(workspaceId: string) {
  const editor = useBuiltinMCPManagement("workspaces", workspaceId, workspaceId);
  useSettingsSaveContributor({
    id: `builtin-mcp:${workspaceId}`,
    revision: JSON.stringify(editor.draft),
    isDirty: editor.dirty,
    canSave: Boolean(editor.view?.can_edit && !editor.pending && !editor.error),
    save: editor.save,
    discard: editor.discard,
  });
  return editor;
}
