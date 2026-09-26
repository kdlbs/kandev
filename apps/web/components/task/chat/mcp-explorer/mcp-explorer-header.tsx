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
      className="h-11 w-11 shrink-0 cursor-pointer"
      aria-label={t("task:mcpClose")}
      data-testid="mcp-explorer-close"
      onClick={onClose}
    >
      <IconX className="h-4 w-4" aria-hidden="true" />
    </Button>
  );
}

function HeaderCopy({ mobile }: { mobile: boolean }) {
  const { t } = useTranslation();
  if (mobile) {
    return (
      <div className="min-w-0">
        <DrawerTitle className="text-[13px] leading-5">{t("task:mcpExplorerTitle")}</DrawerTitle>
        <DrawerDescription className="text-[13px] leading-5">
          {t("task:mcpExplorerDescription")}
        </DrawerDescription>
      </div>
    );
  }
  return (
    <div className="min-w-0">
      <DialogTitle className="text-[13px] leading-5">{t("task:mcpExplorerTitle")}</DialogTitle>
      <DialogDescription className="text-[13px] leading-5">
        {t("task:mcpExplorerDescription")}
      </DialogDescription>
    </div>
  );
}

export function McpExplorerHeader({ onClose, mobile }: { onClose: () => void; mobile: boolean }) {
  const content = (
    <div className="flex min-w-0 items-start justify-between gap-3">
      <HeaderCopy mobile={mobile} />
      <ExplorerCloseButton onClose={onClose} />
    </div>
  );
  if (mobile) {
    return <DrawerHeader className="shrink-0 border-b text-left">{content}</DrawerHeader>;
  }
  return <DialogHeader className="shrink-0">{content}</DialogHeader>;
}
