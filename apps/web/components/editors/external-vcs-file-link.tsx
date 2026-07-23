"use client";

import { IconBrandGithub, IconBrandGitlab } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import { useExternalVcsFileLink } from "@/hooks/domains/workspace/use-external-vcs-file-link";
import type { UseExternalVcsFileLinkInput } from "@/hooks/domains/workspace/use-external-vcs-file-link";
import {
  useSessionGitStatus,
  useSessionGitStatusByRepo,
} from "@/hooks/domains/session/use-session-git-status";
import type { ExternalVcsProvider } from "@/lib/utils/external-vcs-file-url";

export type ExternalVcsFileLinkProps = UseExternalVcsFileLinkInput & {
  size?: "xs" | "sm" | "touch";
};

const providerNames: Record<ExternalVcsProvider, string> = {
  github: "GitHub",
  gitlab: "GitLab",
  azure_devops: "Azure DevOps",
};

const sizeClasses = {
  xs: "size-5",
  sm: "size-6",
  touch: "size-11",
} as const;

function ProviderIcon({ provider }: { provider: ExternalVcsProvider }) {
  if (provider === "github") {
    return <IconBrandGithub data-testid="github-provider-icon" aria-hidden="true" />;
  }
  if (provider === "gitlab") {
    return <IconBrandGitlab data-testid="gitlab-provider-icon" aria-hidden="true" />;
  }
  return <AzureDevOpsIcon />;
}

function buttonSize(size: NonNullable<ExternalVcsFileLinkProps["size"]>) {
  if (size === "touch") return "icon" as const;
  if (size === "xs") return "icon-xs" as const;
  return "icon-sm" as const;
}

export function useExternalVcsFileStatus(
  filePath: string,
  sessionId?: string | null,
  repositoryName?: string,
) {
  const gitStatus = useSessionGitStatus(sessionId ?? null);
  const statusesByRepo = useSessionGitStatusByRepo(sessionId ?? null);
  const status = repositoryName
    ? statusesByRepo.find((entry) => entry.repository_name === repositoryName)?.status
    : gitStatus;
  return status?.files?.[filePath];
}

export function ExternalVcsFileLink({ size = "xs", ...input }: ExternalVcsFileLinkProps) {
  const link = useExternalVcsFileLink(input);
  if (!link) return null;

  const label = `Open file in ${providerNames[link.provider]}`;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          asChild
          variant="ghost"
          size={buttonSize(size)}
          className={`${sizeClasses[size]} cursor-pointer text-muted-foreground hover:text-foreground`}
        >
          <a
            href={link.url}
            target="_blank"
            rel="noopener noreferrer"
            aria-label={label}
            title={label}
          >
            <ProviderIcon provider={link.provider} />
          </a>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
