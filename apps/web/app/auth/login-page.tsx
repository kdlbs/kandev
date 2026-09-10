"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { IconLock } from "@tabler/icons-react";
import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";
import { login } from "@/lib/api/domains/auth-api";
import { useAppStore } from "@/components/state-provider";
import type { SsoProvider } from "@/lib/state/slices/auth/types";
import { useTranslation } from "react-i18next";

// LoginSsoButtons renders one "Continue with <provider>" button per
// plugin-contributed SSO provider, below a divider. Each button is a plain
// navigation to the plugin's login-initiate webhook.
function LoginSsoButtons({ providers }: { providers: SsoProvider[] }) {
  const { t } = useTranslation();
  if (providers.length === 0) return null;
  return (
    <div className="mt-4 flex flex-col gap-2" data-testid="login-sso">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span className="h-px flex-1 bg-border" />
        {t("auth:orContinueWith")}
        <span className="h-px flex-1 bg-border" />
      </div>
      {providers.map((provider) => (
        <Button
          key={provider.id}
          asChild
          variant="outline"
          className="cursor-pointer"
          data-testid={`login-sso-${provider.id}`}
        >
          <a href={provider.initiateUrl}>
            {t("auth:continueWithProvider", { provider: provider.displayName })}
          </a>
        </Button>
      ))}
    </div>
  );
}

export function LoginPage() {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const ssoProviders = useAppStore((s) => s.auth.ssoProviders) ?? [];

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await login({ email, password });
      // Full reload re-fetches the boot payload with the new session cookie —
      // do not try to hot-swap store state after login.
      window.location.assign("/");
    } catch (err) {
      setSubmitting(false);
      if (err instanceof ApiError) {
        setError(loginErrorMessage(err, t));
        return;
      }
      setError(t("auth:somethingWentWrong"));
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconLock className="h-4 w-4" /> {t("auth:signIn")}
          </CardTitle>
          <CardDescription>{t("auth:signInToYourKandevAccount")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-3" onSubmit={(e) => void onSubmit(e)}>
            <div className="flex flex-col gap-1">
              <label htmlFor="login-email" className="text-xs text-muted-foreground">
                {t("auth:email")}
              </label>
              <Input
                id="login-email"
                data-testid="login-email"
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1">
              <label htmlFor="login-password" className="text-xs text-muted-foreground">
                {t("auth:password")}
              </label>
              <Input
                id="login-password"
                data-testid="login-password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {error && (
              <p className="text-xs text-destructive" data-testid="login-error">
                {error}
              </p>
            )}
            <Button
              type="submit"
              className="cursor-pointer"
              disabled={submitting}
              data-testid="login-submit"
            >
              {submitting ? t("auth:signingIn") : t("auth:signIn")}
            </Button>
          </form>
          <LoginSsoButtons providers={ssoProviders} />
        </CardContent>
      </Card>
    </div>
  );
}

/**
 * Maps a failed sign-in to the message that tells the user what to actually do.
 *
 * A suspended organization must NOT read as a bad password: the credential was
 * accepted, and saying otherwise sends the user to reset a password that works.
 */
function loginErrorMessage(err: ApiError, t: TFunction): string {
  if (err.status === 429) return t("auth:tooManyAttempts");
  const code = (err.body as { code?: string } | null)?.code;
  if (code === "org_suspended") return t("auth:organizationUnavailable");
  return t("auth:invalidEmailOrPassword");
}
