"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { settingsControlClassName } from "./settings-control";

type OwnerOption = { value: string; label: string };

export function ExecutorOwnerSelect({
  value,
  onChange,
  title,
  disabled,
  options,
}: {
  value: string;
  onChange: (owner: string) => void;
  title: string;
  disabled: boolean;
  options: OwnerOption[];
}) {
  const touch = useTouchDrawer();
  const { isMobile } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const label = options.find((option) => option.value === value)?.label ?? value;
  if (isMobile || touch)
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>
          <Button
            variant="outline"
            disabled={disabled}
            className={settingsControlClassName("w-full cursor-pointer justify-between")}
            aria-label={title}
          >
            {label}
          </Button>
        </DrawerTrigger>
        <DrawerContent aria-describedby={undefined} className="max-h-[80dvh]">
          <DrawerHeader>
            <DrawerTitle>{title}</DrawerTitle>
          </DrawerHeader>
          <div
            role="listbox"
            aria-label={title}
            className="min-h-0 flex-1 overflow-y-auto px-4 pb-[max(1rem,env(safe-area-inset-bottom))]"
          >
            {options.map((option) => (
              <Button
                key={option.value}
                role="option"
                aria-selected={value === option.value}
                variant={value === option.value ? "secondary" : "ghost"}
                className="h-12 w-full cursor-pointer justify-start"
                onClick={() => {
                  onChange(option.value);
                  setOpen(false);
                }}
              >
                {option.label}
              </Button>
            ))}
          </div>
        </DrawerContent>
      </Drawer>
    );
  return (
    <Select
      value={value || "shared"}
      onValueChange={(owner) => onChange(owner === "shared" ? "" : owner)}
      disabled={disabled}
    >
      <SelectTrigger
        aria-label={title}
        className={settingsControlClassName("w-full cursor-pointer")}
      >
        <SelectValue>{label}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value || "shared"}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
