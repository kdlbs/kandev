import { AppSidebar } from "@/components/app-sidebar/app-sidebar";
import { CommandPanel } from "@/components/command-panel";
import { ConfigChatProvider } from "@/components/config-chat/config-chat-provider";
import { DiffWorkerPoolProvider } from "@/components/diff-worker-pool-provider";
import { GlobalCommands } from "@/components/global-commands";
import { LogBufferBridge } from "@/components/log-buffer-bridge";
import { QuickChatProvider } from "@/components/quick-chat/quick-chat-provider";
import { RecentTaskSwitcher } from "@/components/task/recent-task-switcher";
import { SessionFailureToastBridge } from "@/components/session-failure-toast-bridge";
import { TaskDeletedToastBridge } from "@/components/task-deleted-toast-bridge";
import { SidebarViewsSyncBridge } from "@/components/sidebar-views-sync-bridge";
import { TelemetryPageViewBridge } from "@/components/telemetry-page-view-bridge";
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
            <TelemetryPageViewBridge />
            <LogBufferBridge />
            <CommandRegistryProvider>
              <WebSocketConnector />
              <GlobalCommands />
              <CommandPanel />
              <RecentTaskSwitcher />
              <ConfigChatProvider>
                <QuickChatProvider>
                  <div className="flex h-screen min-h-0 w-full overflow-hidden">
                    <AppSidebar />
                    <main className="flex-1 min-w-0 min-h-0 flex flex-col">{children}</main>
                  </div>
                </QuickChatProvider>
              </ConfigChatProvider>
            </CommandRegistryProvider>
          </ToastProvider>
        </TooltipProvider>
      </DiffWorkerPoolProvider>
    </ThemeProvider>
  );
}
