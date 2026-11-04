"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Command, CommandItem, CommandList } from "@kandev/ui/command";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useWalkthroughTap } from "./use-walkthrough-tap";

type StepOption = { label: string; title: string; comments: string };

export function WalkthroughMobileStepPicker({
  options,
  activeStep,
  onSelect,
}: {
  options: StepOption[];
  activeStep: number;
  onSelect: (index: number) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const tapHandlers = useWalkthroughTap();
  return (
    <>
      <Button
        ref={trigger}
        variant="outline"
        role="combobox"
        aria-label={t("diff:walkthroughSelectStep")}
        aria-haspopup="dialog"
        aria-expanded={open}
        data-testid="walkthrough-step-picker"
        className="h-11 w-full min-w-0 justify-between px-2 text-sm"
        onClick={() => setOpen(true)}
        {...tapHandlers(() => setOpen(true))}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            setOpen(true);
          }
        }}
      >
        <span className="min-w-0 truncate">{options[activeStep]?.label}</span>
        <IconChevronDown className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
      </Button>
      <MobilePickerSheet
        open={open}
        onOpenChange={setOpen}
        title={t("diff:walkthroughSelectStep")}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          trigger.current?.focus();
        }}
      >
        <StepOptions
          open={open}
          options={options}
          activeStep={activeStep}
          tapHandlers={tapHandlers}
          onSelect={(index) => {
            setOpen(false);
            onSelect(index);
          }}
        />
      </MobilePickerSheet>
    </>
  );
}

function StepOptions({
  open,
  options,
  activeStep,
  onSelect,
  tapHandlers,
}: {
  open: boolean;
  options: StepOption[];
  activeStep: number;
  onSelect: (index: number) => void;
  tapHandlers: ReturnType<typeof useWalkthroughTap>;
}) {
  const { t } = useTranslation();
  const list = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (open) list.current?.focus({ preventScroll: true });
  }, [open]);
  return (
    <Command
      key={activeStep}
      ref={list}
      tabIndex={0}
      defaultValue={String(activeStep)}
      data-walkthrough-step-options
      className="h-auto rounded-none bg-transparent px-1 py-0"
    >
      <CommandList className="max-h-none overflow-visible" label={t("diff:walkthroughSelectStep")}>
        {options.map((option, index) => (
          <CommandItem
            key={index}
            value={String(index)}
            aria-label={option.label}
            aria-current={index === activeStep ? "step" : undefined}
            data-checked={index === activeStep}
            className="mb-0.5 min-h-11 cursor-pointer gap-2 whitespace-normal border border-transparent px-2 py-2 text-sm last:mb-0 aria-[current=step]:border-primary/50 aria-[current=step]:bg-card [overflow-wrap:anywhere]"
            onSelect={() => onSelect(index)}
            {...tapHandlers(() => onSelect(index))}
          >
            <span
              className="w-6 shrink-0 text-center text-muted-foreground tabular-nums"
              aria-hidden
            >
              {index + 1}
            </span>
            <span className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="font-medium">{option.title}</span>
              <span className="text-xs text-muted-foreground">{option.comments}</span>
            </span>
          </CommandItem>
        ))}
      </CommandList>
    </Command>
  );
}
