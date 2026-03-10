"use client";

import { IconArrowLeft, IconSparkles } from "@tabler/icons-react";
import { usePathname } from "next/navigation";
import { Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList } from "@kandev/ui/breadcrumb";
import { Button } from "@kandev/ui/button";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@kandev/ui/sidebar";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { SettingsAppSidebar } from "@/components/settings/settings-app-sidebar";
import { useAppStore } from "@/components/state-provider";
import { ConfigChatModal } from "@/components/config-chat/config-chat-modal";
import { useConfigChat } from "@/components/config-chat/use-config-chat";

function ConfigChatFAB() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { open } = useConfigChat(workspaceId ?? "");

  if (!workspaceId) return null;

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="icon"
            onClick={() => open()}
            className="fixed bottom-6 right-6 z-50 h-14 w-14 rounded-full shadow-lg cursor-pointer"
          >
            <IconSparkles className="h-6 w-6" />
            <span className="sr-only">AI Config Chat</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent side="left">
          <p className="font-medium">AI Config Chat</p>
          <p className="text-xs text-muted-foreground">Configure Kandev with natural language</p>
        </TooltipContent>
      </Tooltip>
      <ConfigChatModal workspaceId={workspaceId} />
    </>
  );
}

export function SettingsLayoutClient({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const isAgentDetail = pathname.startsWith("/settings/agents/") && pathname !== "/settings/agents";
  const breadcrumbLabel = isAgentDetail ? "Agents" : "Home";
  const breadcrumbHref = isAgentDetail ? "/settings/agents" : "/";

  return (
    <TooltipProvider>
      <SidebarProvider>
        <SettingsAppSidebar />
        <SidebarInset>
          <header className="flex h-16 shrink-0 items-center gap-2">
            <div className="flex items-center gap-2 px-0 sm:px-4">
              <SidebarTrigger size="lg" className="md:hidden h-10 w-10 cursor-pointer" />
              <Breadcrumb>
                <BreadcrumbList>
                  <BreadcrumbItem>
                    <BreadcrumbLink href={breadcrumbHref} className="flex items-center gap-2">
                      <IconArrowLeft className="h-4 w-4" />
                      {breadcrumbLabel}
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
            </div>
          </header>
          <div className="flex flex-1 flex-col gap-4 p-4 pt-0">{children}</div>
          <ConfigChatFAB />
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  );
}
