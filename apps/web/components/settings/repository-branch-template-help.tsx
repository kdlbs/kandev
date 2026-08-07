"use client";

import { Trans, useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";

/**
 * The placeholder tokens are PROTOCOL, not copy: `RenderTaskBranchName` in
 * `apps/backend/internal/worktree/config.go` substitutes each one by exact
 * string match, so a translated `{title}` would be emitted into the branch name
 * verbatim. Only the description beside each token is copy, and it lives in the
 * catalog under `descriptionKey` so it resolves at render rather than at import.
 *
 * The example values are branch-name output — sanitized lowercase ASCII, a
 * ticket key, a UUID — so they travel as interpolated VALUES rather than sitting
 * inside the message a translator edits.
 */
const branchTemplatePlaceholders = [
  {
    token: "{title}",
    descriptionKey: "workspaces:branchTemplateTitle",
    example: "fix-login-flow",
  },
  {
    token: "{title_full}",
    descriptionKey: "workspaces:branchTemplateTitleFull",
    example: "fix-login-flow-after-session-timeout",
  },
  {
    token: "{ticket}",
    descriptionKey: "workspaces:branchTemplateTicket",
    example: "KAN-123, #42",
  },
  { token: "{issue_key}", descriptionKey: "workspaces:branchTemplateIssueKey", example: "" },
  {
    token: "{task_id}",
    descriptionKey: "workspaces:branchTemplateTaskId",
    example: "1f1cf094-db3c-4f42-b425-2cc14a2f7c74",
  },
  { token: "{suffix}", descriptionKey: "workspaces:branchTemplateSuffix", example: "x7p9" },
] as const;

const LITERAL_PREFIX_EXAMPLE = "feature/{ticket}-{title}";

export function RepositoryBranchTemplateHelp() {
  const { t } = useTranslation();
  return (
    <HoverCard openDelay={150} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          aria-label={t("workspaces:branchTemplatePlaceholders")}
          className="cursor-help text-muted-foreground hover:text-foreground"
        >
          <IconInfoCircle className="h-3.5 w-3.5" />
        </button>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-96 text-xs">
        <div className="space-y-2">
          <p className="text-muted-foreground">
            <Trans
              i18nKey="workspaces:branchTemplateLiteralPrefix"
              values={{ example: LITERAL_PREFIX_EXAMPLE }}
            >
              <code className="rounded bg-muted px-1 py-0.5" />
            </Trans>
          </p>
          <dl className="space-y-1.5">
            {branchTemplatePlaceholders.map(({ token, descriptionKey, example }) => (
              <div key={token} className="grid grid-cols-[5.5rem_1fr] gap-2">
                <dt className="font-mono text-foreground">{token}</dt>
                <dd className="text-muted-foreground">{t(descriptionKey, { example })}</dd>
              </div>
            ))}
          </dl>
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}
