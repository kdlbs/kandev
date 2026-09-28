"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconPalette, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskColorSelection } from "@/hooks/use-task-color-selection";
import {
  TASK_COLORS,
  TASK_COLOR_BAR_CLASS,
  TASK_COLOR_LABEL_KEYS,
  type TaskColor,
} from "@/lib/task-colors";
import { cn } from "@/lib/utils";
import { MobilePickerSheet } from "./mobile/mobile-picker-sheet";

type ColorOption = {
  color: TaskColor | null;
  label: string;
  selected: boolean;
  disabled: boolean;
};

export function BulkTaskColorPicker({ taskIds }: { taskIds: string[] }) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const { ids, commonColor, hasColor, setColors, isPending } = useTaskColorSelection(taskIds);
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const choose = (color: TaskColor | null) => {
    void setColors(ids, color);
    setOpen(false);
  };
  const trigger = (
    <Button
      ref={triggerRef}
      variant="outline"
      className="cursor-pointer"
      disabled={ids.length === 0}
      aria-disabled={isPending || undefined}
      aria-label={t("task:color")}
      data-testid="bulk-color-button"
      onClick={isMobile && !isPending ? () => setOpen(true) : undefined}
    >
      <IconPalette />
      <span aria-hidden={isPending}>{isPending ? t("task:bulkColorSaving") : t("task:color")}</span>
    </Button>
  );
  const options = [...TASK_COLORS, null].map((color) => ({
    color,
    label: color ? t(TASK_COLOR_LABEL_KEYS[color]) : t("task:groupNone"),
    selected: color !== null && commonColor === color,
    disabled: isPending || (color === null && !hasColor),
  }));
  const status = (
    <span className="sr-only" role="status">
      {isPending ? t("task:bulkColorSaving") : ""}
    </span>
  );
  if (isMobile)
    return (
      <>
        {trigger}
        {status}
        <MobileBulkColorPicker
          open={open}
          onOpenChange={setOpen}
          count={ids.length}
          options={options}
          onChoose={choose}
          returnFocus={() => triggerRef.current?.focus()}
        />
      </>
    );
  return (
    <>
      <DesktopBulkColorPicker
        open={open}
        onOpenChange={(nextOpen) => {
          if (!nextOpen || !isPending) setOpen(nextOpen);
        }}
        trigger={trigger}
        commonColor={commonColor}
        options={options}
        onChoose={choose}
      />
      {status}
    </>
  );
}

function MobileBulkColorPicker({
  open,
  onOpenChange,
  count,
  options,
  onChoose,
  returnFocus,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  count: number;
  options: ColorOption[];
  onChoose: (color: TaskColor | null) => void;
  returnFocus: () => void;
}) {
  const { t } = useTranslation();
  return (
    <MobilePickerSheet
      open={open}
      onOpenChange={onOpenChange}
      title={t("task:bulkColorTitle", { count })}
      description={t("task:bulkColorAutomaticHint")}
      contentTestId="bulk-color-picker"
      onCloseAutoFocus={(event) => {
        event.preventDefault();
        returnFocus();
      }}
      headerAction={
        <Button
          variant="ghost"
          size="icon"
          className="cursor-pointer"
          aria-label={t("common:close")}
          onClick={() => onOpenChange(false)}
        >
          <IconX />
        </Button>
      }
    >
      {options.map(({ color, label, selected, disabled }) => (
        <Button
          key={color ?? "none"}
          variant="ghost"
          className="w-full cursor-pointer justify-start gap-2"
          aria-pressed={selected}
          disabled={disabled}
          onClick={() => onChoose(color)}
          data-testid={`bulk-color-option-${color ?? "none"}`}
        >
          <ColorSwatch color={color} />
          {label}
          {selected && <IconCheck className="ml-auto" />}
        </Button>
      ))}
    </MobilePickerSheet>
  );
}

function DesktopBulkColorPicker({
  open,
  onOpenChange,
  trigger,
  commonColor,
  options,
  onChoose,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: React.ReactNode;
  commonColor: TaskColor | null;
  options: ColorOption[];
  onChoose: (color: TaskColor | null) => void;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      <DropdownMenuContent
        side="top"
        align="center"
        className="w-64"
        data-testid="bulk-color-picker"
      >
        <div className="px-2 py-1.5 text-xs text-muted-foreground">
          {t("task:bulkColorAutomaticHint")}
        </div>
        <DropdownMenuRadioGroup value={commonColor ?? ""}>
          {options.map(({ color, label, disabled }) => (
            <DropdownMenuRadioItem
              key={color ?? "none"}
              value={color ?? "none"}
              disabled={disabled}
              onSelect={() => onChoose(color)}
              data-testid={`bulk-color-option-${color ?? "none"}`}
            >
              <ColorSwatch color={color} />
              {label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ColorSwatch({ color }: { color: TaskColor | null }) {
  return (
    <span
      className={cn(
        "mr-2 inline-block h-2 w-2 rounded-full",
        color ? TASK_COLOR_BAR_CLASS[color] : "border border-muted-foreground/40",
      )}
    />
  );
}
