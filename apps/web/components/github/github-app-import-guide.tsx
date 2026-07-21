"use client";

import { useState } from "react";
import { IconCheck, IconCopy, IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type { PrepareGitHubAppImportResponse } from "@/lib/types/github";
import { GitHubAppPolicyDialog } from "./github-app-policy-dialog";

const urlLabels: [keyof PrepareGitHubAppImportResponse, string][] = [
  ["public_base_url", "Homepage URL"],
  ["personal_callback_url", "User authorization callback URL"],
  ["setup_url", "Setup URL"],
  ["webhook_url", "Webhook URL"],
];

export function GitHubAppImportGuide({
  preparation,
  settingsUrl,
}: {
  preparation: PrepareGitHubAppImportResponse;
  settingsUrl?: string;
}) {
  const { copy } = useCopyToClipboard();
  const [copied, setCopied] = useState("");
  async function copyValue(value: string) {
    await copy(value);
    setCopied(value);
  }
  return (
    <section className="space-y-3" aria-label="GitHub App configuration instructions">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">Configure the existing App on GitHub</h3>
        <p className="text-xs leading-5 text-muted-foreground">
          Set these exact URLs, create a client secret and webhook secret, and download a private
          key. This one-time setup expires {new Date(preparation.expires_at).toLocaleString()}.
        </p>
        <p className="text-xs leading-5 text-muted-foreground">
          For webhooks, choose <strong>application/json</strong> as the content type and keep SSL
          verification enabled.
        </p>
      </div>
      <div className="divide-y rounded-md border">
        {urlLabels.map(([key, label]) => {
          const value = String(preparation[key]);
          return (
            <div key={key} className="space-y-1 p-3">
              <div className="text-xs font-medium text-muted-foreground">{label}</div>
              <div className="flex min-w-0 items-center gap-2">
                <code className="min-w-0 flex-1 break-all text-xs">{value}</code>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-11 w-11 shrink-0 cursor-pointer"
                  aria-label={`Copy ${label}`}
                  onClick={() => void copyValue(value)}
                >
                  {copied === value ? (
                    <IconCheck className="h-4 w-4 text-green-500" />
                  ) : (
                    <IconCopy className="h-4 w-4" />
                  )}
                </Button>
              </div>
            </div>
          );
        })}
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        {settingsUrl ? (
          <Button asChild variant="outline" className="h-11 cursor-pointer">
            <a href={settingsUrl} target="_blank" rel="noreferrer">
              Open GitHub App settings
              <IconExternalLink className="ml-2 h-4 w-4" />
            </a>
          </Button>
        ) : (
          <Button type="button" variant="outline" className="h-11" disabled>
            Open GitHub App settings
            <IconExternalLink className="ml-2 h-4 w-4" />
          </Button>
        )}
        <GitHubAppPolicyDialog permissions={preparation.permissions} events={preparation.events} />
      </div>
    </section>
  );
}
