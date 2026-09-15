"use client";

import { useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Command, CommandInput, CommandList, CommandGroup, CommandItem } from "@kandev/ui/command";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconPlus, IconBrandGithub, IconWebhook, IconInfoCircle } from "@tabler/icons-react";
import type { TriggerType, TriggerTypeInfo } from "@/lib/types/automation";

import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import { useIsMobile } from "@/hooks/use-mobile";

type TriggerPickerProps = {
  triggerTypes: TriggerTypeInfo[];
  onSelect: (type: TriggerType, config: Record<string, unknown>) => void;
};

type CategoryMeta = {
  headingKey: string;
  icon: typeof IconBrandGithub;
  color: string;
};

// Keyed by the backend's trigger-type category id; only the heading is copy.
const CATEGORY_META: Record<string, CategoryMeta> = {
  github: {
    headingKey: "automations:triggerCategoryGithub",
    icon: IconBrandGithub,
    color: "text-purple-400",
  },
  webhook: {
    headingKey: "automations:triggerCategoryWebhook",
    icon: IconWebhook,
    color: "text-blue-400",
  },
};

export function TriggerPicker({ triggerTypes, onSelect }: TriggerPickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  // Only show condition types (not schedule — that's handled separately).
  const conditionTypes = useMemo(
    () => triggerTypes.filter((t) => t.category !== "schedule"),
    [triggerTypes],
  );

  const groups = useMemo(() => {
    const byCategory = new Map<string, TriggerTypeInfo[]>();
    for (const t of conditionTypes) {
      const list = byCategory.get(t.category) ?? [];
      list.push(t);
      byCategory.set(t.category, list);
    }
    return Array.from(byCategory.entries());
  }, [conditionTypes]);

  const handleSelect = (info: TriggerTypeInfo) => {
    if (!info.enabled) return;
    onSelect(info.type, info.default_config);
    setOpen(false);
  };

  return (
    <ConditionPickerSurface
      open={open}
      onOpenChange={setOpen}
      trigger={
        <Button
          data-testid="add-condition-button"
          variant="ghost"
          size="sm"
          className="cursor-pointer text-muted-foreground"
        >
          <IconPlus className="h-4 w-4 mr-1" />
          {t("automations:addCondition")}
        </Button>
      }
    >
      <Command>
        <CommandInput placeholder={t("automations:searchConditions")} />
        <CommandList>
          {groups.map(([category, items]) => {
            const meta = CATEGORY_META[category];
            if (!meta && !items[0]?.plugin) return null;
            return (
              <PickerGroup
                key={category}
                heading={meta ? t(meta.headingKey) : items[0].plugin!.provider_label}
                icon={meta?.icon ?? IconWebhook}
                color={meta?.color ?? "text-blue-400"}
                items={items}
                onSelect={handleSelect}
              />
            );
          })}
        </CommandList>
      </Command>
    </ConditionPickerSurface>
  );
}

function PickerGroup({
  heading,
  icon: Icon,
  color,
  items,
  onSelect,
}: {
  heading: string;
  icon: typeof IconBrandGithub;
  color: string;
  items: TriggerTypeInfo[];
  onSelect: (info: TriggerTypeInfo) => void;
}) {
  const { t } = useTranslation();
  return (
    <CommandGroup heading={heading}>
      {items.map((item) => (
        <CommandItem
          key={item.plugin ? `${item.plugin.plugin_id}:${item.plugin.condition.key}` : item.type}
          onSelect={() => onSelect(item)}
          disabled={!item.enabled}
          className={!item.enabled ? "opacity-50" : "cursor-pointer"}
        >
          <Icon className={`h-4 w-4 mr-2 ${color}`} />
          {/* item.label and item.description are authored by the backend's
              trigger-type registry, not the frontend. */}
          <span className="flex-1">
            {item.plugin
              ? t(item.plugin.condition.label_key ?? item.label, {
                  ns: `plugin-${item.plugin.plugin_id}`,
                  defaultValue: item.label,
                })
              : item.label}
            {!item.enabled && t("automations:comingSoonSuffix")}
          </span>
          <Tooltip>
            <TooltipTrigger asChild>
              <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
            </TooltipTrigger>
            <TooltipContent side="right" className="max-w-[220px]">
              {item.plugin
                ? t(item.plugin.condition.description_key ?? item.description, {
                    ns: `plugin-${item.plugin.plugin_id}`,
                    defaultValue: item.description,
                  })
                : item.description}
            </TooltipContent>
          </Tooltip>
        </CommandItem>
      ))}
    </CommandGroup>
  );
}

function ConditionPickerSurface({
  open,
  onOpenChange,
  trigger,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: ReactNode;
  children: ReactNode;
}) {
  const mobile = useIsMobile();
  const { t } = useTranslation();
  if (mobile)
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>{t("automations:addCondition")}</DrawerTitle>
          </DrawerHeader>
          <div className="px-4 pb-6">{children}</div>
        </DrawerContent>
      </Drawer>
    );
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent className="w-[320px] p-0" align="start">
        {children}
      </PopoverContent>
    </Popover>
  );
}
