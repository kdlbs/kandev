import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { IconArrowLeft, IconChevronRight, IconPackage } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { BuiltinMCPDescriptor } from "@/lib/types/builtin-mcp";

/** Shared package navigation for workspace, task, and automation editors. */
export function BuiltinPackageRow({
  item,
  labelKey,
  inherited = false,
  dirty = false,
  testId = `builtin-mcp-package-${item.id}`,
  onSelect,
}: {
  item: BuiltinMCPDescriptor;
  labelKey: string;
  inherited?: boolean;
  dirty?: boolean;
  testId?: string;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="ghost"
      data-testid={testId}
      data-settings-dirty={dirty}
      data-settings-dirty-level="container"
      onClick={onSelect}
      className="h-auto max-md:h-auto [@media(pointer:coarse)]:h-auto min-h-14 w-full cursor-pointer justify-start gap-3 whitespace-normal rounded-none border border-transparent p-3 text-left"
    >
      <span className="min-w-0 flex-1">
        <span className="block font-medium">{t(item.label_key)}</span>
        <span className="block break-words text-xs font-normal text-muted-foreground">
          {t(item.description_key)}
        </span>
        {inherited && (
          <span className="block text-xs font-normal text-muted-foreground">
            {t("settings:builtinMcp.choices.inherit")}
          </span>
        )}
      </span>
      <Badge variant="secondary">{t(labelKey)}</Badge>
      <IconChevronRight className="size-4 shrink-0" aria-hidden />
    </Button>
  );
}

/** Keep package identity and Back navigation consistent across native package editors. */
export function BuiltinPackageHeader({
  item,
  onBack,
  backgroundClassName = "bg-background",
}: {
  item: BuiltinMCPDescriptor;
  onBack: () => void;
  backgroundClassName?: string;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div
        className={cn(
          "sticky top-0 z-10 -mx-1 border-b border-border/60 px-1 pb-2",
          backgroundClassName,
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
    </>
  );
}

/** Show registered tools and package metadata without claiming a live connection. */
export function BuiltinPackageInformation({ item }: { item: BuiltinMCPDescriptor }) {
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
