"use client";

import { IconAlertCircle, IconPinFilled } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { IssueTaskIcon } from "@/components/github/issue-task-icon";
import { RegisteredChangeRequestTaskIcon } from "@/components/integrations/registered-change-request-task-icon";
import { TaskAutopilotIcon } from "@/components/task/task-autopilot-icon";
import { TaskContributionIcons } from "./task-contribution-icons";

export function TaskItemLeadingBadges({
  autopilot,
  isPinned,
  taskId,
  prInfo,
  showChangeRequestStatus = true,
  issueInfo,
  agentErrorMessage,
}: {
  autopilot?: boolean;
  isPinned?: boolean;
  taskId?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
  showChangeRequestStatus?: boolean;
  issueInfo?: { url: string; number: number };
  agentErrorMessage?: string | null;
}) {
  return (
    <>
      {autopilot && <TaskAutopilotIcon />}
      {isPinned && (
        <IconPinFilled
          data-testid="task-pinned-icon"
          className="h-3 w-3 shrink-0 text-muted-foreground/60"
        />
      )}
      {showChangeRequestStatus && <TaskContributionIcons taskId={taskId} prInfo={prInfo} />}
      {showChangeRequestStatus && taskId ? (
        <RegisteredChangeRequestTaskIcon taskId={taskId} />
      ) : null}
      {issueInfo && <IssueTaskIcon issueInfo={issueInfo} />}
      {agentErrorMessage && <TaskAgentErrorIcon message={agentErrorMessage} />}
    </>
  );
}

function TaskAgentErrorIcon({ message }: { message: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          data-testid="task-agent-error-icon"
          className="inline-flex shrink-0 cursor-help text-destructive"
          aria-label={t("task:taskHasAnAgentError")}
        >
          <IconAlertCircle className="h-3.5 w-3.5" aria-hidden="true" />
        </span>
      </TooltipTrigger>
      <TooltipContent side="right" className="max-w-[320px] whitespace-pre-wrap break-words">
        {message}
      </TooltipContent>
    </Tooltip>
  );
}
