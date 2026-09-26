"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import type { PublisherIdentity, PublisherProvenance } from "@/lib/types/plugins";

export type PluginPublisherIdentityProps = {
  identity?: PublisherIdentity;
  author?: string;
  sourceName?: string;
  sourceOrigin?: string;
  provenance?: PublisherProvenance;
  matchedSourceName?: string;
  compact?: boolean;
  className?: string;
};

/**
 * Renders host-owned publisher evidence beside the package's declared author.
 * The two values deliberately use separate rows so package metadata cannot
 * look like an identity assertion.
 */
export function PluginPublisherIdentity({
  identity,
  author,
  sourceName,
  sourceOrigin,
  provenance,
  matchedSourceName,
  compact = false,
  className,
}: PluginPublisherIdentityProps) {
  const { t } = useTranslation();
  const verified = identity?.status === "verified" && Boolean(identity.login);
  const source = sourceName || sourceLabel(sourceOrigin || provenance?.origin, t);
  const matchedSource =
    matchedSourceName ||
    (identity?.matched_source === "official"
      ? t("plugins:officialSourceName")
      : identity?.matched_source);

  return (
    <div
      className={`${compact ? "flex flex-wrap items-center gap-x-3 gap-y-1" : "space-y-1"} text-xs text-muted-foreground ${className ?? ""}`}
      data-testid="plugin-publisher-identity"
    >
      <PublisherStatus identity={identity} verified={verified} />
      <PublisherField label={t("plugins:publisherSourceLabel")} value={source} />
      <PublisherField label={t("plugins:declaredAuthorLabel")} value={author} />
      {verified && (
        <PublisherField label={t("plugins:matchedPublisherSourceLabel")} value={matchedSource} />
      )}
    </div>
  );
}

function PublisherStatus({
  identity,
  verified,
}: {
  identity?: PublisherIdentity;
  verified: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
      {verified ? (
        <>
          <span>
            {t("plugins:publisherLabel")}: {identity?.login}
          </span>
          <PublisherBadge identity={identity} />
        </>
      ) : (
        <span className="font-medium text-foreground">{t("plugins:unverifiedPublisher")}</span>
      )}
    </div>
  );
}

function PublisherField({ label, value }: { label: string; value?: string }) {
  if (!value) return null;
  return (
    <div className="flex flex-wrap gap-x-2 gap-y-0.5">
      <span>{label}:</span>
      <span className="break-all text-foreground/80">{value}</span>
    </div>
  );
}

/** Returns whether two host projections describe the same known publisher. */
export function publisherIdentitiesMatch(
  first?: PublisherIdentity,
  second?: PublisherIdentity,
): boolean {
  if (!first || !second || first.status !== second.status) return false;
  if (first.status === "unverified") return true;
  return (
    first.repository_id === second.repository_id &&
    first.owner_id === second.owner_id &&
    first.login === second.login &&
    first.repository === second.repository &&
    first.official === second.official
  );
}

export function PublisherBadge({ identity }: { identity?: PublisherIdentity }) {
  const { t } = useTranslation();
  if (identity?.status !== "verified" || !identity.login) return null;
  return (
    <Badge
      variant="outline"
      className={
        identity.official
          ? "border-primary/40 bg-primary/10 text-primary"
          : "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400"
      }
    >
      {identity.official ? t("plugins:officialPublisher") : t("plugins:verifiedPublisher")}
    </Badge>
  );
}

function sourceLabel(
  origin: PublisherProvenance["origin"] | undefined,
  t: (key: string) => string,
): string {
  switch (origin) {
    case "catalog":
      return t("plugins:sourceCatalog");
    case "url":
      return t("plugins:sourceDirectUrl");
    case "upload":
      return t("plugins:sourceUploadedFile");
    case "sideload":
      return t("plugins:sourceSideload");
    default:
      return t("plugins:sourceUnknown");
  }
}
