"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { IconUserPlus } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import { acceptInvite, previewInvite, type InvitePreview } from "@/lib/api/domains/auth-api";
import { useTranslation } from "react-i18next";

type InvitePageProps = {
  token?: string;
};

function useInvitePreview(token: string | undefined) {
  const { t } = useTranslation();
  const [preview, setPreview] = useState<InvitePreview | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!token) {
      setPreviewError(t("auth:inviteLinkMissingToken"));
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
            ? t("auth:inviteLinkInvalidOrExpired")
            : t("auth:couldNotLoadInvite"),
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
  const { t } = useTranslation();
  return (
    <form className="flex flex-col gap-3" onSubmit={onSubmit}>
      <div className="flex flex-col gap-1">
        <label htmlFor="invite-display-name" className="text-xs text-muted-foreground">
          {t("auth:displayName")}
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
          {t("auth:email")}
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
          {t("auth:password")}
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
        {submitting ? t("auth:joining") : t("auth:acceptInvite")}
      </Button>
    </form>
  );
}

export function InvitePage({ token }: InvitePageProps) {
  const { t } = useTranslation();
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
          ? (err.message ?? t("auth:couldNotAcceptInvite"))
          : t("auth:somethingWentWrong"),
      );
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconUserPlus className="h-4 w-4" /> {t("auth:acceptInvite")}
          </CardTitle>
          <CardDescription>
            {preview
              ? t("auth:youVeBeenInvitedAs", { role: preview.role })
              : t("auth:joinThisKandevDeployment")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading && (
            <p className="text-sm text-muted-foreground" data-testid="invite-loading">
              {t("auth:checkingInvite")}
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
