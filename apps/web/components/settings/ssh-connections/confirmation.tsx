"use client";
import { useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { settingsActionClassName } from "../settings-control";
export function ManagerConfirmation({
  label,
  title,
  description,
  target,
  busy,
  onConfirm,
  onDismiss,
  destructive = false,
}: {
  label: string;
  title: string;
  description: ReactNode;
  target: string;
  busy: boolean;
  onConfirm: () => Promise<unknown>;
  onDismiss?: () => void;
  destructive?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLButtonElement>(null);
  const actions = {
    open,
    title,
    description,
    onOpenChange: (nextOpen: boolean) => {
      setOpen(nextOpen);
      if (!nextOpen) onDismiss?.();
    },
    cancelLabel: t("common:cancel"),
    confirmLabel: label,
    confirmDisabled: busy,
    onConfirm: () => {
      void onConfirm().catch(() => undefined);
    },
  };
  return (
    <>
      <Button
        ref={ref}
        type="button"
        className={settingsActionClassName()}
        variant={destructive ? "destructive" : "outline"}
        disabled={busy}
        onClick={() => setOpen(true)}
      >
        {label}
      </Button>
      <MobileActionConfirmation
        {...actions}
        targetKey={target}
        focusReturnRef={ref}
        variant={destructive ? "destructive" : "default"}
        fallback={<ActionConfirmPopover {...actions} anchorRef={ref} />}
      />
    </>
  );
}
