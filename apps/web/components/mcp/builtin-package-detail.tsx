import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { IconArrowLeft, IconClock, IconPackage, IconShieldLock } from "@tabler/icons-react";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { cn } from "@/lib/utils";
import type { BuiltinMCPDescriptor, BuiltinMCPPackage } from "@/lib/types/builtin-mcp";
import type { useBuiltinMCPManagement } from "@/hooks/domains/workspace/use-builtin-mcp-management";
import { BuiltinMCPError } from "./builtin-package-list";
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

function PackageInformation({ item }: { item: BuiltinMCPDescriptor }) {
  const { t } = useTranslation();
  return (
    <Accordion
      type="multiple"
      className="[&_[data-slot=accordion-content]]:motion-reduce:animate-none"
    >
      <AccordionItem value="tools">
        <AccordionTrigger className="min-h-11 items-center px-3 text-[13px] transition-colors md:min-h-9 [@media(pointer:coarse)]:min-h-11">
          {t("settings:builtinMcp.registeredTools", { count: item.tools.length })}
        </AccordionTrigger>
        <AccordionContent className="space-y-3 px-1 pb-3">
          <p className="text-xs text-muted-foreground">{t("settings:builtinMcp.catalogOnly")}</p>
          <ul className="space-y-2">
            {item.tools.map((tool) => (
              <li key={tool.name} className="break-all font-mono text-xs">
                {tool.name}
              </li>
            ))}
          </ul>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="details">
        <AccordionTrigger className="min-h-11 items-center px-3 text-[13px] transition-colors md:min-h-9 [@media(pointer:coarse)]:min-h-11">
          {t("settings:builtinMcp.packageDetails")}
        </AccordionTrigger>
        <AccordionContent className="space-y-3 px-1 pb-3 text-xs text-muted-foreground">
          <p>{t("settings:builtinMcp.owner")}</p>
          <code className="block break-all">{item.runtime_name}</code>
          <div className="space-y-1">
            <h3 className="font-medium text-foreground">{t("settings:builtinMcp.contexts")}</h3>
            <p>
              {item.surfaces
                .map((surface) => t(`settings:builtinMcp.surfaces.${surface}`))
                .join(", ")}
            </p>
          </div>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
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
      <div
        className={cn(
          "sticky top-0 z-10 -mx-1 border-b border-border/60 px-1 pb-2",
          scope === "tasks"
            ? "bg-popover max-md:bg-background [@media(pointer:coarse)]:bg-background"
            : "bg-background",
        )}
      >
        <Button
          type="button"
          autoFocus
          variant="ghost"
          className={controlSizingClassName("standard", "-ml-2")}
          onClick={onBack}
        >
          <IconArrowLeft className="size-4" aria-hidden />
          {t("settings:builtinMcp.backToPackages")}
        </Button>
      </div>
      <div className="flex items-start gap-3">
        <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted/60">
          <IconPackage className="size-5" strokeWidth={1.75} aria-hidden />
        </div>
        <div className="min-w-0 space-y-1">
          <Badge variant="outline" className="text-[11px] font-medium">
            {t("settings:builtinMcp.packageLabel")}
          </Badge>
          <h2 className="text-lg font-semibold leading-6">{t(item.label_key)}</h2>
          <p className="text-[13px] text-muted-foreground">{t(item.description_key)}</p>
        </div>
      </div>
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
      <PackageInformation item={item} />
    </section>
  );
}
