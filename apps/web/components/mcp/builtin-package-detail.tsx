import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { IconClock, IconShieldLock } from "@tabler/icons-react";
import type { BuiltinMCPDescriptor, BuiltinMCPPackage } from "@/lib/types/builtin-mcp";
import type { useBuiltinMCPManagement } from "@/hooks/domains/workspace/use-builtin-mcp-management";
import { BuiltinMCPError } from "./builtin-package-list";
import { BuiltinPackageHeader, BuiltinPackageInformation } from "./builtin-package-content";
import { builtinPackageChoice } from "./builtin-package-choice";
export { BuiltinPackageList } from "./builtin-package-list";

export type BuiltinMCPEditor = ReturnType<typeof useBuiltinMCPManagement>;

function packageRestriction(editor: BuiltinMCPEditor, item: BuiltinMCPDescriptor) {
  if (item.id === "core") return "settings:builtinMcp.coreRequired";
  const surface = editor.view?.surface;
  if (!surface) return null;
  if (item.required_surfaces?.some((value) => value === surface))
    return "settings:builtinMcp.configurationFixed";
  if (!item.surfaces.some((value) => value === surface)) return "settings:builtinMcp.ineligible";
  return null;
}
function packageChoices(scope: "workspaces" | "tasks", id: BuiltinMCPPackage) {
  if (scope === "tasks") return ["inherit", "on", "off"];
  if (id === "configuration") return ["blocked", "available"];
  return ["blocked", "available", "default"];
}

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
  const restriction = packageRestriction(editor, item);
  if (restriction) return <p className="rounded-md bg-muted p-3 text-sm">{t(restriction)}</p>;
  const { choice, enabled, dirty } = builtinPackageChoice(view, editor.draft, scope, item.id);
  const blocked = view.policy.packages[item.id] === "blocked";
  const choices = packageChoices(scope, item.id);
  return (
    <div
      className="space-y-3 rounded-lg border border-border/70 bg-muted/20 p-3"
      data-settings-dirty={dirty}
      data-settings-dirty-level="container"
    >
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <label className="block text-sm font-medium" htmlFor={`builtin-choice-${item.id}`}>
          {t(
            scope === "tasks"
              ? "settings:builtinMcp.taskChoice"
              : "settings:builtinMcp.workspacePolicy",
          )}
        </label>
        <Select
          value={choice}
          onValueChange={(value) => editor.setChoice(item.id, value as typeof choice)}
          disabled={!view.can_edit || editor.pending}
        >
          <SelectTrigger
            id={`builtin-choice-${item.id}`}
            data-testid={`builtin-mcp-choice-${item.id}`}
            data-settings-dirty={dirty}
            className={controlSizingClassName("standard", "w-full md:w-64")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {choices.map((value) => (
              <SelectItem
                key={value}
                value={value}
                disabled={scope === "tasks" && value === "on" && blocked}
              >
                {t(`settings:builtinMcp.choices.${value}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {scope === "tasks" && (
        <p className="text-sm" data-testid="builtin-mcp-effective">
          {t(enabled ? "settings:builtinMcp.effectiveOn" : "settings:builtinMcp.effectiveOff")}{" "}
          {blocked && t("settings:builtinMcp.blocked")}
        </p>
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
