"use client";

import { useState } from "react";
import { IconCheck, IconCopy, IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type { PrepareGitHubAppImportResponse } from "@/lib/types/github";
import { GitHubAppPolicyDialog } from "./github-app-policy-dialog";
import { Trans, useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";

// These are the verbatim field names on GitHub's own "Register a new GitHub App"
// form, which is English-only. The guide tells the user which field to paste each
// value into, so translating them would break that mapping — they are references
// to another product's UI, not our copy.
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
  const { t } = useTranslation();
  const { copy } = useCopyToClipboard();
  const [copied, setCopied] = useState("");
  async function copyValue(value: string) {
    await copy(value);
    setCopied(value);
  }
  return (
    <section className="space-y-3" aria-label={t("github:githubAppConfigurationInstructions")}>
      <div className="space-y-1">
        <h3 className="text-sm font-medium">{t("github:configureTheExistingAppOnGithub")}</h3>
        <p className="text-xs leading-5 text-muted-foreground">
          {t("github:setTheseExactUrlsCreateAClientSecret", {
            expiresAt: formatDateTime(preparation.expires_at),
          })}
        </p>
        <p className="text-xs leading-5 text-muted-foreground">
          {/* The content-type value is a literal the user selects on GitHub, so it
              is interpolated rather than written into the catalog. */}
          <Trans
            i18nKey="github:forWebhooksChooseContentType"
            values={{ contentType: "application/json" }}
          >
            For webhooks, choose <strong>{"{{contentType}}"}</strong> as the content type and keep
            SSL verification enabled.
          </Trans>
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
                  aria-label={t("github:copy", { label })}
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
              {t("github:openGithubAppSettings")}
              <IconExternalLink className="ml-2 h-4 w-4" />
            </a>
          </Button>
        ) : (
          <Button type="button" variant="outline" className="h-11" disabled>
            {t("github:openGithubAppSettings")}
            <IconExternalLink className="ml-2 h-4 w-4" />
          </Button>
        )}
        <GitHubAppPolicyDialog permissions={preparation.permissions} events={preparation.events} />
      </div>
    </section>
  );
}
