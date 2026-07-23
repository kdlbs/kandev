import "./globals.css";
import { ThemeProvider } from "@/components/theme-provider";
import { StateProvider } from "@/components/state-provider";
import { WebSocketConnector } from "@/components/ws-connector";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { Toaster as SonnerToaster } from "@kandev/ui/sonner";
import { CommandRegistryProvider } from "@/lib/commands/command-registry";
import { CommandPanel } from "@/components/command-panel";
import { GlobalCommands } from "@/components/global-commands";
import { RecentTaskSwitcher } from "@/components/task/recent-task-switcher";
import { DiffWorkerPoolProvider } from "@/components/diff-worker-pool-provider";
import { AppSidebar } from "@/components/app-sidebar/app-sidebar";
import { AppStatusSurfaceProvider } from "@/components/app-status-bar/app-status-surface-provider";
import { QuickChatProvider } from "@/components/quick-chat/quick-chat-provider";
import { ConfigChatProvider } from "@/components/config-chat/config-chat-provider";
import { SessionFailureToastBridge } from "@/components/session-failure-toast-bridge";
import { SidebarViewsSyncBridge } from "@/components/sidebar-views-sync-bridge";
import { LogBufferBridge } from "@/components/log-buffer-bridge";
import { getFeatureFlagsAction, getRuntimeDebugModeAction } from "@/app/actions/features";

export const metadata = {
  title: "Kandev - AI Kanban",
  description: "AI-powered workflow management for developers",
};

export const viewport = {
  // Enable safe area insets for iOS devices (notch, home indicator)
  viewportFit: "cover",
  // Prevent iOS auto-zoom on input focus (for app-like experience)
  maximumScale: 1,
  userScalable: false,
};

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const envDebugMode = process.env.KANDEV_DEBUG === "true";

  // SSR-fetch the deployment's feature flags so the entire client tree
  // (including the sidebar nav and gated routes) renders with the correct
  // visibility on the first paint. Falls back to all-off when the backend
  // is unreachable. See docs/decisions/0007-runtime-feature-flags.md.
  const [features, runtimeDebugMode] = await Promise.all([
    getFeatureFlagsAction(),
    getRuntimeDebugModeAction(),
  ]);
  const debugMode = envDebugMode || runtimeDebugMode;

  const runtimeConfigScript = debugMode ? "window.__KANDEV_DEBUG = true;" : null;

  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <meta name="apple-mobile-web-app-title" content="Kandev" />
        {/* Inject runtime config before app chunks so debug UI flags
            are visible when client modules first evaluate. */}
        {runtimeConfigScript ? (
          <script dangerouslySetInnerHTML={{ __html: runtimeConfigScript }} />
        ) : null}
        {/* Preload the Seti icon webfont so file-tree glyphs (review dialog,
            file browser) don't flash blank on first render. */}
        <link
          rel="preload"
          href="/fonts/seti/seti.woff"
          as="font"
          type="font/woff"
          crossOrigin="anonymous"
        />
      </head>
      <body className="antialiased font-sans">
        <StateProvider initialState={{ features }}>
          <ThemeProvider>
            <DiffWorkerPoolProvider>
              <TooltipProvider>
                <ToastProvider>
                  <SonnerToaster richColors position="top-right" />
                  <SessionFailureToastBridge />
                  <SidebarViewsSyncBridge />
                  <LogBufferBridge />
                  <CommandRegistryProvider>
                    <WebSocketConnector />
                    <GlobalCommands />
                    <CommandPanel />
                    <RecentTaskSwitcher />
                    <ConfigChatProvider>
                      <QuickChatProvider>
                        <AppStatusSurfaceProvider>
                          <div className="flex min-h-0 flex-1 overflow-hidden">
                            <AppSidebar />
                            <div className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
                          </div>
                        </AppStatusSurfaceProvider>
                      </QuickChatProvider>
                    </ConfigChatProvider>
                  </CommandRegistryProvider>
                </ToastProvider>
              </TooltipProvider>
            </DiffWorkerPoolProvider>
          </ThemeProvider>
        </StateProvider>
      </body>
    </html>
  );
}
