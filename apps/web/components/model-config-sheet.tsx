"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconX } from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import { Drawer, DrawerClose, DrawerContent, DrawerTitle } from "@kandev/ui/drawer";

export type ModelConfigSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  onCloseAutoFocus?: (event: Event) => void;
  children: ReactNode;
};

/**
 * Phone shell for the shared model selector: a fixed header, one vertical
 * scroller, and safe-area clearance for the single Apply action. It owns no
 * selection state; the selector keeps its draft while the sheet is open.
 */
export function ModelConfigSheet({
  open,
  onOpenChange,
  title,
  onCloseAutoFocus,
  children,
}: ModelConfigSheetProps) {
  const { t } = useTranslation();
  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent
        onCloseAutoFocus={onCloseAutoFocus}
        className="data-[vaul-drawer-direction=bottom]:!max-h-[90dvh]"
      >
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="flex items-center justify-between gap-2 px-4 pb-2">
            <DrawerTitle className="text-sm">{title}</DrawerTitle>
            <DrawerClose asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-11 w-11 shrink-0 cursor-pointer"
                aria-label={t("common:close")}
                data-testid="model-config-sheet-close"
              >
                <IconX className="h-4 w-4" aria-hidden />
              </Button>
            </DrawerClose>
          </div>
          <div
            className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 pb-[calc(1rem+env(safe-area-inset-bottom))]"
            data-testid="model-config-sheet-scroll"
          >
            {children}
          </div>
        </div>
      </DrawerContent>
    </Drawer>
  );
}
