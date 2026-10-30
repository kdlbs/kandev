import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { IconArrowLeft } from "@tabler/icons-react";
import type { BuiltinMCPDescriptor, BuiltinMCPPackage } from "@/lib/types/builtin-mcp";
import type { useBuiltinMCPManagement } from "@/hooks/domains/workspace/use-builtin-mcp-management";
import { BuiltinMCPError } from "./builtin-package-list";
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
  const choice = editor.draft[item.id] ?? "inherit";
  const blocked = view.policy.packages[item.id] === "blocked";
  const choices = packageChoices(scope, item.id);
  const enabled =
    !blocked &&
    (choice === "on" || (choice === "inherit" && view.policy.packages[item.id] === "default"));
  return (
    <div className="space-y-3">
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
          className={controlSizingClassName("standard", "w-full")}
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
      {scope === "tasks" && (
        <p className="text-sm" data-testid="builtin-mcp-effective">
          {t(enabled ? "settings:builtinMcp.effectiveOn" : "settings:builtinMcp.effectiveOff")}{" "}
          {blocked && t("settings:builtinMcp.blocked")}
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
    <section className="min-w-0 space-y-5 pb-4" data-testid="builtin-mcp-detail">
      <div className="sticky top-0 z-10 bg-background">
        <Button
          autoFocus
          variant="ghost"
          className={controlSizingClassName("standard")}
          onClick={onBack}
        >
          <IconArrowLeft className="size-4" aria-hidden />
          {t("common:back")}
        </Button>
      </div>
      <div className="space-y-2">
        <Badge variant="outline">{t("settings:builtinMcp.title")}</Badge>
        <h2 className="text-lg font-semibold">{t(item.label_key)}</h2>
        <p className="text-sm">{t(item.description_key)}</p>
        <p className="text-sm text-muted-foreground">{t("settings:builtinMcp.owner")}</p>
        <code className="block break-all text-xs text-muted-foreground">{item.runtime_name}</code>
      </div>
      <BuiltinMCPError editor={editor} />
      {item.id === "configuration" && scope === "tasks" && (
        <p className="rounded-md border p-3 text-sm">
          {t("settings:builtinMcp.configurationAuthority")}
        </p>
      )}
      <PackageChoice editor={editor} item={item} scope={scope} />
      {item.id === "configuration" && scope === "workspaces" && (
        <p className="text-sm text-muted-foreground">
          {t("settings:builtinMcp.configurationFixed")}
        </p>
      )}
      <p className="text-sm text-muted-foreground">
        {t(
          scope === "tasks"
            ? "settings:builtinMcp.taskTiming"
            : "settings:builtinMcp.workspaceTiming",
        )}
      </p>
      <div className="space-y-2">
        <h3 className="text-sm font-semibold">{t("settings:builtinMcp.contexts")}</h3>
        <p className="text-sm">
          {item.surfaces.map((surface) => t(`settings:builtinMcp.surfaces.${surface}`)).join(", ")}
        </p>
      </div>
      <details className="rounded-md border p-3">
        <summary className="min-h-7 cursor-pointer text-sm font-medium max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
          {t("settings:builtinMcp.registeredTools", { count: item.tools.length })}
        </summary>
        <p className="py-2 text-xs text-muted-foreground">{t("settings:builtinMcp.catalogOnly")}</p>
        <ul className="space-y-2">
          {item.tools.map((tool) => (
            <li key={tool.name} className="break-all font-mono text-xs">
              {tool.name}
            </li>
          ))}
        </ul>
      </details>
    </section>
  );
}
