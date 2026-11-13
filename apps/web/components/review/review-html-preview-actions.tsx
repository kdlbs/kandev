"use client";

import { useState, type ReactNode } from "react";
import { IconEye } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTranslation } from "react-i18next";

export function useHtmlPreviewLabel(previewing?: boolean): string {
  const { t } = useTranslation();
  return previewing ? t("review:showDiff") : t("editors:previewHtml");
}

function HtmlPreviewTouchDrawer({
  open,
  onOpenChange,
  onConfirm,
  trigger,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  trigger: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent data-testid="review-html-preview-help-drawer">
        <DrawerHeader>
          <DrawerTitle>{t("editors:previewHtml")}</DrawerTitle>
          <DrawerDescription>{t("review:htmlPreviewWorkspaceSource")}</DrawerDescription>
        </DrawerHeader>
        <div className="px-4 text-sm text-muted-foreground">
          <p>{t("task:htmlPreviewTrustedCode")}</p>
        </div>
        <DrawerFooter className="pb-[max(1rem,env(safe-area-inset-bottom))]">
          <Button
            type="button"
            data-testid="review-html-preview-confirm"
            className="min-h-11 cursor-pointer"
            onClick={onConfirm}
          >
            {t("editors:previewHtml")}
          </Button>
          <DrawerClose asChild>
            <Button type="button" variant="outline" className="min-h-11 cursor-pointer">
              {t("common:cancel")}
            </Button>
          </DrawerClose>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
}

/**
 * Compact desktop action for fine pointers. Coarse pointers get the same
 * action behind a drawer so the source and trusted-code copy is reachable
 * before workspace HTML executes. Returning to the diff stays immediate.
 */
export function HtmlPreviewToolbarButton({
  active,
  onToggle,
}: {
  active?: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const touchDrawer = useTouchDrawer();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const showDisclosure = touchDrawer && !active;
  const label = useHtmlPreviewLabel(active);
  const className = `h-6 w-6 p-0 cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11 ${
    active ? "bg-muted opacity-100" : "opacity-60 hover:opacity-100"
  }`;
  const button = (
    <Button
      type="button"
      aria-label={label}
      aria-pressed={active}
      aria-haspopup={showDisclosure ? "dialog" : undefined}
      aria-expanded={showDisclosure ? drawerOpen : undefined}
      variant="ghost"
      size="sm"
      data-testid="review-html-preview-toggle"
      className={className}
      onClick={showDisclosure ? undefined : onToggle}
    >
      <IconEye className="h-3.5 w-3.5" />
    </Button>
  );
  if (showDisclosure) {
    return (
      <HtmlPreviewTouchDrawer
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        onConfirm={() => {
          setDrawerOpen(false);
          onToggle();
        }}
        trigger={button}
      />
    );
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent>
        <p>{label}</p>
        <p className="mt-1 max-w-xs">{t("review:htmlPreviewWorkspaceSource")}</p>
        <p className="mt-1 max-w-xs text-muted-foreground">{t("task:htmlPreviewTrustedCode")}</p>
      </TooltipContent>
    </Tooltip>
  );
}
