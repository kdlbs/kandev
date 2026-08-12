"use client";

import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { RemoteContributionResolutionAction } from "../use-remote-contribution-resolution";

export type MobileContributionResolutionDrawerProps = {
  open: boolean;
  action: RemoteContributionResolutionAction;
  repositoryName: string;
  expectedRemoteHead: string;
  isLoading: boolean;
  errorKey?: string | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

export function MobileContributionResolutionDrawer({
  open,
  action,
  repositoryName,
  expectedRemoteHead,
  isLoading,
  errorKey,
  onOpenChange,
  onConfirm,
}: MobileContributionResolutionDrawerProps) {
  const { t } = useTranslation();
  const isReplace = action === "replace";
  const title = t(isReplace ? "task:replacePRBranch" : "task:usePRVersion");
  const description = isReplace
    ? t("task:replacePRBranchConfirmation", {
        repository: repositoryName,
        head: expectedRemoteHead,
      })
    : t("task:usePRVersionConfirmation", { head: expectedRemoteHead });

  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent
        data-testid="mobile-remote-contribution-drawer"
        className="inset-x-3 bottom-3 max-h-[calc(100dvh-1.5rem)] overflow-hidden rounded-xl pb-[calc(env(safe-area-inset-bottom)+0.75rem)]"
      >
        <DrawerHeader className="px-4 pb-2">
          <DrawerTitle>{title}</DrawerTitle>
          <DrawerDescription>{description}</DrawerDescription>
        </DrawerHeader>
        {errorKey && (
          <div className="max-h-[30dvh] overflow-y-auto px-4 text-sm text-destructive">
            {t(errorKey)}
          </div>
        )}
        <DrawerFooter className="grid grid-cols-2 gap-2 px-4 pt-3">
          <Button
            type="button"
            variant="outline"
            className="h-11 cursor-pointer"
            onClick={() => onOpenChange(false)}
          >
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            className="h-11 cursor-pointer"
            data-testid="mobile-remote-contribution-confirm"
            disabled={isLoading}
            onClick={onConfirm}
          >
            {title}
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
}
