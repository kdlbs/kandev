import type { Metadata, Viewport } from "next";
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

export const viewport: Viewport = {
  // Enable safe area insets for iOS devices (notch, home indicator)
  viewportFit: "cover",
  // Prevent iOS auto-zoom on input focus (for app-like experience)
  maximumScale: 1,
  userScalable: false,
};

/**
 * Extract port from URL string (e.g., "http://localhost:8080" -> "8080")
 */
function extractPort(url: string): string | null {
  try {
    const parsed = new URL(url);
    return parsed.port || null;
  } catch {
    return null;
  }
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  // Extract ports from server-side env vars
  // We inject only ports, not full URLs, so client can build URLs from window.location.hostname
  // This allows accessing the app from any device (iPhone, Tailscale, etc.)
  const apiPort = extractPort(process.env.KANDEV_API_BASE_URL ?? "");
  const mcpPort = extractPort(process.env.KANDEV_MCP_SERVER_URL ?? "");

  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <meta name="apple-mobile-web-app-title" content="Kandev" />
      </head>
      <body className="antialiased font-sans">
        {(apiPort || mcpPort) ? (
          // Inject runtime ports for production bundles where ports are chosen at launch.
          // Client will build full URLs using window.location.hostname + port
          <script
            dangerouslySetInnerHTML={{
              __html: [
                apiPort ? `window.__KANDEV_API_PORT = ${JSON.stringify(apiPort)};` : '',
                mcpPort ? `window.__KANDEV_MCP_PORT = ${JSON.stringify(mcpPort)};` : '',
              ].filter(Boolean).join('\n'),
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
