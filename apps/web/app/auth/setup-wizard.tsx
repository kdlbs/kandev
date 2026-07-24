"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { IconShieldLock } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import { setup } from "@/lib/api/domains/auth-api";

type SetupFormProps = {
  email: string;
  setEmail: (v: string) => void;
  password: string;
  setPassword: (v: string) => void;
  displayName: string;
  setDisplayName: (v: string) => void;
  error: string | null;
  submitting: boolean;
  onSubmit: (e: React.FormEvent) => void;
};

function SetupForm({
  email,
  setEmail,
  password,
  setPassword,
  displayName,
  setDisplayName,
  error,
  submitting,
  onSubmit,
}: SetupFormProps) {
  return (
    <form className="flex flex-col gap-3" onSubmit={onSubmit}>
      <div className="flex flex-col gap-1">
        <label htmlFor="setup-display-name" className="text-xs text-muted-foreground">
          Display name
        </label>
        <Input
          id="setup-display-name"
          data-testid="setup-display-name"
          autoComplete="name"
          required
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor="setup-email" className="text-xs text-muted-foreground">
          Email
        </label>
        <Input
          id="setup-email"
          data-testid="setup-email"
          type="email"
          autoComplete="email"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor="setup-password" className="text-xs text-muted-foreground">
          Password
        </label>
        <Input
          id="setup-password"
          data-testid="setup-password"
          type="password"
          autoComplete="new-password"
          required
          minLength={8}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <p className="text-[11px] text-muted-foreground">At least 8 characters.</p>
      </div>
      {error && (
        <p className="text-xs text-destructive" data-testid="setup-error">
          {error}
        </p>
      )}
      <Button
        type="submit"
        className="cursor-pointer"
        disabled={submitting}
        data-testid="setup-submit"
      >
        {submitting ? "Creating admin account..." : "Create admin account"}
      </Button>
    </form>
  );
}

export function SetupWizard() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await setup({ email, password, display_name: displayName });
      // Full reload re-fetches the boot payload with the new session cookie.
      window.location.assign("/");
    } catch (err) {
      setSubmitting(false);
      if (err instanceof ApiError) {
        setError(
          err.status === 409
            ? "Setup is no longer available — an account already exists."
            : (err.message ?? "Setup failed. Please try again."),
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
            <IconShieldLock className="h-4 w-4" /> Set up Kandev
          </CardTitle>
          <CardDescription>
            Authentication is enabled for this deployment. Create the first account below — it
            becomes the admin and can invite additional users.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <SetupForm
            email={email}
            setEmail={setEmail}
            password={password}
            setPassword={setPassword}
            displayName={displayName}
            setDisplayName={setDisplayName}
            error={error}
            submitting={submitting}
            onSubmit={(e) => void onSubmit(e)}
          />
        </CardContent>
      </Card>
    </div>
  );
}
