import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import type { BuiltinMCPDescriptor, BuiltinMCPPackage } from "@/lib/types/builtin-mcp";
import type { BuiltinMCPEditor } from "./builtin-package-detail";
import { ApiError } from "@/lib/api/client";
import { BuiltinPackageRow } from "./builtin-package-content";
import {
  TaskPackageSwitch,
  TaskPackageInheritance,
  WorkspacePackageControls,
} from "./builtin-package-controls";
import { builtinPackageChoice, builtinPackageRestriction } from "./builtin-package-choice";

type PackageRowProps = {
  editor: BuiltinMCPEditor;
  item: BuiltinMCPDescriptor;
  scope: "tasks" | "workspaces";
};

function PackageRowNotes({ editor, item, scope }: PackageRowProps) {
  const { t } = useTranslation();
  const restriction = builtinPackageRestriction(editor.view, item);
  if (restriction)
    return item.id === "core" ? null : (
      <p className="text-xs text-muted-foreground">{t(restriction)}</p>
    );
  if (scope === "workspaces") return <WorkspacePackageControls editor={editor} item={item} />;
  return (
    <>
      <TaskPackageInheritance editor={editor} item={item} />
      {editor.view?.policy.packages[item.id] === "blocked" && (
        <p id={`builtin-mcp-blocked-${item.id}`} className="text-xs text-muted-foreground">
          {t("settings:builtinMcp.blocked")}
        </p>
      )}
      {item.id === "configuration" && (
        <p className="text-xs text-muted-foreground">
          {t("settings:builtinMcp.configurationAuthority")}
        </p>
      )}
    </>
  );
}

function PackageRow({
  editor,
  item,
  scope,
  onSelect,
}: PackageRowProps & { onSelect: (id: BuiltinMCPPackage) => void }) {
  const state = builtinPackageChoice(editor.view, editor.draft, scope, item.id);
  const restriction = builtinPackageRestriction(editor.view, item);
  return (
    <BuiltinPackageRow
      item={item}
      labelKey={state.labelKey}
      dirty={state.dirty}
      control={
        scope === "tasks" && !restriction ? (
          <TaskPackageSwitch editor={editor} item={item} compact />
        ) : undefined
      }
      onSelect={() => onSelect(item.id)}
    >
      <PackageRowNotes editor={editor} item={item} scope={scope} />
    </BuiltinPackageRow>
  );
}

export function BuiltinMCPError({ editor }: { editor: BuiltinMCPEditor }) {
  const { t } = useTranslation();
  if (!editor.error) return null;
  const code = editor.error instanceof ApiError ? editor.error.status : 0;
  let errorKey = "settings:builtinMcp.loadFailed";
  if (code === 409) errorKey = "settings:builtinMcp.conflict";
  if (code === 403) errorKey = "settings:builtinMcp.denied";
  return (
    <div role="alert" className="space-y-2 rounded-md border border-destructive/40 p-3 text-sm">
      <p>{t(errorKey)}</p>
      <Button
        className={controlSizingClassName("standard")}
        variant="outline"
        onClick={() => void editor.reload()}
        disabled={editor.pending}
      >
        {t("settings:builtinMcp.reload")}
      </Button>
    </div>
  );
}

export function BuiltinPackageList({
  editor,
  onSelect,
  returnFocus,
  scope = "tasks",
}: {
  editor: BuiltinMCPEditor;
  returnFocus?: BuiltinMCPPackage | null;
  scope?: "tasks" | "workspaces";
  onSelect: (id: BuiltinMCPPackage) => void;
}) {
  const { t } = useTranslation();
  const root = useRef<HTMLElement>(null);
  useEffect(() => {
    if (returnFocus)
      root.current
        ?.querySelector<HTMLButtonElement>(`[data-testid="builtin-mcp-package-${returnFocus}"]`)
        ?.focus();
  }, [returnFocus]);
  return (
    <section ref={root} className="min-w-0 space-y-3" data-testid="builtin-mcp-packages">
      <h2 className="text-base font-semibold">{t("settings:builtinMcp.title")}</h2>
      <p className="text-sm text-muted-foreground">{t("settings:builtinMcp.packageSummary")}</p>
      {scope === "workspaces" && (
        <p className="text-xs text-muted-foreground">{t("settings:builtinMcp.workspaceTiming")}</p>
      )}
      <BuiltinMCPError editor={editor} />
      {editor.pending && <p role="status">{t("common:loading")}</p>}
      <div
        className="divide-y rounded-lg border"
        data-settings-dirty={
          editor.view?.catalog.some(
            (item) => builtinPackageChoice(editor.view, editor.draft, scope, item.id).dirty,
          ) ?? false
        }
        data-settings-dirty-level="card"
      >
        {editor.view?.catalog.map((item) => (
          <PackageRow key={item.id} editor={editor} item={item} scope={scope} onSelect={onSelect} />
        ))}
      </div>
    </section>
  );
}
