"use client";

import { useId, useRef, type RefObject } from "react";
import { Button } from "@kandev/ui/button";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
} from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";

type CloseTerminalConfirmPopoverProps = {
  open: boolean;
  terminalName: string;
  anchorRef: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

/**
 * Compact confirmation anchored to the close control that opened it.
 * Teardown starts only after confirmation; the popover releases immediately.
 */
export function CloseTerminalConfirmPopover({
  open,
  terminalName,
  anchorRef,
  focusBoundaryRef,
  onOpenChange,
  onConfirm,
}: CloseTerminalConfirmPopoverProps) {
  const { t } = useTranslation();
  const titleId = useId();
  const descriptionId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const confirmedRef = useRef(false);

  return (
    <Popover
      modal={false}
      open={open}
      onOpenChange={(nextOpen) => {
        if (nextOpen) confirmedRef.current = false;
        onOpenChange(nextOpen);
      }}
    >
      {/* Radix accepts a null current value at runtime while its public type omits it. */}
      <PopoverAnchor virtualRef={anchorRef as RefObject<HTMLElement>} />
      <PopoverContent
        role="dialog"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        data-testid="terminal-close-confirm-popover"
        side="bottom"
        align="end"
        sideOffset={8}
        className="w-64 gap-3 p-3"
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          cancelRef.current?.focus();
        }}
        onFocusOutside={(event) => {
          if (focusBoundaryRef?.current?.contains(event.target as Node)) {
            event.preventDefault();
          }
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (!confirmedRef.current) anchorRef.current?.focus();
          confirmedRef.current = false;
        }}
      >
        <PopoverHeader>
          <PopoverTitle id={titleId}>{t("task:closeTerminal")}</PopoverTitle>
          <PopoverDescription id={descriptionId}>
            {t("task:thisStopsTheShellAndAny", { terminalName })}
          </PopoverDescription>
        </PopoverHeader>
        <div className="flex justify-end gap-2">
          <Button
            ref={cancelRef}
            type="button"
            variant="outline"
            className="h-10 px-3 transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]"
            onClick={() => onOpenChange(false)}
          >
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            className="h-10 px-3 transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]"
            onClick={() => {
              confirmedRef.current = true;
              onOpenChange(false);
              void onConfirm();
            }}
          >
            {t("task:closeTerminal2")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
