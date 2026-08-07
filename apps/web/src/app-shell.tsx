import { useTranslation } from "react-i18next";

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
import { UpdateAvailableToastBridge } from "@/components/update-available-toast-bridge";
import { SidebarViewsSyncBridge } from "@/components/sidebar-views-sync-bridge";
import { ThemeProvider } from "@/components/theme-provider";
import { ToastProvider } from "@/components/toast-provider";
import { WebSocketConnector } from "@/components/ws-connector";
import { CommandRegistryProvider } from "@/lib/commands/command-registry";
import { I18nProvider } from "@/lib/i18n/provider";
import { Toaster as SonnerToaster } from "@kandev/ui/sonner";
import { TooltipProvider } from "@kandev/ui/tooltip";

type AppShellProps = {
  children: React.ReactNode;
};

/**
 * sonner names its toast region `Notifications alt+T` by default — copy a
 * screen-reader user hears on every screen, and the last un-migrated
 * `aria-label` the pseudo-coverage oracle reported.
 *
 * Deliberately its own component rather than a `t()` call in `AppShell`:
 * `<I18nProvider>` is mounted BY `AppShell`, so `AppShell`'s own render runs
 * outside the provider and would not re-render on a locale switch.
 */
function AppToaster() {
  const { t } = useTranslation();
  return (
    <SonnerToaster
      richColors
      position="top-right"
      containerAriaLabel={t("common:toastRegionLabel")}
    />
  );
}

export function AppShell({ children }: AppShellProps) {
  return (
    <I18nProvider>
      <ThemeProvider>
        <DiffWorkerPoolProvider>
          <TooltipProvider>
            <ToastProvider>
              <AppToaster />
              <SessionFailureToastBridge />
              <TaskDeletedToastBridge />
              <UpdateAvailableToastBridge />
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
                    <div className="flex h-dvh min-h-0 w-full overflow-hidden">
                      <AppSidebar />
                      <AppStatusSurfaceProvider>
                        <main className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</main>
                      </AppStatusSurfaceProvider>
                    </div>
                  </QuickChatProvider>
                </ConfigChatProvider>
              </CommandRegistryProvider>
            </ToastProvider>
          </TooltipProvider>
        </DiffWorkerPoolProvider>
      </ThemeProvider>
    </I18nProvider>
  );
}
