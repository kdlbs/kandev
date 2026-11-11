"use client";

import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DialogDescription, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { DrawerDescription, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useTranslation } from "react-i18next";

function ExplorerCloseButton({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="h-7 w-7 shrink-0 cursor-pointer [@media(pointer:coarse)]:h-12 [@media(pointer:coarse)]:w-12 max-md:h-12 max-md:w-12"
      aria-label={t("task:mcpClose")}
      data-testid="mcp-explorer-close"
      onClick={onClose}
    >
      <IconX className="h-4 w-4" aria-hidden="true" />
    </Button>
  );
}

function HeaderCopy() {
  const { t } = useTranslation();
  return (
    <div className="min-w-0">
      <DialogTitle className="text-base font-semibold leading-6">
        {t("task:mcpExplorerTitle")}
      </DialogTitle>
      <DialogDescription className="mt-1 text-pretty text-[13px] leading-5">
        {t("task:mcpExplorerDescription")}
      </DialogDescription>
    </div>
  );
}

export function McpExplorerHeader({ onClose, mobile }: { onClose: () => void; mobile: boolean }) {
  const { t } = useTranslation();
  if (mobile) {
    return (
      <DrawerHeader className="shrink-0 border-b">
        <div className="grid min-w-0 grid-cols-[3rem_minmax(0,1fr)_3rem] items-center gap-x-2">
          <DrawerTitle className="col-start-2 text-center text-base font-semibold leading-6">
            {t("task:mcpExplorerTitle")}
          </DrawerTitle>
          <div className="col-start-3 row-start-1 justify-self-end">
            <ExplorerCloseButton onClose={onClose} />
          </div>
          <DrawerDescription className="col-span-3 mt-1 text-center text-pretty text-[13px] leading-5">
            {t("task:mcpExplorerDescription")}
          </DrawerDescription>
        </div>
      </DrawerHeader>
    );
  }
  return (
    <DialogHeader className="shrink-0">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <HeaderCopy />
        <ExplorerCloseButton onClose={onClose} />
      </div>
    </DialogHeader>
  );
}
