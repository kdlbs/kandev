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
  if (question?.title?.trim()) return question.title;
  if (question?.prompt) return question.prompt;
  if (bundle.context) return bundle.context;
  return fallback;
}

// Secondary text: task title then task identifier (design-01#Data-and-contracts).
export function rowSecondaryText(bundle: ClarificationInboxBundle): string {
  return bundle.task_title || bundle.task_id;
}

// How many questions the bundle carries, counted by the presence of a
// clarification question rather than by message count: a bundle's message list
// can hold context messages that are not themselves questions (design-03#D3).
export function rowQuestionCount(bundle: ClarificationInboxBundle): number {
  return bundle.messages.filter((message) => {
    const metadata = message.metadata as ClarificationRequestMetadata | undefined;
    return Boolean(metadata?.question);
  }).length;
}
