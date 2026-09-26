"use client";

import type { ReactNode, RefObject } from "react";
import { Dialog, DialogContent } from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useTranslation } from "react-i18next";

export function NewSessionDialogSurface({
  open,
  onOpenChange,
  isMobile,
  previewOpen,
  onClosePreview,
  mobileTitleRef,
  hasConversationFork,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isMobile: boolean;
  previewOpen: boolean;
  onClosePreview: () => void;
  mobileTitleRef: RefObject<HTMLHeadingElement | null>;
  hasConversationFork: boolean;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const handleEscapeKeyDown = (event: globalThis.KeyboardEvent) => {
    if (!previewOpen) return;
    event.preventDefault();
    onClosePreview();
  };
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="!top-0 !bottom-auto !mt-0 !h-dvh !max-h-dvh min-h-0 overflow-hidden rounded-none pb-[env(safe-area-inset-bottom,0px)]"
          data-testid="new-session-drawer"
          onEscapeKeyDown={handleEscapeKeyDown}
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            requestAnimationFrame(() => mobileTitleRef.current?.focus());
          }}
        >
          {previewOpen ? (
            <DrawerHeader className="sr-only">
              <DrawerTitle>{t("task:conversationForkPreview")}</DrawerTitle>
            </DrawerHeader>
          ) : null}
          <div className="flex h-full min-h-0 flex-col">{children}</div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        onEscapeKeyDown={handleEscapeKeyDown}
        className={
          hasConversationFork
            ? "flex max-h-[85dvh] min-h-0 flex-col overflow-hidden sm:max-w-[520px]"
            : "min-w-0 overflow-hidden sm:max-w-[520px]"
        }
      >
        {children}
      </DialogContent>
    </Dialog>
  );
}
