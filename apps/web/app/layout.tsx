import type { Metadata, Viewport } from "next";
import "./globals.css";
import { ThemeProvider } from "@/components/theme-provider";
import { StateProvider } from "@/components/state-provider";
import { WebSocketConnector } from "@/components/ws-connector";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { CommandRegistryProvider } from "@/lib/commands/command-registry";
import { CommandPanel } from "@/components/command-panel";
import { GlobalCommands } from "@/components/global-commands";
import { DiffWorkerPoolProvider } from "@/components/diff-worker-pool-provider";
import { SessionFailureToastBridge } from "@/components/session-failure-toast-bridge";

export const metadata: Metadata = {
  title: "KanDev - AI Kanban",
  description: "AI-powered workflow management for developers",
};

export const viewport: Viewport = {
  // Enable safe area insets for iOS devices (notch, home indicator)
  viewportFit: "cover",
  // Prevent iOS auto-zoom on input focus (for app-like experience)
  maximumScale: 1,
  userScalable: false,
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  // API port injection for dev mode (browser opens at web port, API on backend port).
  // In production single-port mode, this is not needed — the client uses
  // window.location.origin (same-origin, works for any domain / reverse proxy).
  const apiPort = process.env.NEXT_PUBLIC_KANDEV_API_PORT ?? null;
  const debugMode = process.env.NEXT_PUBLIC_KANDEV_DEBUG === "true";

  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <meta name="apple-mobile-web-app-title" content="KanDev" />
      </head>
      <body className="antialiased font-sans">
        {apiPort || debugMode ? (
          <script
            dangerouslySetInnerHTML={{
              __html: [
                apiPort ? `window.__KANDEV_API_PORT = ${JSON.stringify(apiPort)};` : "",
                debugMode ? `window.__KANDEV_DEBUG = true;` : "",
              ]
                .filter(Boolean)
                .join("\n"),
            }}
          />
        ) : null}
        <StateProvider>
          <ThemeProvider>
            <DiffWorkerPoolProvider>
              <TooltipProvider>
                <ToastProvider>
                  <SessionFailureToastBridge />
                  <CommandRegistryProvider>
                    <WebSocketConnector />
                    <GlobalCommands />
                    <CommandPanel />
                    {children}
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
