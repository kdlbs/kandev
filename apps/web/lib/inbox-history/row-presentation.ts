import type { ClarificationOption, ClarificationRequestMetadata } from "@/lib/types/http-agents";
import type {
  InboxHistoryBundle,
  InboxHistoryPermissionMetadata,
  InboxHistoryReason,
} from "@/lib/types/inbox-history";

export type InboxHistoryQuestionView = {
  id: string;
  title: string;
  prompt: string;
  optionLabels: string[];
};

// AC .30: a clarification bundle renders each question's title/prompt and
// option labels, in the order the API already returned them (AC .19) --
// including a question whose own status is already terminal.
export function inboxHistoryClarificationQuestions(
  bundle: InboxHistoryBundle,
): InboxHistoryQuestionView[] {
  return bundle.messages.map((message, index) => {
    const metadata = message.metadata as ClarificationRequestMetadata | undefined;
    const question = metadata?.question;
    return {
      id: question?.id || `${bundle.pending_id}-${index}`,
      title: question?.title ?? "",
      prompt: question?.prompt ?? "",
      optionLabels: (question?.options ?? []).map((option: ClarificationOption) => option.label),
    };
  });
}

// AC .30: a permission bundle carries no `question` object -- its text is the
// message's own `content` and its choices are `metadata.options[].name`, not
// `.label`. A permission bundle is always a bundle of one (system design
// "Record shape").
export function inboxHistoryPermissionContent(bundle: InboxHistoryBundle): {
  content: string;
  optionNames: string[];
} {
  const message = bundle.messages[0];
  const metadata = message?.metadata as InboxHistoryPermissionMetadata | undefined;
  return {
    content: message?.content ?? "",
    optionNames: (metadata?.options ?? []).map((option) => option.name),
  };
}

const REASON_LABEL_KEYS: Record<InboxHistoryReason, string> = {
  superseded: "inboxHistory:reasonSuperseded",
  session_ended: "inboxHistory:reasonSessionEnded",
  unreadable: "inboxHistory:reasonUnreadable",
};

// AC .9: the three reasons must read as distinguishable statements, never the
// raw `pending` status.
export function inboxHistoryReasonLabelKey(reason: InboxHistoryReason): string {
  return REASON_LABEL_KEYS[reason];
}

// Secondary text: task title then task identifier, matching the Needs-you
// sibling's row-presentation convention.
export function inboxHistorySecondaryText(bundle: InboxHistoryBundle): string {
  return bundle.task_title || bundle.task_id;
}
