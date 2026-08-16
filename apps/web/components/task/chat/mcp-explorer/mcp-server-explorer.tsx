"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { selectMcpServerName } from "./mcp-explorer-view-model";
import { McpServerDetail } from "./mcp-server-detail";
import { McpServerList } from "./mcp-server-list";

function ExplorerCloseButton({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="h-11 w-11 shrink-0"
      aria-label={t("task:mcpClose")}
      data-testid="mcp-explorer-close"
      onClick={onClose}
    >
      <IconX className="h-4 w-4" aria-hidden="true" />
    </Button>
  );
}

function ExplorerHeader({ onClose, mobile }: { onClose: () => void; mobile: boolean }) {
  const { t } = useTranslation();
  const content = (
    <>
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          {mobile ? (
            <DrawerTitle>{t("task:mcpExplorerTitle")}</DrawerTitle>
          ) : (
            <DialogTitle>{t("task:mcpExplorerTitle")}</DialogTitle>
          )}
          {mobile ? (
            <DrawerDescription>{t("task:mcpExplorerDescription")}</DrawerDescription>
          ) : (
            <DialogDescription>{t("task:mcpExplorerDescription")}</DialogDescription>
          )}
        </div>
        <ExplorerCloseButton onClose={onClose} />
      </div>
    </>
  );
  return mobile ? (
    <DrawerHeader className="shrink-0 border-b text-left">{content}</DrawerHeader>
  ) : (
    <DialogHeader className="shrink-0">{content}</DialogHeader>
  );
}

function ExplorerBody({
  servers,
  selectedName,
  onSelect,
  onBack,
  mobileDetail,
  touch,
}: {
  servers: MCPAttachmentServer[];
  selectedName: string | null;
  onSelect: (name: string) => void;
  onBack: () => void;
  mobileDetail: boolean;
  touch: boolean;
}) {
  const selectedServer = servers.find((server) => server.name === selectedName) ?? null;
  return (
    <div className="min-h-0 flex-1 overflow-hidden text-sm">
      {!touch && (
        <div className="grid h-full min-h-0 grid-cols-[15rem_minmax(0,1fr)] gap-4">
          <div className="min-h-0 overflow-y-auto border-r border-border/70 pr-3">
            <McpServerList servers={servers} selectedName={selectedName} onSelect={onSelect} />
          </div>
          <div className="min-h-0 overflow-y-auto pr-1">
            <McpServerDetail server={selectedServer} />
          </div>
        </div>
      )}
      {touch && (
        <div className="h-full min-h-0">
          {mobileDetail ? (
            <div
              data-testid="mcp-server-detail-scroll"
              className="h-full min-h-0 overflow-y-auto pr-1"
            >
              <McpServerDetail server={selectedServer} onBack={onBack} />
            </div>
          ) : (
            <McpServerList servers={servers} selectedName={selectedName} onSelect={onSelect} />
          )}
        </div>
      )}
    </div>
  );
}

export function McpServerExplorer({
  trigger,
  servers,
}: {
  trigger: ReactNode;
  servers: MCPAttachmentServer[];
}) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const usesTouchSurface = isMobile || !isFinePointer;
  const [open, setOpen] = useState(false);
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [mobileDetail, setMobileDetail] = useState(false);
  const fallbackSelection = useMemo(() => selectMcpServerName(servers), [servers]);
  const currentSelection = useMemo(
    () => selectMcpServerName(servers, selectedName),
    [servers, selectedName],
  );

  useEffect(() => {
    setSelectedName((current) => selectMcpServerName(servers, current));
  }, [servers]);

  useEffect(() => {
    if (!open) return;
    setSelectedName(fallbackSelection);
    setMobileDetail(false);
  }, [fallbackSelection, open]);

  const selectServer = (name: string) => {
    setSelectedName(name);
    if (usesTouchSurface) setMobileDetail(true);
  };
  const close = () => setOpen(false);
  const body = (
    <ExplorerBody
      servers={servers}
      selectedName={currentSelection}
      onSelect={selectServer}
      onBack={() => setMobileDetail(false)}
      mobileDetail={mobileDetail}
      touch={usesTouchSurface}
    />
  );

  if (usesTouchSurface) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          data-testid="mcp-server-explorer"
          className={cn(
            "flex flex-col overflow-hidden pb-[env(safe-area-inset-bottom)]",
            isMobile ? "!max-h-[100dvh] h-[100dvh]" : "max-h-[80dvh]",
          )}
        >
          <ExplorerHeader onClose={close} mobile />
          <div className="flex min-h-0 flex-1 flex-col px-4 pb-4">{body}</div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent
        data-testid="mcp-server-explorer"
        enterConfirms={false}
        className="flex max-h-[85dvh] min-h-[min(34rem,85dvh)] flex-col overflow-hidden sm:max-w-4xl"
      >
        <ExplorerHeader onClose={close} mobile={false} />
        {body}
      </DialogContent>
    </Dialog>
  );
}
