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

function answerText(
  message: Message,
  answer: ClarificationAnswer | undefined,
  fileLabels: string[],
): string {
  const question = metadataFor(message)?.question;
  if (!answer || !question) return "";
  const selectedLabels = (answer.selected_options ?? []).map((optionId) => {
    const option = question.options.find((candidate) => candidate.option_id === optionId);
    return option?.label ?? optionId;
  });
  if (answer.custom_text?.trim()) selectedLabels.push(answer.custom_text.trim());
  if (fileLabels.length > 0) selectedLabels.push(fileLabels.join(", "));
  return selectedLabels.join(", ");
}

function annotationEvidenceLines(answer: ClarificationAnswer | undefined): string[] {
  if (!answer?.annotations?.length) return [];
  const evidence = JSON.stringify({
    question_id: answer.question_id,
    annotations: answer.annotations,
  });
  return [`Answer annotations (JSON): ${sanitizeOrdinaryMessageText(evidence)}`];
}

// orderedLateClarificationAnswers returns answers in question order. The flat
// attachment list sent with a late answer must use this same order so the
// numbered file labels in the fallback text match the delivered attachments.
export function orderedLateClarificationAnswers(
  messages: readonly Message[],
  answers: readonly ClarificationAnswer[],
): ClarificationAnswer[] {
  const answersByQuestionId = new Map(answers.map((answer) => [answer.question_id, answer]));
  return sortQuestionMessages(messages).flatMap((message) => {
    const metadata = metadataFor(message);
    const answer = answersByQuestionId.get(metadata?.question_id ?? metadata?.question?.id ?? "");
    return answer ? [answer] : [];
  });
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

  let attachmentIndex = 0;
  for (const [index, message] of ordered.entries()) {
    const metadata = metadataFor(message);
    const question = metadata?.question;
    if (!question) continue;
    const title = question.title?.trim();
    const prompt = sanitizeOrdinaryMessageText(question.prompt.trim());
    const storedAnswer = answersByQuestionId.get(metadata.question_id ?? question.id);
    const fileLabels = (storedAnswer?.attachments ?? []).map((attachment) => {
      attachmentIndex += 1;
      return `${attachment.name} (#${attachmentIndex})`;
    });
    const answer = sanitizeOrdinaryMessageText(answerText(message, storedAnswer, fileLabels));
    const questionHeading = `${labels.questionLabel} ${index + 1}${title ? `: ${title}` : ""}`;
    const section = [questionHeading, prompt, `${labels.answerLabel}: ${answer || ""}`];
    section.push(...annotationEvidenceLines(storedAnswer));
    sections.push(section.join("\n"));
  }

  return sections.join("\n\n");
}
