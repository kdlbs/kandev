import type { ClarificationQuestion, QuestionImage } from "@/lib/types/http";

/** One image on the review stage, in segment order. */
export type ReviewImage = {
  id: string;
  alt: string;
  choiceLabel?: string;
  ref: QuestionImage;
};

/** One question in the review view. */
export type ReviewSegment = {
  questionId: string;
  index: number;
  label: string;
  images: ReviewImage[];
};

const MAX_SEGMENT_IMAGES = 4;
const MAX_LABEL_LENGTH = 48;

function isImage(value: unknown): value is QuestionImage {
  if (!value || typeof value !== "object") return false;
  const image = value as Partial<QuestionImage>;
  return typeof image.id === "string" && typeof image.alt === "string";
}

function validImages(images: unknown): QuestionImage[] {
  return Array.isArray(images) ? images.filter(isImage) : [];
}

/** Question images first, then each choice's images in option order. */
export function buildSegmentImages(question: ClarificationQuestion): ReviewImage[] {
  const result: ReviewImage[] = [];
  const seen = new Set<string>();
  const add = (image: QuestionImage, choiceLabel?: string) => {
    if (seen.has(image.id) || result.length >= MAX_SEGMENT_IMAGES) return;
    seen.add(image.id);
    result.push(
      choiceLabel === undefined
        ? { id: image.id, alt: image.alt, ref: image }
        : { id: image.id, alt: image.alt, choiceLabel, ref: image },
    );
  };
  validImages(question.images).forEach((image) => add(image));
  for (const option of Array.isArray(question.options) ? question.options : []) {
    validImages(option?.images).forEach((image) => add(image, option.label));
  }
  return result;
}

function segmentLabel(question: ClarificationQuestion): string {
  const source = (question.title || question.prompt || "").replace(/\s+/g, " ").trim();
  if (source.length <= MAX_LABEL_LENGTH) return source;
  return `${source.slice(0, MAX_LABEL_LENGTH - 1).trimEnd()}…`;
}

export function buildReviewSegments(
  questions: readonly { questionId: string; question: ClarificationQuestion }[],
): ReviewSegment[] {
  return questions.map(({ questionId, question }, index) => ({
    questionId,
    index,
    label: segmentLabel(question),
    images: buildSegmentImages(question),
  }));
}

export function hasReviewImages(questions: readonly ClarificationQuestion[]): boolean {
  return questions.some((question) => buildSegmentImages(question).length > 0);
}

/** One-based stage position of an image, or 0 when the image is not in the segment. */
export function segmentImageIndex(images: readonly ReviewImage[], imageId: string): number {
  return images.findIndex((image) => image.id === imageId) + 1;
}
