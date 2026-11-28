"use client";

import { useCallback, useState } from "react";
import { addDesktopDiscoveryRootAction } from "@/app/actions/workspaces";
import { useToast } from "@/components/toast-provider";
import { useRepositoryDiscovery } from "@/hooks/domains/workspace/use-repository-discovery";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { RepositoryDiscoveryControls } from "@/components/repository-discovery-controls";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export type RepositoryDiscoveryDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  isInitialLoading?: boolean;
};

export function RepositoryDiscoveryDialog({
  open,
  onOpenChange,
  workspaceId,
  isInitialLoading = false,
}: RepositoryDiscoveryDialogProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();

  const body = (
    <div className="py-2 overflow-y-auto max-h-[70vh]">
      <RepositoryDiscoveryControls
        workspaceId={workspaceId}
        enabled={open}
        presentation="dialog"
        isInitialLoading={isInitialLoading}
      />
    </div>
  );

  const footerButton = (
    <Button
      type="button"
      variant="outline"
      className="cursor-pointer max-md:min-h-11 max-md:w-full [@media(pointer:coarse)]:min-h-11"
      onClick={() => onOpenChange(false)}
    >
      {t("common:close")}
    </Button>
  );

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="max-h-[88dvh] min-w-0 overflow-hidden px-4 pb-4"
          data-testid="repository-discovery-drawer"
        >
          <DrawerHeader className="text-left px-0">
            <DrawerTitle>{t("workspaces:repositoryDiscoveryTitle")}</DrawerTitle>
            <DrawerDescription>
              {t("workspaces:chooseFoldersToDiscoverRepositoriesDescription")}
            </DrawerDescription>
          </DrawerHeader>
          {body}
          <DrawerFooter className="px-0 pt-2">{footerButton}</DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl" data-testid="repository-discovery-dialog">
        <DialogHeader>
          <DialogTitle>{t("workspaces:repositoryDiscoveryTitle")}</DialogTitle>
          <DialogDescription>
            {t("workspaces:chooseFoldersToDiscoverRepositoriesDescription")}
          </DialogDescription>
        </DialogHeader>
        {body}
        <DialogFooter>{footerButton}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function useDiscoverySettingsState(workspaceId: string | null) {
  const discovery = useRepositoryDiscovery(workspaceId);
  const showDiscoveryControls = discovery.desktopRuntime;
  const [discoverySettingsOpen, setDiscoverySettingsOpen] = useState(false);
  const [addingHome, setAddingHome] = useState(false);
  const { toast } = useToast();
  const { t } = useTranslation();
  const handleAddHomeAndOpenDiscovery = useCallback(async () => {
    setDiscoverySettingsOpen(true);
    setAddingHome(true);
    try {
      await addDesktopDiscoveryRootAction("~");
      await discovery.synchronizeAfterRootMutation("load");
    } catch (error) {
      toast({
        title: t("workspaces:failedToDiscoverRepositories"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    } finally {
      setAddingHome(false);
    }
  }, [discovery, toast, t]);
  return {
    showDiscoveryControls,
    discoverySettingsOpen,
    addingHome,
    setDiscoverySettingsOpen,
    handleAddHomeAndOpenDiscovery,
  };
}
