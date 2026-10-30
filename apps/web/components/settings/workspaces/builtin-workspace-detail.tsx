import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  BuiltinPackageDetail,
  type BuiltinMCPEditor,
} from "@/components/mcp/builtin-package-detail";
import type { BuiltinMCPDescriptor } from "@/lib/types/builtin-mcp";
export function BuiltinWorkspaceDetail({
  editor,
  item,
  onBack,
}: {
  editor: BuiltinMCPEditor;
  item: BuiltinMCPDescriptor;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <BuiltinPackageDetail editor={editor} item={item} scope="workspaces" onBack={onBack} />
      <Button
        variant="outline"
        className={controlSizingClassName("standard")}
        disabled={!editor.view?.can_edit || editor.pending}
        onClick={editor.reset}
      >
        {t("settings:builtinMcp.reset")}
      </Button>
    </div>
  );
}
