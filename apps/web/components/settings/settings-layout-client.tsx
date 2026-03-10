"use client";

import { IconArrowLeft, IconMessageChatbot } from "@tabler/icons-react";
import { usePathname } from "next/navigation";
import { Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList } from "@kandev/ui/breadcrumb";
import { Button } from "@kandev/ui/button";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@kandev/ui/sidebar";
import { SettingsAppSidebar } from "@/components/settings/settings-app-sidebar";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { ConfigChatModal } from "@/components/config-chat/config-chat-modal";
import { useConfigChat } from "@/components/config-chat/use-config-chat";

function ConfigChatButton() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { open, isStarting } = useConfigChat(workspaceId ?? "");

  if (!workspaceId) return null;

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => open()}
        disabled={isStarting}
        className="cursor-pointer gap-2"
      >
        <IconMessageChatbot className="h-4 w-4" />
        Config Chat
      </Button>
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
          <header className="flex h-16 shrink-0 items-center gap-2 justify-between">
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
            <div className="flex items-center gap-2 px-4">
              <ConfigChatButton />
            </div>
          </header>
          <div className="flex flex-1 flex-col gap-4 p-4 pt-0">{children}</div>
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  );
}
