"use client";

import { useId, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { settingsControlClassName } from "./settings-control";

type Choice = { value: string; label: string; disabled?: boolean };

type SandboxChoiceProps = {
  label: string;
  value: string;
  choices: Choice[];
  onChange: (value: string) => void;
  disabled?: boolean;
  placeholder: string;
  native?: boolean;
};

export function SandboxChoice({
  label,
  value,
  choices,
  onChange,
  disabled = false,
  placeholder,
  native = false,
}: SandboxChoiceProps) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  const selected = choices.find((choice) => choice.value === value);
  if (native)
    return (
      <div className="min-w-0 space-y-2">
        <Label htmlFor={id}>{label}</Label>
        <select
          id={id}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          disabled={disabled}
          className={settingsControlClassName(
            "w-full min-w-0 rounded-md border border-input bg-background px-3",
          )}
        >
          {!value && (
            <option value="" disabled>
              {placeholder}
            </option>
          )}
          {choices.map((choice) => (
            <option key={choice.value} value={choice.value} disabled={choice.disabled}>
              {choice.label}
            </option>
          ))}
        </select>
      </div>
    );
  return (
    <div className="min-w-0 space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {usesDesktopWorkbench ? (
        <Select value={value} onValueChange={onChange} disabled={disabled}>
          <SelectTrigger id={id} className={settingsControlClassName("w-full")}>
            <SelectValue placeholder={placeholder} />
          </SelectTrigger>
          <SelectContent>
            {choices.map((choice) => (
              <SelectItem key={choice.value} value={choice.value} disabled={choice.disabled}>
                {choice.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <>
          <Button
            id={id}
            type="button"
            variant="outline"
            className="min-h-11 w-full justify-start whitespace-normal text-left"
            aria-haspopup="dialog"
            aria-expanded={open}
            disabled={disabled}
            onClick={() => setOpen(true)}
          >
            {selected?.label ?? placeholder}
          </Button>
          <MobilePickerSheet open={open} onOpenChange={setOpen} title={label}>
            <div role="listbox" aria-label={label} className="space-y-1">
              {choices.map((choice) => (
                <Button
                  key={choice.value}
                  type="button"
                  variant={choice.value === value ? "secondary" : "ghost"}
                  role="option"
                  aria-selected={choice.value === value}
                  disabled={choice.disabled}
                  className="min-h-11 w-full justify-start whitespace-normal text-left"
                  onClick={() => {
                    onChange(choice.value);
                    setOpen(false);
                  }}
                >
                  {choice.label}
                </Button>
              ))}
            </div>
          </MobilePickerSheet>
        </>
      )}
    </div>
  );
}
