"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import type { PluginRecord } from "@/lib/types/plugins";
import { PluginPublisherIdentity } from "./plugin-publisher-identity";

export function PluginPublisherVerification({
  plugin,
  canManage,
  busy,
  error,
  success,
  onVerify,
}: {
  plugin: PluginRecord;
  canManage: boolean;
  busy: boolean;
  error?: string;
  success: boolean;
  onVerify: () => void;
}) {
  const { t } = useTranslation();
  const verified = plugin.publisher_identity?.status === "verified";
  const matchedSourceName =
    plugin.publisher_identity?.matched_source === "official"
      ? t("plugins:officialSourceName")
      : undefined;

  return (
    <section
      className="space-y-3 rounded-lg border border-border/70 p-4"
      data-testid="plugin-publisher-verification"
    >
      <PluginPublisherIdentity
        identity={plugin.publisher_identity}
        provenance={plugin.publisher_provenance}
        author={plugin.author}
        matchedSourceName={matchedSourceName}
      />
      {verified && success && (
        <p role="status" className="text-sm text-emerald-700 dark:text-emerald-400">
          {t("plugins:publisherVerificationSucceeded")}
        </p>
      )}
      {!verified && canManage && (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant="outline"
            className={controlSizingClassName(
              "compact",
              "cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11",
            )}
            disabled={busy || !plugin.installation_id}
            aria-busy={busy ? "true" : undefined}
            onClick={onVerify}
            data-testid="plugin-verify-publisher"
          >
            {busy ? t("plugins:verifyingPublisher") : t("plugins:verifyPublisher")}
          </Button>
        </div>
      )}
      {canManage && error && (
        <div className="space-y-2" role="alert" data-testid="plugin-publisher-verification-error">
          <p className="text-sm text-destructive">{error}</p>
          <Button
            type="button"
            variant="ghost"
            className={controlSizingClassName(
              "compact",
              "cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11",
            )}
            disabled={busy || !plugin.installation_id}
            onClick={onVerify}
          >
            {t("plugins:retryVerification")}
          </Button>
        </div>
      )}
    </section>
  );
}
