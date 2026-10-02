"use client";

import { useState } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";

export type ChangesPanelRefreshStatus = "loading" | "unavailable" | null;

export function ChangesPanelRefreshIndicator({
  status,
  ...props
}: {
  status: ChangesPanelRefreshStatus;
  hasPriorData: boolean;
  failedRepositories: string[];
}) {
  if (!status) return null;
  return <ChangesPanelRefreshStatus status={status} {...props} />;
}

function ChangesPanelRefreshStatus({
  status,
  hasPriorData,
  failedRepositories,
}: {
  status: Exclude<ChangesPanelRefreshStatus, null>;
  hasPriorData: boolean;
  failedRepositories: string[];
}) {
  const { t } = useTranslation();
  const [isFocused, setIsFocused] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const [isFocusDismissed, setIsFocusDismissed] = useState(false);

  const messages =
    status === "loading"
      ? [t("task:loadingChanges")]
      : [
          t("task:gitStatusAutomaticRetry"),
          ...(hasPriorData ? [t("task:gitStatusShowingLastAvailable")] : []),
          ...failedRepositories.map((repository) =>
            t("task:gitStatusRepositoryUnavailable", { repository }),
          ),
        ];
  const description = messages.join(" ");

  return (
    <Tooltip open={(isFocused && !isFocusDismissed) || isOpen} onOpenChange={setIsOpen}>
      <TooltipTrigger asChild>
        <span
          role="status"
          aria-live="polite"
          tabIndex={0}
          data-testid="changes-refresh-status"
          onFocus={() => {
            setIsFocused(true);
            setIsFocusDismissed(false);
          }}
          onBlur={() => {
            setIsFocused(false);
            setIsFocusDismissed(false);
          }}
          className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
        >
          <span className="sr-only">{description}</span>
          {status === "loading" ? (
            <Spinner aria-hidden="true" role="presentation" className="size-3.5" />
          ) : (
            <IconAlertTriangle
              aria-hidden="true"
              className="size-3.5 text-amber-500"
              focusable="false"
            />
          )}
        </span>
      </TooltipTrigger>
      <TooltipContent
        onEscapeKeyDown={() => {
          setIsFocusDismissed(true);
          setIsOpen(false);
        }}
      >
        <div className="space-y-1">
          {messages.map((message, index) => (
            <p key={`${index}-${message}`}>{message}</p>
          ))}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}
