import { useCallback, useState, type Dispatch, type SetStateAction } from "react";
import type { ClarificationAnswerAnnotation } from "@/lib/types/http";
import { MAX_REGIONS, type RegionRect } from "@/lib/question-review/regions";
import type { Size } from "@/lib/question-review/viewport";

export type ReviewAnnotations = Record<string, readonly ClarificationAnswerAnnotation[]>;
export type RegionSelection = { questionId: string; index: number } | null;

/**
 * Edits the regions held in the answer draft. Selection and annotate mode are
 * view state: they live with the open review view, not in the draft.
 */
export function useReviewAnnotations(
  annotations: ReviewAnnotations,
  setAnnotations: Dispatch<SetStateAction<ReviewAnnotations>>,
) {
  const [selected, setSelected] = useState<RegionSelection>(null);
  const [annotating, setAnnotating] = useState(false);
  const list = useCallback((questionId: string) => annotations[questionId] ?? [], [annotations]);

  const add = useCallback(
    (questionId: string, imageId: string, rect: RegionRect, image: Size) => {
      const current = annotations[questionId] ?? [];
      if (current.length >= MAX_REGIONS) return;
      const region: ClarificationAnswerAnnotation = {
        image_id: imageId,
        ...rect,
        image_width: image.width,
        image_height: image.height,
        comment: "",
      };
      setAnnotations((all) => ({ ...all, [questionId]: [...(all[questionId] ?? []), region] }));
      setSelected({ questionId, index: current.length });
    },
    [annotations, setAnnotations],
  );

  const update = useCallback(
    (questionId: string, index: number, patch: Partial<ClarificationAnswerAnnotation>) => {
      setAnnotations((all) => ({
        ...all,
        [questionId]: (all[questionId] ?? []).map((region, i) =>
          i === index ? { ...region, ...patch } : region,
        ),
      }));
    },
    [setAnnotations],
  );

  const remove = useCallback(
    (questionId: string, index: number) => {
      setAnnotations((all) => {
        const next = (all[questionId] ?? []).filter((_, i) => i !== index);
        const rest = { ...all };
        if (next.length > 0) rest[questionId] = next;
        else delete rest[questionId];
        return rest;
      });
      setSelected(null);
    },
    [setAnnotations],
  );

  return { list, add, update, remove, selected, setSelected, annotating, setAnnotating };
}

export type ReviewAnnotationController = ReturnType<typeof useReviewAnnotations>;
