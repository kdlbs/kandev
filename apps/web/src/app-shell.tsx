import { AppSidebar } from "@/components/app-sidebar/app-sidebar";
import { AppStatusSurfaceProvider } from "@/components/app-status-bar/app-status-surface-provider";
import { CommandPanel } from "@/components/command-panel";
import { ConfigChatProvider } from "@/components/config-chat/config-chat-provider";
import { DiffWorkerPoolProvider } from "@/components/diff-worker-pool-provider";
import { DesktopCommandHost } from "@/components/desktop-command-host";
import { GlobalCommands } from "@/components/global-commands";
import { LogBufferBridge } from "@/components/log-buffer-bridge";
import { QuickChatProvider } from "@/components/quick-chat/quick-chat-provider";
import { RecentTaskSwitcher } from "@/components/task/recent-task-switcher";
import { SessionFailureToastBridge } from "@/components/session-failure-toast-bridge";
import { TaskDeletedToastBridge } from "@/components/task-deleted-toast-bridge";
import { SidebarViewsSyncBridge } from "@/components/sidebar-views-sync-bridge";
import { ThemeProvider } from "@/components/theme-provider";
import { ToastProvider } from "@/components/toast-provider";
import { WebSocketConnector } from "@/components/ws-connector";
import { CommandRegistryProvider } from "@/lib/commands/command-registry";
import { Toaster as SonnerToaster } from "@kandev/ui/sonner";
import { TooltipProvider } from "@kandev/ui/tooltip";

type AppShellProps = {
  children: React.ReactNode;
};

export function AppShell({ children }: AppShellProps) {
  return (
    <ThemeProvider>
      <DiffWorkerPoolProvider>
        <TooltipProvider>
          <ToastProvider>
            <SonnerToaster richColors position="top-right" />
            <SessionFailureToastBridge />
            <TaskDeletedToastBridge />
            <SidebarViewsSyncBridge />
            <LogBufferBridge />
            <CommandRegistryProvider>
              <DesktopCommandHost />
              <WebSocketConnector />
              <GlobalCommands />
              <CommandPanel />
              <RecentTaskSwitcher />
              <ConfigChatProvider>
                <QuickChatProvider>
                  <AppStatusSurfaceProvider>
                    <div className="flex min-h-0 flex-1 overflow-hidden">
                      <AppSidebar />
                      <main className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</main>
                    </div>
                  </AppStatusSurfaceProvider>
                </QuickChatProvider>
              </ConfigChatProvider>
            </CommandRegistryProvider>
          </ToastProvider>
        </TooltipProvider>
      </DiffWorkerPoolProvider>
    </ThemeProvider>
  );
}
