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
export function BuiltinPackageSelector({
  editor,
  selected,
  select,
  onSaved,
}: BuiltinPackageSelection) {
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
      <p className="text-xs text-muted-foreground">{t("settings:builtinMcp.taskTiming")}</p>
      {editor.view?.can_edit && (
        <div className="sticky bottom-0 flex flex-wrap gap-2 border-t bg-background py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
          <Button
            data-testid="builtin-mcp-save"
            className={controlSizingClassName("standard")}
            disabled={!editor.dirty || editor.pending || Boolean(editor.error)}
            onClick={() => {
              void editor
                .save()
                .then(onSaved)
                .catch(() => undefined);
            }}
          >
            {t("settings:builtinMcp.save")}
          </Button>
          <Button
            variant="outline"
            className={controlSizingClassName("standard")}
            disabled={!editor.dirty || editor.pending}
            onClick={editor.discard}
          >
            {t("settings:builtinMcp.discard")}
          </Button>
          <Button
            variant="ghost"
            className={controlSizingClassName("standard")}
            disabled={editor.pending}
            onClick={editor.reset}
          >
            {t("settings:builtinMcp.reset")}
          </Button>
        </div>
      )}
    </div>
  );
}
