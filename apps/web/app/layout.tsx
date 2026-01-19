import type { Metadata } from "next";
import "./globals.css";
import { ThemeProvider } from "@/components/theme-provider";
import { StateProvider } from "@/components/state-provider";
import { WebSocketConnector } from "@/components/ws-connector";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";

export const metadata: Metadata = {
  title: "Kandev - AI Kanban",
  description: "AI-powered kanban board for developers",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const runtimeApiBaseUrl = process.env.KANDEV_API_BASE_URL ?? "";
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="antialiased font-sans">
        {runtimeApiBaseUrl ? (
          // Inject runtime API base URL for production bundles where ports are chosen at launch.
          <script
            dangerouslySetInnerHTML={{
              __html: `window.__KANDEV_API_BASE_URL = ${JSON.stringify(runtimeApiBaseUrl)};`,
            }}
          />
        ) : null}
        <StateProvider>
          <ThemeProvider>
            <TooltipProvider>
              <ToastProvider>
                <WebSocketConnector />
                {children}
              </ToastProvider>
            </TooltipProvider>
          </ThemeProvider>
        </StateProvider>
      </body>
    </html>
  );
}
