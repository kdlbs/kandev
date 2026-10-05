"use client";

import { useRef, useState, type ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { IconCheck, IconChevronDown } from "@tabler/icons-react";
import { AgentSelector } from "@/components/task-create-dialog-selectors";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type AgentOption = ComponentProps<typeof AgentSelector>["options"][number];

type QuickChatAgentPickerProps = {
  options: AgentOption[];
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  labelId: string;
  ariaDescribedBy: string;
};

export function QuickChatAgentPicker({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  labelId,
  ariaDescribedBy,
}: QuickChatAgentPickerProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const touch = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const selected = options.find((option) => option.value === value);

  if (!isMobile && !touch) {
    return (
      <AgentSelector
        options={options}
        value={value}
        onValueChange={onValueChange}
        disabled={disabled}
        placeholder={placeholder}
        triggerClassName="h-7 w-full justify-between px-1 hover:bg-accent/50"
        popoverPortal
        testId="agent-profile-selector"
        ariaDescribedBy={ariaDescribedBy}
      />
    );
  }

  return (
    <>
      <Button
        ref={triggerRef}
        type="button"
        variant="outline"
        role="combobox"
        aria-labelledby={`${labelId} quick-chat-agent-selected-value`}
        aria-describedby={ariaDescribedBy}
        aria-haspopup="dialog"
        aria-expanded={open}
        disabled={disabled}
        data-testid="agent-profile-selector"
        className="h-11 min-h-11 w-full justify-between cursor-pointer"
        onClick={() => setOpen(true)}
      >
        <span id="quick-chat-agent-selected-value" className="min-w-0 flex-1 text-left">
          {selected?.renderTriggerLabel?.() ??
            selected?.renderLabel?.() ??
            selected?.label ??
            placeholder}
        </span>
        <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" aria-hidden />
      </Button>
      <MobilePickerSheet
        open={open}
        onOpenChange={setOpen}
        title={t("chat:agentProfile")}
        contentTestId="quick-chat-agent-picker-content"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          triggerRef.current?.focus();
        }}
      >
        <Command>
          <CommandInput placeholder={t("task:searchAgents")} className="h-11 min-h-12" />
          <CommandList>
            <CommandEmpty>{t("task:noAgentFound")}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value}
                  keywords={[option.label]}
                  className="min-h-12 py-2"
                  onSelect={() => {
                    onValueChange(option.value);
                    setOpen(false);
                  }}
                >
                  <span className="min-w-0 flex-1 truncate">
                    {option.renderLabel ? option.renderLabel() : option.label}
                  </span>
                  <IconCheck
                    className={`ml-2 h-4 w-4 shrink-0 ${option.value === value ? "opacity-100" : "opacity-0"}`}
                    aria-hidden
                  />
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </MobilePickerSheet>
    </>
  );
}
