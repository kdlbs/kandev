import type { ClarificationAnswer, ClarificationRequestMetadata, Message } from "@/lib/types/http";

export type LateClarificationMessageLabels = {
  questionLabel: string;
  answerLabel: string;
  contextLabel: string;
};

export type LateClarificationSnapshot = {
  messages: readonly Message[];
  answers: readonly ClarificationAnswer[];
};

function metadataFor(message: Message): ClarificationRequestMetadata | undefined {
  return message.metadata as ClarificationRequestMetadata | undefined;
}

function sanitizeOrdinaryMessageText(value: string): string {
  return value.replace(/kandev-system/gi, "kandev system");
}

function sortQuestionMessages(messages: readonly Message[]): Message[] {
  return [...messages].sort((a, b) => {
    const ai = metadataFor(a)?.question_index ?? 0;
    const bi = metadataFor(b)?.question_index ?? 0;
    return ai - bi;
  });
}

function answerText(message: Message, answer: ClarificationAnswer | undefined): string {
  const question = metadataFor(message)?.question;
  if (!answer || !question) return "";
  const selectedLabels = (answer.selected_options ?? []).map((optionId) => {
    const option = question.options.find((candidate) => candidate.option_id === optionId);
    return option?.label ?? optionId;
  });
  if (answer.custom_text?.trim()) selectedLabels.push(answer.custom_text.trim());
  return selectedLabels.join(", ");
}

export function formatLateClarificationMessage(
  messages: readonly Message[],
  answers: readonly ClarificationAnswer[],
  labels: LateClarificationMessageLabels,
): string {
  const answersByQuestionId = new Map(answers.map((answer) => [answer.question_id, answer]));
  const ordered = sortQuestionMessages(messages);
  const firstContext = metadataFor(ordered[0])?.context?.trim();
  const sections: string[] = [];

  if (firstContext) {
    sections.push(`${labels.contextLabel}: ${sanitizeOrdinaryMessageText(firstContext)}`);
  }

  for (const [index, message] of ordered.entries()) {
    const metadata = metadataFor(message);
    const question = metadata?.question;
    if (!question) continue;
    const title = question.title?.trim();
    const prompt = sanitizeOrdinaryMessageText(question.prompt.trim());
    const answer = sanitizeOrdinaryMessageText(
      answerText(message, answersByQuestionId.get(metadata.question_id ?? question.id)),
    );
    const questionHeading = `${labels.questionLabel} ${index + 1}${title ? `: ${title}` : ""}`;
    sections.push([questionHeading, prompt, `${labels.answerLabel}: ${answer || ""}`].join("\n"));
  }

  return sections.join("\n\n");
}
