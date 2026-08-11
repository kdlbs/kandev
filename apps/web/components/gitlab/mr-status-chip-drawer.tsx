"use client";

import { useEffect, useState } from "react";
import { IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { MRCIPopover } from "./mr-ci-popover";
import { MRStatusChipTriggerContent, useMRChipTriggerAttrs } from "./mr-status-chip-trigger";
import { useFrozenChipSelection, type ChipAutomation } from "./mr-status-chip-selection";
import type { MRChipStatus } from "./mr-task-icon";
import type { TaskMR } from "@/lib/types/gitlab";

/** Coarse-pointer disclosure variant: the same MRCIPopover body inside a bottom-sheet drawer. */
export function MRStatusChipDrawer({
  mrs,
  openCount,
  liveSelected,
  liveStatus,
  automation,
  taskId,
  canLink,
  onUnlink,
  onLink,
}: {
  mrs: TaskMR[];
  openCount: number;
  liveSelected: TaskMR;
  liveStatus: MRChipStatus;
  automation: ChipAutomation;
  taskId: string;
  canLink: boolean;
  onUnlink: (associationId: string) => void;
  onLink: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { actedOnMR, frozen, shouldForceClose } = useFrozenChipSelection(mrs, liveSelected, open);

  useEffect(() => {
    if (shouldForceClose) setOpen(false);
  }, [shouldForceClose]);

  const attrs = useMRChipTriggerAttrs({ openCount, liveStatus, actedOnMR, frozen });
  // The chip closes its own disclosure before the link dialog opens: the
  // drawer is itself a focus-trapping dialog, so opening a second one
  // inside it would strand focus (spec: Link and unlink from the chip).
  const handleLink = () => {
    setOpen(false);
    onLink();
  };

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      {/* asChild registers this button as Radix's Dialog trigger ref, which its
          default onCloseAutoFocus reads to restore focus on close — an
          untracked plain button leaves nothing to focus and it falls back to
          document.body (spec: Accessibility, "closing the drawer returns
          focus to the chip trigger"). */}
      <DrawerTrigger asChild>
        <button type="button" aria-haspopup="dialog" aria-expanded={open} {...attrs}>
          <MRStatusChipTriggerContent
            openCount={openCount}
            liveStatus={liveStatus}
            automation={automation}
          />
        </button>
      </DrawerTrigger>
      <DrawerContent data-testid="mr-status-chip-drawer" className="max-h-[80vh] flex flex-col">
        <DrawerHeader className="flex flex-row items-center justify-between border-b py-2">
          <DrawerTitle className="text-sm">
            {t("gitlab:mrChipDrawerTitle", { mriid: actedOnMR.mr_iid })}
          </DrawerTitle>
          <DrawerDescription className="sr-only">
            {t("gitlab:mrChipDrawerDescription")}
          </DrawerDescription>
          <DrawerClose asChild>
            <Button
              data-testid="mr-status-chip-drawer-close"
              variant="ghost"
              size="icon-sm"
              aria-label={t("gitlab:mrChipDrawerClose")}
              className="cursor-pointer"
            >
              <IconX className="h-4 w-4" />
            </Button>
          </DrawerClose>
        </DrawerHeader>
        <div className="flex-1 min-h-0 overflow-y-auto p-3" data-vaul-no-drag>
          <MRCIPopover
            mr={actedOnMR}
            taskId={taskId}
            enabled={open}
            canLink={canLink}
            onLink={handleLink}
            onUnlink={() => onUnlink(actedOnMR.id)}
          />
        </div>
      </DrawerContent>
    </Drawer>
  );
}
