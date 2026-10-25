import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { IconChevronRight } from "@tabler/icons-react";
import type { BuiltinMCPPackage } from "@/lib/types/builtin-mcp";
import type { BuiltinMCPEditor } from "./builtin-package-detail";
import { ApiError } from "@/lib/api/client";

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
}: {
  editor: BuiltinMCPEditor;
  returnFocus?: BuiltinMCPPackage | null;
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
      <p className="text-sm text-muted-foreground">{t("settings:builtinMcp.owner")}</p>
      <BuiltinMCPError editor={editor} />
      {editor.pending && <p role="status">{t("common:loading")}</p>}
      <div className="divide-y rounded-lg border">
        {editor.view?.catalog.map((item) => (
          <Button
            type="button"
            key={item.id}
            variant="ghost"
            data-testid={`builtin-mcp-package-${item.id}`}
            onClick={() => onSelect(item.id)}
            className="h-auto min-h-14 w-full cursor-pointer justify-start gap-3 whitespace-normal rounded-none p-3 text-left"
          >
            <span className="min-w-0 flex-1">
              <span className="block font-medium">{t(item.label_key)}</span>
              <span className="block break-words text-xs font-normal text-muted-foreground">
                {t(item.description_key)}
              </span>
            </span>
            {item.id === "core" && (
              <Badge variant="secondary">{t("settings:builtinMcp.required")}</Badge>
            )}
            <IconChevronRight className="size-4 shrink-0" aria-hidden />
          </Button>
        ))}
      </div>
    </section>
  );
}
