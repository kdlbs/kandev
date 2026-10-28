"use client";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconDots, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { settingsActionClassName } from "../settings-control";

export function ResourceActions({
  label,
  description,
  target,
  busy,
  onDelete,
}: {
  label: string;
  description: string;
  target: string;
  busy: boolean;
  onDelete: () => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLButtonElement>(null);
  const pendingConfirmation = useRef(false);
  const actions = {
    open,
    title: label,
    description,
    onOpenChange: setOpen,
    cancelLabel: t("common:cancel"),
    confirmLabel: label,
    confirmDisabled: busy,
    onConfirm: () => {
      void onDelete()
        .then(() => setOpen(false))
        .catch(() => undefined);
    },
  };
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            ref={ref}
            variant="ghost"
            size="icon"
            aria-label={t("settings:sshManager.ui.moreActions")}
            className={settingsActionClassName(
              "w-7 max-md:w-11 [@media(pointer:coarse)]:w-11 cursor-pointer",
            )}
          >
            <IconDots className="size-4" aria-hidden />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          onCloseAutoFocus={(event) => {
            if (!pendingConfirmation.current) return;
            event.preventDefault();
            pendingConfirmation.current = false;
            ref.current?.focus();
            setOpen(true);
          }}
        >
          <DropdownMenuItem
            disabled={busy}
            className="text-destructive cursor-pointer"
            onSelect={() => {
              pendingConfirmation.current = true;
            }}
          >
            <IconTrash className="size-4" aria-hidden />
            {label}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <MobileActionConfirmation
        {...actions}
        targetKey={target}
        focusReturnRef={ref}
        variant="destructive"
        fallback={<ActionConfirmPopover {...actions} anchorRef={ref} />}
      />
    </>
  );
}
