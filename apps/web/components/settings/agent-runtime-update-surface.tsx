"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowLeft, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { cn } from "@/lib/utils";

export function RuntimeUpdateDetailHeader({
  isMobile,
  title,
  description,
  back,
  onClose,
}: {
  isMobile: boolean;
  title: string;
  description: string;
  back?: { label?: string; onClick: () => void; testId: string };
  onClose?: () => void;
}) {
  const { t } = useTranslation();
  const Header = isMobile ? DrawerHeader : DialogHeader;
  const Title = isMobile ? DrawerTitle : DialogTitle;
  const Description = isMobile ? DrawerDescription : DialogDescription;
  const backLabel = back?.label ?? t("agents:runtimeUpdateBackToAll");
  const guidanceInsets = isMobile ? "px-4 py-3 text-left" : "px-4 pb-1 pt-3 pe-10";

  return (
    <Header
      className={cn(
        "shrink-0",
        onClose ? "flex-row items-center gap-2 border-b px-4 py-2 md:px-5" : guidanceInsets,
        back && !onClose && "flex-row items-center gap-2 border-b py-2 ps-2",
      )}
    >
      {back && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-muted-foreground focus-visible:text-foreground active:bg-muted"
          aria-label={backLabel}
          title={backLabel}
          onClick={back.onClick}
          data-testid={back.testId}
        >
          <IconArrowLeft className="size-4" aria-hidden="true" />
        </Button>
      )}
      <Title className={cn((back || onClose) && "text-start", onClose && "flex-1")}>{title}</Title>
      <Description className="sr-only">{description}</Description>
      {onClose && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-muted-foreground"
          aria-label={t("common:close")}
          onClick={onClose}
        >
          <IconX className="size-4" aria-hidden />
        </Button>
      )}
    </Header>
  );
}

export function AgentRuntimeUpdateSurface({
  agentName,
  displayName,
  isMobile,
  open,
  onOpenChange,
  body,
  footer,
  managedFallback,
  onBack,
  backLabel,
}: {
  agentName: string;
  displayName: string;
  isMobile: boolean;
  open: boolean;
  onOpenChange: (nextOpen: boolean) => void;
  body: ReactNode;
  footer: (mobile?: boolean) => ReactNode;
  managedFallback?: boolean;
  onBack?: () => void;
  backLabel?: string;
}) {
  const { t } = useTranslation();
  const description = managedFallback
    ? t("agents:runtimeFallbackReview")
    : t("agents:runtimeUpdateSessionsNote");
  const header = (
    <RuntimeUpdateDetailHeader
      isMobile={isMobile}
      title={t("agents:updateAgent", { name: displayName })}
      description={description}
      onClose={() => onOpenChange(false)}
      back={
        onBack
          ? { label: backLabel, onClick: onBack, testId: `agent-update-back-${agentName}` }
          : undefined
      }
    />
  );
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="flex max-h-[92dvh] flex-col overflow-clip data-[vaul-drawer-direction=bottom]:max-h-[92dvh]"
          data-testid={`agent-update-drawer-${agentName}`}
        >
          {header}
          {body}
          {footer(true)}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-[34rem]"
        data-testid={`agent-update-dialog-${agentName}`}
      >
        {header}
        {body}
        {footer()}
      </DialogContent>
    </Dialog>
  );
}
