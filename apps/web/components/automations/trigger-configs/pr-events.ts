// `value` is the GitHub PR event name persisted in the trigger config and
// matched by the backend — protocol. Label and tooltip are copy.
export const PR_EVENTS = [
  {
    value: "opened",
    labelKey: "automations:prEventOpened",
    tooltipKey: "automations:prEventOpenedTooltip",
  },
  {
    value: "new_commits",
    labelKey: "automations:prEventNewCommits",
    tooltipKey: "automations:prEventNewCommitsTooltip",
  },
  {
    value: "review_requested",
    labelKey: "automations:prEventReviewRequested",
    tooltipKey: "automations:prEventReviewRequestedTooltip",
  },
] as const;
