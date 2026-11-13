import { useTranslation } from "react-i18next";
import { IconClock, IconShieldLock } from "@tabler/icons-react";
import type { BuiltinMCPDescriptor } from "@/lib/types/builtin-mcp";
import type { useBuiltinMCPManagement } from "@/hooks/domains/workspace/use-builtin-mcp-management";
import { BuiltinMCPError } from "./builtin-package-list";
import { BuiltinPackageHeader, BuiltinPackageInformation } from "./builtin-package-content";
import { builtinPackageChoice, builtinPackageRestriction } from "./builtin-package-choice";
import { TaskPackageControls, WorkspacePackageControls } from "./builtin-package-controls";
export { BuiltinPackageList } from "./builtin-package-list";

export type BuiltinMCPEditor = ReturnType<typeof useBuiltinMCPManagement>;

function PackageChoice({
  editor,
  item,
  scope,
}: {
  editor: BuiltinMCPEditor;
  item: BuiltinMCPDescriptor;
  scope: "workspaces" | "tasks";
}) {
  const { t } = useTranslation();
  const view = editor.view!;
  const restriction = builtinPackageRestriction(editor.view, item);
  if (restriction) return <p className="rounded-md bg-muted p-3 text-sm">{t(restriction)}</p>;
  const { dirty } = builtinPackageChoice(view, editor.draft, scope, item.id);
  return (
    <div
      role="group"
      aria-label={t(item.label_key)}
      tabIndex={-1}
      className="space-y-3 rounded-lg border border-border/70 bg-muted/20 p-3"
      data-settings-dirty={dirty}
      data-settings-dirty-level="container"
    >
      {scope === "tasks" ? (
        <TaskPackageControls editor={editor} item={item} />
      ) : (
        <WorkspacePackageControls editor={editor} item={item} />
      )}
      {item.id === "configuration" && scope === "tasks" && (
        <p className="flex items-start gap-2 border-t border-border/60 pt-3 text-xs text-muted-foreground">
          <IconShieldLock className="mt-0.5 size-4 shrink-0" aria-hidden />
          <span>{t("settings:builtinMcp.configurationAuthority")}</span>
        </p>
      )}
      {!view.can_edit && (
        <p className="text-sm text-muted-foreground">{t("settings:builtinMcp.readOnly")}</p>
      )}
    </div>
  );
}

export function BuiltinPackageDetail({
  editor,
  item,
  scope,
  onBack,
}: {
  editor: BuiltinMCPEditor;
  item: BuiltinMCPDescriptor;
  scope: "workspaces" | "tasks";
  onBack: () => void;
}) {
  const { t } = useTranslation();
  return (
    <section className="min-w-0 space-y-4 pb-4" data-testid="builtin-mcp-detail">
      <BuiltinPackageHeader
        item={item}
        onBack={onBack}
        backgroundClassName={
          scope === "tasks"
            ? "bg-popover max-md:bg-background [@media(pointer:coarse)]:bg-background"
            : "bg-background"
        }
      />
      <BuiltinMCPError editor={editor} />
      <PackageChoice editor={editor} item={item} scope={scope} />
      {item.id === "configuration" && scope === "workspaces" && (
        <p className="text-xs text-muted-foreground">
          {t("settings:builtinMcp.configurationFixed")}
        </p>
      )}
      <p className="flex items-start gap-2 text-xs text-muted-foreground">
        <IconClock className="mt-0.5 size-4 shrink-0" aria-hidden />
        <span>
          {t(
            scope === "tasks"
              ? "settings:builtinMcp.taskTiming"
              : "settings:builtinMcp.workspaceTiming",
          )}
        </span>
      </p>
      <BuiltinPackageInformation item={item} />
    </section>
  );
}
