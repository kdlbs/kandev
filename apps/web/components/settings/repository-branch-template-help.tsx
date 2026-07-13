"use client";

import { IconInfoCircle } from "@tabler/icons-react";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";

const branchTemplatePlaceholders = [
  [
    "{title}",
    "Task title sanitized to lowercase ASCII, hyphen-separated, max 20 chars. Example: fix-login-flow.",
  ],
  [
    "{title_full}",
    "Same sanitizing as title, but max 80 chars. Example: fix-login-flow-after-session-timeout.",
  ],
  [
    "{ticket}",
    "Task identifier first; otherwise Jira, Linear, GitHub issue, or GitHub PR metadata. Examples: KAN-123, #42.",
  ],
  ["{issue_key}", "Alias for ticket. Use whichever name reads better in your template."],
  [
    "{task_id}",
    "Kandev task UUID, sanitized for branch names. Example: 1f1cf094-db3c-4f42-b425-2cc14a2f7c74.",
  ],
  [
    "{suffix}",
    "Short random suffix. Optional, but recommended to avoid branch name clashes. Example: x7p9.",
  ],
] as const;

export function RepositoryBranchTemplateHelp() {
  return (
    <HoverCard openDelay={150} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          aria-label="Branch template placeholders"
          className="cursor-help text-muted-foreground hover:text-foreground"
        >
          <IconInfoCircle className="h-3.5 w-3.5" />
        </button>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-96 text-xs">
        <div className="space-y-2">
          <p className="text-muted-foreground">
            Write literal prefixes directly, for example{" "}
            <code className="rounded bg-muted px-1 py-0.5">
              feature/{"{ticket}"}-{"{title}"}
            </code>
            .
          </p>
          <dl className="space-y-1.5">
            {branchTemplatePlaceholders.map(([name, description]) => (
              <div key={name} className="grid grid-cols-[5.5rem_1fr] gap-2">
                <dt className="font-mono text-foreground">{name}</dt>
                <dd className="text-muted-foreground">{description}</dd>
              </div>
            ))}
          </dl>
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}
