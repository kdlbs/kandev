"use client";

import { useState, type ReactNode } from "react";
import { Dialog, DialogContent, DialogTrigger } from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerTrigger } from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { McpExplorerHeader } from "./mcp-explorer-header";
import { McpServerList } from "./mcp-server-list";
import { McpStatusTooltip } from "./mcp-status-presentation";
import { McpToolDetail } from "./mcp-tool-detail";
import { McpToolList } from "./mcp-tool-list";
import { useMcpExplorerNavigation } from "./use-mcp-explorer-navigation";

type Navigation = ReturnType<typeof useMcpExplorerNavigation>;

function ToolPages({ navigation, touch }: { navigation: Navigation; touch: boolean }) {
  const { t } = useTranslation();
  if (!navigation.server) {
    return (
      <div className="flex h-full items-center justify-center text-[13px] text-muted-foreground">
        {t("task:mcpNoServers")}
      </div>
    );
  }
  if (navigation.page === "tool" && navigation.tool) {
    return <McpToolDetail tool={navigation.tool} onBack={navigation.backToTools} />;
  }
  return (
    <McpToolList
      server={navigation.server}
      scrollRef={navigation.listScrollRef}
      onSelect={(name) => navigation.selectTool(name)}
      onBack={touch ? navigation.backToServers : undefined}
    />
  );
}

function ExplorerBody({
  servers,
  navigation,
  touch,
}: {
  servers: MCPAttachmentServer[];
  navigation: Navigation;
  touch: boolean;
}) {
  if (touch && navigation.page === "servers") {
    return (
      <div className="h-full min-h-0 overflow-hidden">
        <McpServerList
          servers={servers}
          selectedName={navigation.selectedName}
          onSelect={navigation.selectServer}
        />
      </div>
    );
  }
  if (touch) return <ToolPages navigation={navigation} touch />;
  return (
    <div className="grid h-full min-h-0 grid-cols-[15rem_minmax(0,1fr)] gap-4">
      <div className="min-h-0 overflow-hidden border-r border-border/70 pr-3">
        <McpServerList
          servers={servers}
          selectedName={navigation.selectedName}
          onSelect={navigation.selectServer}
        />
      </div>
      <div className="min-h-0 overflow-hidden pr-1">
        <ToolPages navigation={navigation} touch={false} />
      </div>
    </div>
  );
}

function DesktopExplorer({
  trigger,
  servers,
  open,
  setOpen,
  navigation,
}: {
  trigger: ReactNode;
  servers: MCPAttachmentServer[];
  open: boolean;
  setOpen: (open: boolean) => void;
  navigation: Navigation;
}) {
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <DialogTrigger asChild>{trigger}</DialogTrigger>
        </TooltipTrigger>
        <TooltipContent>
          <McpStatusTooltip servers={servers} />
        </TooltipContent>
      </Tooltip>
      <DialogContent
        data-testid="mcp-server-explorer"
        enterConfirms={false}
        showCloseButton={false}
        className="grid max-h-[85dvh] min-h-[min(34rem,85dvh)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden sm:max-w-4xl"
      >
        <McpExplorerHeader onClose={() => setOpen(false)} mobile={false} />
        <div className="min-h-0 overflow-hidden text-[13px] leading-5">
          <ExplorerBody servers={servers} navigation={navigation} touch={false} />
        </div>
      </DialogContent>
    </Dialog>
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
  const touch = isMobile || !isFinePointer;
  const [open, setOpen] = useState(false);
  const navigation = useMcpExplorerNavigation({ servers, open, touch });
  if (!touch) {
    return (
      <DesktopExplorer
        trigger={trigger}
        servers={servers}
        open={open}
        setOpen={setOpen}
        navigation={navigation}
      />
    );
  }
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent
        data-testid="mcp-server-explorer"
        className={cn(
          "flex flex-col overflow-hidden pb-[env(safe-area-inset-bottom)]",
          isMobile ? "h-[100dvh] !max-h-[100dvh]" : "max-h-[80dvh]",
        )}
      >
        <McpExplorerHeader onClose={() => setOpen(false)} mobile />
        <div className="min-h-0 flex-1 overflow-hidden px-4 pb-4 text-[13px] leading-5">
          <ExplorerBody servers={servers} navigation={navigation} touch />
        </div>
      </DrawerContent>
    </Drawer>
  );
}
