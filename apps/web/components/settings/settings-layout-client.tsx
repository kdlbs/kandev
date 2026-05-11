"use client";

import { usePathname } from "next/navigation";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@kandev/ui/sidebar";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { PageTopbar } from "@/components/page-topbar";
import { SettingsAppSidebar } from "@/components/settings/settings-app-sidebar";

export function SettingsLayoutClient({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const isAgentDetail = pathname.startsWith("/settings/agents/") && pathname !== "/settings/agents";
  const backHref = isAgentDetail ? "/settings/agents" : "/";
  const backLabel = isAgentDetail ? "Agents" : "Kandev";
  const title = isAgentDetail ? "Agent" : "Settings";

  return (
    <TooltipProvider>
      <SidebarProvider>
        <SettingsAppSidebar />
        <SidebarInset>
          <PageTopbar
            title={title}
            backHref={backHref}
            backLabel={backLabel}
            className="h-16 border-b-0"
            leading={<SidebarTrigger size="lg" className="md:hidden h-10 w-10 cursor-pointer" />}
          />
          <div className="flex min-w-0 flex-1 flex-col gap-4 p-4 pt-0 mb-20">{children}</div>
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  );
}
