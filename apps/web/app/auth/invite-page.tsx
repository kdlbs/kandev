"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { IconUserPlus } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import { acceptInvite, previewInvite, type InvitePreview } from "@/lib/api/domains/auth-api";

type InvitePageProps = {
  token?: string;
};

function useInvitePreview(token: string | undefined) {
  const [preview, setPreview] = useState<InvitePreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!token) {
      setPreviewError("This invite link is missing its token.");
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    previewInvite(token)
      .then((result) => {
        if (cancelled) return;
        setPreview(result);
      })
      .catch((err) => {
        if (cancelled) return;
        setPreviewError(
          err instanceof ApiError
            ? "This invite link is invalid or has expired. Ask an admin for a new one."
            : "Could not load this invite. Please try again.",
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  return { preview, previewError, loading };
}

type AcceptFormProps = {
  displayName: string;
  setDisplayName: (v: string) => void;
  email: string;
  setEmail: (v: string) => void;
  emailReadOnly: boolean;
  password: string;
  setPassword: (v: string) => void;
  error: string | null;
  submitting: boolean;
  onSubmit: (e: React.FormEvent) => void;
};

function AcceptForm({
  displayName,
  setDisplayName,
  email,
  setEmail,
  emailReadOnly,
  password,
  setPassword,
  error,
  submitting,
  onSubmit,
}: AcceptFormProps) {
  return (
    <form className="flex flex-col gap-3" onSubmit={onSubmit}>
      <div className="flex flex-col gap-1">
        <label htmlFor="invite-display-name" className="text-xs text-muted-foreground">
          Display name
        </label>
        <Input
          id="invite-display-name"
          data-testid="invite-display-name"
          autoComplete="name"
          required
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor="invite-email" className="text-xs text-muted-foreground">
          Email
        </label>
        <Input
          id="invite-email"
          data-testid="invite-email"
          type="email"
          autoComplete="email"
          required
          readOnly={emailReadOnly}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor="invite-password" className="text-xs text-muted-foreground">
          Password
        </label>
        <Input
          id="invite-password"
          data-testid="invite-password"
          type="password"
          autoComplete="new-password"
          required
          minLength={8}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </div>
      {error && (
        <p className="text-xs text-destructive" data-testid="invite-error">
          {error}
        </p>
      )}
      <Button
        type="submit"
        className="cursor-pointer"
        disabled={submitting}
        data-testid="invite-accept-submit"
      >
        {submitting ? "Joining..." : "Accept invite"}
      </Button>
    </form>
  );
}

export function InvitePage({ token }: InvitePageProps) {
  const { preview, previewError, loading } = useInvitePreview(token);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (preview?.email) setEmail(preview.email);
  }, [preview]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!token) return;
    setError(null);
    setSubmitting(true);
    try {
      await acceptInvite({ token, email, password, display_name: displayName });
      window.location.assign("/");
    } catch (err) {
      setSubmitting(false);
      setError(
        err instanceof ApiError
          ? (err.message ?? "Could not accept this invite.")
          : "Something went wrong. Please try again.",
      );
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconUserPlus className="h-4 w-4" /> Accept invite
          </CardTitle>
          <CardDescription>
            {preview ? `You've been invited as ${preview.role}.` : "Join this Kandev deployment."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading && (
            <p className="text-sm text-muted-foreground" data-testid="invite-loading">
              Checking invite...
            </p>
          )}
          {!loading && previewError && (
            <p className="text-sm text-destructive" data-testid="invite-preview-error">
              {previewError}
            </p>
          )}
          {!loading && !previewError && (
            <AcceptForm
              displayName={displayName}
              setDisplayName={setDisplayName}
              email={email}
              setEmail={setEmail}
              emailReadOnly={Boolean(preview?.email)}
              password={password}
              setPassword={setPassword}
              error={error}
              submitting={submitting}
              onSubmit={(e) => void onSubmit(e)}
            />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
