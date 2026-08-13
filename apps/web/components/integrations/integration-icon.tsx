"use client";

import {
  IconFilter,
  IconGitMerge,
  IconGitPullRequest,
  IconGitPullRequestClosed,
} from "@tabler/icons-react";

export type IntegrationIconName = "filter" | "merged" | "pull-request" | "pull-request-closed";

const ICONS = {
  filter: IconFilter,
  merged: IconGitMerge,
  "pull-request": IconGitPullRequest,
  "pull-request-closed": IconGitPullRequestClosed,
} satisfies Record<IntegrationIconName, typeof IconFilter>;

export function IntegrationIcon({
  name,
  className,
}: {
  name: IntegrationIconName;
  className?: string;
}) {
  const Icon = ICONS[name];
  return <Icon aria-hidden="true" className={className} data-integration-icon={name} />;
}
