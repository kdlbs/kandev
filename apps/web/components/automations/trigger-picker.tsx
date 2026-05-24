"use client";

import { useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Command, CommandInput, CommandList, CommandGroup, CommandItem } from "@kandev/ui/command";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconPlus, IconBrandGithub, IconWebhook, IconInfoCircle } from "@tabler/icons-react";
import type { TriggerType, TriggerTypeInfo } from "@/lib/types/automation";

type TriggerPickerProps = {
  triggerTypes: TriggerTypeInfo[];
  onSelect: (type: TriggerType, config: Record<string, unknown>) => void;
};

type CategoryMeta = {
  heading: string;
  icon: typeof IconBrandGithub;
  color: string;
};

const CATEGORY_META: Record<string, CategoryMeta> = {
  github: { heading: "GitHub", icon: IconBrandGithub, color: "text-purple-400" },
  webhook: { heading: "Webhook", icon: IconWebhook, color: "text-blue-400" },
};

export function TriggerPicker({ triggerTypes, onSelect }: TriggerPickerProps) {
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
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          data-testid="add-condition-button"
          variant="ghost"
          size="sm"
          className="cursor-pointer text-muted-foreground"
        >
          <IconPlus className="h-4 w-4 mr-1" />
          Add Condition
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[320px] p-0" align="start">
        <Command>
          <CommandInput placeholder="Search conditions..." />
          <CommandList>
            {groups.map(([category, items]) => {
              const meta = CATEGORY_META[category];
              if (!meta) return null;
              return (
                <PickerGroup
                  key={category}
                  heading={meta.heading}
                  icon={meta.icon}
                  color={meta.color}
                  items={items}
                  onSelect={handleSelect}
                />
              );
            })}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
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
  return (
    <CommandGroup heading={heading}>
      {items.map((item) => (
        <CommandItem
          key={item.type}
          onSelect={() => onSelect(item)}
          disabled={!item.enabled}
          className={!item.enabled ? "opacity-50" : "cursor-pointer"}
        >
          <Icon className={`h-4 w-4 mr-2 ${color}`} />
          <span className="flex-1">
            {item.label}
            {!item.enabled && " (Coming soon)"}
          </span>
          <Tooltip>
            <TooltipTrigger asChild>
              <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
            </TooltipTrigger>
            <TooltipContent side="right" className="max-w-[220px]">
              {item.description}
            </TooltipContent>
          </Tooltip>
        </CommandItem>
      ))}
    </CommandGroup>
  );
}
