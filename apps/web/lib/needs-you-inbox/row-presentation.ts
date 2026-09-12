import type { ClarificationRequestMetadata } from "@/lib/types/http-agents";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";

function firstQuestion(bundle: ClarificationInboxBundle) {
  const metadata = bundle.messages[0]?.metadata as ClarificationRequestMetadata | undefined;
  return metadata?.question;
}

// design-01#Data-and-contracts: title, else prompt, else the bundle's shared
// context, else the caller-supplied localized fallback. Never blank.
export function rowPrimaryText(bundle: ClarificationInboxBundle, fallback: string): string {
  const question = firstQuestion(bundle);
  if (question?.title) return question.title;
  if (question?.prompt) return question.prompt;
  if (bundle.context) return bundle.context;
  return fallback;
}

// Secondary text: task title then task identifier (design-01#Data-and-contracts).
export function rowSecondaryText(bundle: ClarificationInboxBundle): string {
  return bundle.task_title || bundle.task_id;
}
