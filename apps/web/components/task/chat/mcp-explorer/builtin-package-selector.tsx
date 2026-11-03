import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  BuiltinPackageDetail,
  BuiltinPackageList,
  type BuiltinMCPEditor,
} from "@/components/mcp/builtin-package-detail";
import type { BuiltinMCPPackage } from "@/lib/types/builtin-mcp";

export type BuiltinPackageSelection = {
  editor: BuiltinMCPEditor;
  selected: BuiltinMCPPackage | null;
  select: (id: BuiltinMCPPackage | null) => void;
  onSaved: () => void;
};
export function BuiltinPackageSelector({ editor, selected, select }: BuiltinPackageSelection) {
  const { t } = useTranslation();
  const returnFocus = useRef<BuiltinMCPPackage | null>(null);
  if (selected) returnFocus.current = selected;
  const item = editor.view?.catalog.find((entry) => entry.id === selected);
  return (
    <div className="space-y-4 pb-5" data-testid="builtin-mcp-task-selector">
      {item ? (
        <BuiltinPackageDetail
          editor={editor}
          item={item}
          scope="tasks"
          onBack={() => select(null)}
        />
      ) : (
        <BuiltinPackageList returnFocus={returnFocus.current} editor={editor} onSelect={select} />
      )}
      {!item && (
        <p className="text-xs text-muted-foreground">{t("settings:builtinMcp.taskTiming")}</p>
      )}
      {!item && editor.view?.can_edit && (
        <Button
          type="button"
          variant="ghost"
          className={controlSizingClassName("standard")}
          disabled={editor.pending}
          onClick={editor.reset}
        >
          {t("settings:builtinMcp.reset")}
        </Button>
      )}
    </div>
  );
}
