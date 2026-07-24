import { InvitePage } from "@/app/auth/invite-page";
import { LoginPage } from "@/app/auth/login-page";
import { SetupWizard } from "@/app/auth/setup-wizard";
import { ThemeProvider } from "@/components/theme-provider";
import { ToastProvider } from "@/components/toast-provider";
import { Toaster as SonnerToaster } from "@kandev/ui/sonner";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { usePathname, useSearchParams } from "@/lib/routing/client-router";

/**
 * Mounts the same theme/tooltip/toast chrome AppShell gives first-party
 * pages, without WebSocketConnector/AppSidebar/CommandRegistry — those all
 * assume an authenticated session and must never mount pre-auth.
 */
function AuthScreens({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider>
      <TooltipProvider>
        <ToastProvider>
          <SonnerToaster richColors position="top-right" />
          {children}
        </ToastProvider>
      </TooltipProvider>
    </ThemeProvider>
  );
}

/**
 * Decides, before AppShell/SpaRoutes ever mount, whether the visitor needs
 * the setup wizard, the login/invite screen, or the normal authenticated app
 * shell. See apps/backend/internal/auth/state.go StatePayload for the
 * mode/authenticated contract this mirrors.
 */
export function useAuthGateDecision(): "setup" | "login" | "invite" | "app" {
  const mode = useAppStore((s) => s.auth.mode);
  const authenticated = useAppStore((s) => s.auth.authenticated);
  const pathname = usePathname();

  if (mode === "setup") return "setup";
  if (mode === "enabled" && !authenticated) {
    return pathname === "/invite" ? "invite" : "login";
  }
  return "app";
}

export function AuthGatedScreen({ decision }: { decision: "setup" | "login" | "invite" }) {
  const searchParams = useSearchParams();
  return (
    <AuthScreens>
      {decision === "setup" && <SetupWizard />}
      {decision === "login" && <LoginPage />}
      {decision === "invite" && <InvitePage token={searchParams.get("token") ?? undefined} />}
    </AuthScreens>
  );
}
