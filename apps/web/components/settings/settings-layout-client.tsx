'use client';

import { IconArrowLeft } from '@tabler/icons-react';
import { usePathname } from 'next/navigation';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
} from '@kandev/ui/breadcrumb';
import { SidebarInset, SidebarProvider } from '@kandev/ui/sidebar';
import { SettingsAppSidebar } from '@/components/settings/settings-app-sidebar';
import { TooltipProvider } from '@kandev/ui/tooltip';

export function SettingsLayoutClient({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const isAgentDetail = pathname.startsWith('/settings/agents/') && pathname !== '/settings/agents';
  const breadcrumbLabel = isAgentDetail ? 'Agents' : 'Home';
  const breadcrumbHref = isAgentDetail ? '/settings/agents' : '/';

  return (
    <TooltipProvider>
      <SidebarProvider>
        <SettingsAppSidebar />
        <SidebarInset>
          <header className="flex h-16 shrink-0 items-center gap-2">
            <div className="flex items-center gap-2 px-4">
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
          <div className="flex flex-1 flex-col gap-4 p-4 pt-0">
            {children}
          </div>
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  );
}
