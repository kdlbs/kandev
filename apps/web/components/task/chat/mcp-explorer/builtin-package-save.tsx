import {
  SettingsFloatingSave,
  type SettingsSaveStatus,
} from "@/components/settings/settings-floating-save";
import type { BuiltinPackageSelection } from "./builtin-package-selector";

export function BuiltinPackageSave({
  editor,
  onSaved,
}: Pick<BuiltinPackageSelection, "editor" | "onSaved">) {
  if (!editor.view?.can_edit || !editor.dirty) return null;
  let status: SettingsSaveStatus = "dirty";
  if (editor.pending) status = "saving";
  if (editor.error) status = "error";
  return (
    <SettingsFloatingSave
      placement="panel"
      saveTestId="builtin-mcp-save"
      status={status}
      canSave={!editor.error && !editor.pending}
      errorKind={editor.error ? "save" : null}
      navigationIntent={null}
      isDiscarding={false}
      onSave={async () => {
        try {
          await editor.save();
          onSaved();
          return true;
        } catch {
          return false;
        }
      }}
      onReset={editor.discard}
      onDiscardAndLeave={editor.discard}
      onContinueEditing={() => undefined}
    />
  );
}
