import type {
  ClarificationAnswer,
  ClarificationAnswerAnnotation,
  ClarificationAnswerAttachment,
} from "@/lib/types/http";
import { fitAnnotationToPixelLimit } from "@/lib/question-review/regions";

/**
 * Applies the current ready files and drawn regions to an answer. Regions always
 * come from the current draft, so a region deleted after a failed attempt is not
 * resent; empty lists are omitted. Regions on images over the server's size
 * limit are reported in a scaled pixel space.
 */
export function withAnswerExtras(
  answer: ClarificationAnswer,
  files: readonly ClarificationAnswerAttachment[],
  regions: readonly ClarificationAnswerAnnotation[] | undefined,
): ClarificationAnswer {
  const hasRegions = Boolean(regions && regions.length > 0);
  if (files.length === 0 && !hasRegions && answer.annotations === undefined) return answer;
  const { annotations: _previous, ...base } = answer;
  const next: ClarificationAnswer = { ...base };
  if (files.length > 0) next.attachments = [...files];
  if (hasRegions) next.annotations = regions!.map(fitAnnotationToPixelLimit);
  return next;
}
