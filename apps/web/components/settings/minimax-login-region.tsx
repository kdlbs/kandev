"use client";

import { useTranslation } from "react-i18next";
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
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (region: string) => void;
};

export function MiniMaxLoginRegion({ open, onOpenChange, onSelect }: Props) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const choices = (
    <div className={isMobile ? "flex flex-col gap-3 px-4 pb-4" : "flex justify-end gap-2"}>
      <Button className={isMobile ? "h-12 min-h-12" : undefined} onClick={() => onSelect("cn")}>
        {t("agents:minimaxRegionCN")}
      </Button>
      <Button className={isMobile ? "h-12 min-h-12" : undefined} onClick={() => onSelect("global")}>
        {t("agents:minimaxRegionGlobal")}
      </Button>
    </div>
  );
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="pb-[env(safe-area-inset-bottom)]"
          data-testid="minimax-login-region-drawer"
        >
          <DrawerHeader>
            <DrawerTitle>{t("agents:minimaxChooseRegionTitle")}</DrawerTitle>
            <DrawerDescription>{t("agents:minimaxChooseRegionDescription")}</DrawerDescription>
          </DrawerHeader>
          {choices}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("agents:minimaxChooseRegionTitle")}</DialogTitle>
          <DialogDescription>{t("agents:minimaxChooseRegionDescription")}</DialogDescription>
        </DialogHeader>
        {choices}
      </DialogContent>
    </Dialog>
  );
}
