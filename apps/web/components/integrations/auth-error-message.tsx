"use client";

import Link from "next/link";
import { IconLockExclamation } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

// Drops support URLs upstream APIs sometimes inline into error response
// bodies — they're noise once the user has a clear CTA. Shared because both
// jira and linear surface verbose, URL-laden messages.
const URL_RE = /\bhttps?:\/\/\S+/g;

export function cleanIntegrationErrorMessage(error: string): string {
  return error.replace(URL_RE, "").replace(/\s+/g, " ").trim();
}

export type IntegrationAuthErrorMessageProps = {
  error: string;
  // Display name shown in the headline / button label, e.g. "Jira", "Linear".
  name: string;
  // Where the "Reconnect" button takes the user; null/undefined falls back
  // to the global settings page.
  reconnectHref: string;
  // Integration-specific check for whether the error is an auth failure
  // (each integration's API formats status codes differently).
  isAuthError: (error: string) => boolean;
  // Long-form copy shown under the headline in the non-compact variant.
  // Integration-specific because the user-facing remediation differs
  // (session vs API key, etc.).
  authErrorBody: string;
  // Inline variant for cases where context is already rendered above.
  compact?: boolean;
};

// IntegrationAuthErrorMessage renders the standard error UI shown when an
// integration request fails. Auth failures get a Reconnect CTA pointing at
// the workspace's settings page; everything else just shows the cleaned
// upstream message.
export function IntegrationAuthErrorMessage({
  error,
  name,
  reconnectHref,
  isAuthError,
  authErrorBody,
  compact,
}: IntegrationAuthErrorMessageProps) {
  const isAuth = isAuthError(error);

  if (compact) {
    return (
      <div className="flex items-center gap-3 text-sm">
        <span className={isAuth ? "text-muted-foreground" : "text-destructive"}>
          {isAuth ? `${name} authentication required.` : cleanIntegrationErrorMessage(error)}
        </span>
        {isAuth && (
          <Button asChild size="sm" variant="outline" className="cursor-pointer h-7 text-xs">
            <Link href={reconnectHref}>Reconnect {name}</Link>
          </Button>
        )}
      </div>
    );
  }

  return (
    <div className="max-w-md text-center space-y-4">
      {isAuth ? (
        <>
          <IconLockExclamation className="h-10 w-10 mx-auto text-muted-foreground" />
          <div className="space-y-1.5">
            <h2 className="text-lg font-semibold">{name} authentication required</h2>
            <p className="text-sm text-muted-foreground">{authErrorBody}</p>
          </div>
          <Button asChild size="sm" className="cursor-pointer">
            <Link href={reconnectHref}>Reconnect {name}</Link>
          </Button>
        </>
      ) : (
        <p className="text-sm text-destructive">{cleanIntegrationErrorMessage(error)}</p>
      )}
    </div>
  );
}
