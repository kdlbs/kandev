"use client";
import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { settingsControlClassName } from "../settings-control";
export function ConnectionPicker({
  id,
  label,
  value,
  options,
  onChange,
  disabled = false,
}: {
  id: string;
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const touch = useTouchDrawer();
  const [open, setOpen] = useState(false);
  return (
    <div className="space-y-1 min-w-0">
      <Label htmlFor={id}>{label}</Label>
      {touch ? (
        <Drawer open={open} onOpenChange={setOpen}>
          <DrawerTrigger asChild>
            <Button
              id={id}
              type="button"
              variant="outline"
              disabled={disabled}
              className="min-h-11 w-full justify-between text-left"
            >
              <span className="truncate">
                {options.find((option) => option.value === value)?.label ?? options[0]?.label}
              </span>
            </Button>
          </DrawerTrigger>
          <DrawerContent aria-describedby={undefined}>
            <DrawerHeader>
              <DrawerTitle>{label}</DrawerTitle>
            </DrawerHeader>
            <div
              role="listbox"
              aria-label={label}
              className="max-h-[60dvh] overflow-y-auto overscroll-contain px-4 pb-[max(1rem,env(safe-area-inset-bottom))]"
            >
              {options.map((option) => (
                <Button
                  key={option.value}
                  type="button"
                  role="option"
                  aria-selected={option.value === value}
                  variant={option.value === value ? "secondary" : "ghost"}
                  className="min-h-11 w-full justify-start whitespace-normal text-left"
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
      ) : (
        <select
          id={id}
          disabled={disabled}
          className={settingsControlClassName("w-full rounded-md border bg-background px-2")}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      )}
    </div>
  );
}
