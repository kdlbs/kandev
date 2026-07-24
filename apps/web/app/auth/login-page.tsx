"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { IconLock } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import { login } from "@/lib/api/domains/auth-api";

export function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

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
        setError(
          err.status === 429
            ? "Too many attempts. Please wait a moment and try again."
            : "Invalid email or password.",
        );
        return;
      }
      setError("Something went wrong. Please try again.");
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconLock className="h-4 w-4" /> Sign in
          </CardTitle>
          <CardDescription>Sign in to your Kandev account to continue.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-3" onSubmit={(e) => void onSubmit(e)}>
            <div className="flex flex-col gap-1">
              <label htmlFor="login-email" className="text-xs text-muted-foreground">
                Email
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
                Password
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
              {submitting ? "Signing in..." : "Sign in"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
