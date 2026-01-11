import type { Metadata } from "next";
import "./globals.css";
import { ThemeProvider } from "@/components/theme-provider";
import { StateProvider } from "@/components/state-provider";
import { WebSocketConnector } from "@/components/ws-connector";

export const metadata: Metadata = {
  title: "Kandev - AI Kanban",
  description: "AI-powered kanban board for developers",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="antialiased font-sans">
        <StateProvider>
          <ThemeProvider>
            <WebSocketConnector />
            {children}
          </ThemeProvider>
        </StateProvider>
      </body>
    </html>
  );
}
