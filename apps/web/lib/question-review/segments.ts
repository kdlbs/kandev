import type { ClarificationQuestion, QuestionImage, QuestionImageGroup } from "@/lib/types/http";

/** One image on the review stage, in segment order. */
export type ReviewImage = {
  id: string;
  alt: string;
  /** The agent's one-word label, when it is valid. */
  label?: string;
  choiceLabel?: string;
  ref: QuestionImage;
};

/** One screen of a segment's stage: a compared pair, a lone single, or one gallery image. */
export type ReviewItem =
  | { kind: "pair"; images: [ReviewImage, ReviewImage] }
  | { kind: "single"; image: ReviewImage }
  | { kind: "gallery"; image: ReviewImage };

/** One question in the review view. */
export type ReviewSegment = {
  questionId: string;
  index: number;
  label: string;
  images: ReviewImage[];
  /** True when the question declares valid image groups. */
  grouped: boolean;
  items: ReviewItem[];
};

const MAX_SEGMENT_IMAGES = 4;
const MAX_LABEL_LENGTH = 48;
const IMAGE_LABEL_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_-]{0,15}$/;

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
    const entry: ReviewImage = { id: image.id, alt: image.alt, ref: image };
    if (typeof image.label === "string" && IMAGE_LABEL_PATTERN.test(image.label)) {
      entry.label = image.label;
    }
    if (choiceLabel !== undefined) entry.choiceLabel = choiceLabel;
    result.push(entry);
  };
  validImages(question.images).forEach((image) => add(image));
  for (const option of Array.isArray(question.options) ? question.options : []) {
    validImages(option?.images).forEach((image) => add(image, option.label));
  }
  return result;
}

function galleryItems(images: readonly ReviewImage[]): ReviewItem[] {
  return images.map((image) => ({ kind: "gallery", image }));
}

function groupItem(group: unknown, byId: Map<string, ReviewImage>): ReviewItem | null {
  if (!group || typeof group !== "object") return null;
  const { kind, image_ids: ids } = group as Partial<QuestionImageGroup>;
  if (!Array.isArray(ids) || !ids.every((id) => typeof id === "string" && byId.has(id))) {
    return null;
  }
  const found = ids.map((id) => byId.get(id)!);
  if (kind === "pair" && found.length === 2 && ids[0] !== ids[1]) {
    return { kind: "pair", images: [found[0], found[1]] };
  }
  if (kind === "single" && found.length === 1) return { kind: "single", image: found[0] };
  return null;
}

function itemImageIds(item: ReviewItem): string[] {
  return item.kind === "pair" ? item.images.map((image) => image.id) : [item.image.id];
}

/**
 * Declared groups in order, then every image no group names as a single.
 * Without groups, or when any group is malformed, the images form a gallery.
 */
export function buildSegmentItems(
  images: readonly ReviewImage[],
  groups: readonly QuestionImageGroup[] | undefined,
): ReviewItem[] {
  if (!Array.isArray(groups) || groups.length === 0) return galleryItems(images);
  const byId = new Map(images.map((image) => [image.id, image]));
  const items: ReviewItem[] = [];
  const used = new Set<string>();
  for (const group of groups) {
    const item = groupItem(group, byId);
    const ids = item ? itemImageIds(item) : [];
    if (!item || ids.some((id) => used.has(id))) return galleryItems(images);
    ids.forEach((id) => used.add(id));
    items.push(item);
  }
  for (const image of images) {
    if (!used.has(image.id)) items.push({ kind: "single", image });
  }
  return items;
}

/** The item that shows an image, or -1. */
export function itemIndexForImage(items: readonly ReviewItem[], imageId: string): number {
  return items.findIndex((item) => itemImageIds(item).includes(imageId));
}

function segmentLabel(question: ClarificationQuestion): string {
  const source = (question.title || question.prompt || "").replace(/\s+/g, " ").trim();
  if (source.length <= MAX_LABEL_LENGTH) return source;
  return `${source.slice(0, MAX_LABEL_LENGTH - 1).trimEnd()}…`;
}

export function buildReviewSegments(
  questions: readonly { questionId: string; question: ClarificationQuestion }[],
): ReviewSegment[] {
  return questions.map(({ questionId, question }, index) => {
    const images = buildSegmentImages(question);
    const items = buildSegmentItems(images, question.image_groups);
    return {
      questionId,
      index,
      label: segmentLabel(question),
      images,
      grouped: items.some((item) => item.kind !== "gallery"),
      items,
    };
  });
}

export function hasReviewImages(questions: readonly ClarificationQuestion[]): boolean {
  return questions.some((question) => buildSegmentImages(question).length > 0);
}

/** One-based stage position of an image, or 0 when the image is not in the segment. */
export function segmentImageIndex(images: readonly ReviewImage[], imageId: string): number {
  return images.findIndex((image) => image.id === imageId) + 1;
}

/** Short picker name: the agent's label, or the one-based stage number. */
export function imageShortName(image: ReviewImage, index: number): string {
  return image.label ?? String(index + 1);
}
