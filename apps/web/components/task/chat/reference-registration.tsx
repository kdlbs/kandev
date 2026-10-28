"use client";

import { useState } from "react";
import type { TFunction } from "i18next";
import { IconChevronDown } from "@tabler/icons-react";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { generateUUID } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import type { EntityReference, EntityReferenceAssociationMode } from "@/lib/types/entity-reference";

export function isLinkableEntityReference(reference: EntityReference): boolean {
  return (
    (reference.provider === "github" && reference.kind === "pull_request") ||
    (reference.provider === "gitlab" && reference.kind === "merge_request")
  );
}

export function createEntityReferenceSelection(
  reference: EntityReference,
  selectionId = generateUUID(),
  linkOnSendEnabled = true,
): EntityReference {
  return {
    ...reference,
    selection_id: selectionId,
    registration_status: undefined,
    registration_error_code: undefined,
    association_mode:
      linkOnSendEnabled && isLinkableEntityReference(reference) ? "link_on_send" : "context_only",
  };
}

export function changeEntityReferenceAssociationMode(
  reference: EntityReference,
  mode: EntityReferenceAssociationMode,
  selectionId?: string,
): EntityReference {
  const next: EntityReference = { ...reference, association_mode: mode };
  if (
    mode === "link_on_send" &&
    (reference.association_mode !== "link_on_send" || reference.registration_status === "unlinked")
  ) {
    next.selection_id = selectionId ?? generateUUID();
    next.registration_status = undefined;
    next.registration_error_code = undefined;
  }
  return next;
}

type ReferenceRegistrationControlProps = {
  referenceLabel: string;
  mode?: EntityReferenceAssociationMode;
  linkingEnabled?: boolean;
  commitOn?: "send" | "save";
  linkAgain?: boolean;
  onModeChange: (mode: EntityReferenceAssociationMode) => void;
};

function registrationLinkLabel(t: TFunction, commitOn = "send", linkAgain = false): string {
  if (commitOn === "save") {
    return t(linkAgain ? "task:referenceLinkAgainOnSave" : "task:referenceLinkOnSave");
  }
  return t("task:referenceLinkOnSend");
}

function RegistrationChoices({
  mode,
  onModeChange,
  commitOn,
  linkAgain,
}: Pick<ReferenceRegistrationControlProps, "mode" | "onModeChange" | "commitOn" | "linkAgain">) {
  const { t } = useTranslation();
  const activeMode = mode ?? "context_only";
  const choices: { mode: EntityReferenceAssociationMode; label: string }[] = [
    { mode: "link_on_send", label: registrationLinkLabel(t, commitOn, linkAgain) },
    { mode: "context_only", label: t("task:referenceContextOnly") },
  ];
  return (
    <div role="radiogroup" aria-label={t("task:referenceRegistrationMode")} className="grid gap-1">
      {choices.map((choice) => (
        <button
          key={choice.mode}
          type="button"
          role="radio"
          aria-checked={activeMode === choice.mode}
          data-testid={`reference-registration-${choice.mode}`}
          className="flex min-h-7 w-full cursor-pointer items-center rounded-md px-3 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onClick={() => onModeChange(choice.mode)}
        >
          <span
            className="mr-2 inline-flex size-4 items-center justify-center rounded-full border"
            aria-hidden="true"
          >
            {activeMode === choice.mode ? (
              <span className="size-2 rounded-full bg-primary" />
            ) : null}
          </span>
          {choice.label}
        </button>
      ))}
    </div>
  );
}

export function ReferenceRegistrationControl({
  referenceLabel,
  mode,
  linkingEnabled = true,
  commitOn = "send",
  linkAgain = false,
  onModeChange,
}: ReferenceRegistrationControlProps) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const activeMode = mode ?? "context_only";
  const modeLabel =
    activeMode === "link_on_send"
      ? registrationLinkLabel(t, commitOn, linkAgain)
      : t("task:referenceContextOnly");
  if (!linkingEnabled) {
    return (
      <span
        className="inline-flex max-w-full items-center gap-1 rounded bg-primary/10 px-1.5 py-0.5 align-baseline text-xs text-primary"
        data-testid="reference-context-only-chip"
      >
        <span className="max-w-[12rem] truncate">{referenceLabel}</span>
        <span className="shrink-0 opacity-80">| {t("task:referenceContextOnly")}</span>
      </span>
    );
  }
  const trigger = (
    <button
      type="button"
      aria-label={t("task:referenceRegistrationMode")}
      aria-haspopup={usesTouchDrawer ? "dialog" : "menu"}
      aria-expanded={open}
      data-testid="reference-registration-trigger"
      className="inline-flex min-h-7 max-w-full cursor-pointer items-center gap-1 rounded bg-primary/10 px-1.5 py-0.5 align-baseline text-xs text-primary hover:bg-primary/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
    >
      <span className="max-w-[12rem] truncate">{referenceLabel}</span>
      <span className="shrink-0 opacity-80">| {modeLabel}</span>
      <IconChevronDown className="size-3.5 shrink-0" aria-hidden="true" />
    </button>
  );
  const updateMode = (nextMode: EntityReferenceAssociationMode, close = true) => {
    onModeChange(nextMode);
    if (close) setOpen(false);
  };

  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent className="max-h-[min(60dvh,28rem)] rounded-t-2xl px-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
          <DrawerHeader className="px-0 text-left">
            <DrawerTitle>{t("task:referenceRegistrationMode")}</DrawerTitle>
          </DrawerHeader>
          <div className="min-h-0 overflow-y-auto overscroll-contain">
            <RegistrationChoices
              mode={activeMode}
              commitOn={commitOn}
              linkAgain={linkAgain}
              onModeChange={(nextMode) => updateMode(nextMode, false)}
            />
          </div>
          <DrawerFooter className="shrink-0 border-t px-0 pb-[max(1rem,env(safe-area-inset-bottom))]">
            <DrawerClose asChild>
              <Button type="button" className="min-h-11 w-full">
                {t("task:done")}
              </Button>
            </DrawerClose>
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent className="w-52 p-1" align="start">
        <RegistrationChoices
          mode={activeMode}
          commitOn={commitOn}
          linkAgain={linkAgain}
          onModeChange={updateMode}
        />
      </PopoverContent>
    </Popover>
  );
}
