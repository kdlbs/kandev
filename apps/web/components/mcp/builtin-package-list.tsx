import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { IconChevronRight } from "@tabler/icons-react";
import type { BuiltinMCPDescriptor, BuiltinMCPPackage } from "@/lib/types/builtin-mcp";
import type { BuiltinMCPEditor } from "./builtin-package-detail";
import { ApiError } from "@/lib/api/client";
import { builtinPackageChoice } from "./builtin-package-choice";

function PackageRow({
  editor,
  item,
  scope,
  onSelect,
}: {
  editor: BuiltinMCPEditor;
  item: BuiltinMCPDescriptor;
  scope: "tasks" | "workspaces";
  onSelect: (id: BuiltinMCPPackage) => void;
}) {
  const { t } = useTranslation();
  const state = builtinPackageChoice(editor.view, editor.draft, scope, item.id);
  return (
    <Button
      type="button"
      variant="ghost"
      data-testid={`builtin-mcp-package-${item.id}`}
      data-settings-dirty={state.dirty}
      data-settings-dirty-level="container"
      onClick={() => onSelect(item.id)}
      className="h-auto max-md:h-auto [@media(pointer:coarse)]:h-auto min-h-14 w-full cursor-pointer justify-start gap-3 whitespace-normal rounded-none border border-transparent p-3 text-left"
    >
      <span className="min-w-0 flex-1">
        <span className="block font-medium">{t(item.label_key)}</span>
        <span className="block break-words text-xs font-normal text-muted-foreground">
          {t(item.description_key)}
        </span>
        {state.inherited && (
          <span className="block text-xs font-normal text-muted-foreground">
            {t("settings:builtinMcp.choices.inherit")}
          </span>
        )}
      </span>
      <Badge variant="secondary">{t(state.labelKey)}</Badge>
      <IconChevronRight className="size-4 shrink-0" aria-hidden />
    </Button>
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
