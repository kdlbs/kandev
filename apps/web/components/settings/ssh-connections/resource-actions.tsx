"use client";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
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
      <Button
        ref={ref}
        variant="ghost"
        className={settingsActionClassName("text-muted-foreground hover:text-destructive")}
        aria-label={label}
        disabled={busy}
        onClick={() => setOpen(true)}
      >
        <IconTrash className="size-3.5 mr-1.5" aria-hidden />
        {t("common:delete")}
      </Button>
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
