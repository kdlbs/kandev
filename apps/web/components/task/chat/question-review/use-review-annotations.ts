import { useCallback, useRef, useState, type Dispatch, type SetStateAction } from "react";
import type { ClarificationAnswerAnnotation } from "@/lib/types/http";
import { MAX_REGIONS, type RegionRect } from "@/lib/question-review/regions";
import type { Size } from "@/lib/question-review/viewport";

export type ReviewAnnotations = Record<string, readonly ClarificationAnswerAnnotation[]>;
export type RegionSelection = { questionId: string; index: number } | null;
export type RegionCommenting = { questionId: string; index: number; isNew: boolean } | null;

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
  const [commenting, setCommentingState] = useState<RegionCommenting>(null);
  /** Phone regions sheet. */
  const [listOpen, setListOpen] = useState(false);
  // Read synchronously: the comment popover saves and then closes in one tick.
  const commentingRef = useRef<RegionCommenting>(null);
  const setCommenting = useCallback((next: RegionCommenting) => {
    commentingRef.current = next;
    setCommentingState(next);
  }, []);
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
      setCommenting({ questionId, index: current.length, isNew: true });
    },
    [annotations, setAnnotations, setCommenting],
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
      setAnnotations((all) => withoutRegion(all, questionId, index));
      setSelected(null);
      setCommenting(null);
    },
    [setAnnotations, setCommenting],
  );

  const editComment = useCallback(
    (questionId: string, index: number) => {
      setSelected({ questionId, index });
      setCommenting({ questionId, index, isNew: false });
    },
    [setCommenting],
  );

  const saveComment = useCallback(
    (comment: string) => {
      const target = commentingRef.current;
      if (!target) return;
      setCommenting(null);
      update(target.questionId, target.index, { comment });
    },
    [setCommenting, update],
  );

  /** Closes the comment popover. A new region that never got a comment is discarded. */
  const closeComment = useCallback(() => {
    const target = commentingRef.current;
    setCommenting(null);
    if (!target?.isNew) return;
    setAnnotations((all) => {
      const region = all[target.questionId]?.[target.index];
      return region && !region.comment.trim()
        ? withoutRegion(all, target.questionId, target.index)
        : all;
    });
    setSelected(null);
  }, [setAnnotations, setCommenting]);

  return {
    list,
    add,
    update,
    remove,
    selected,
    setSelected,
    annotating,
    setAnnotating,
    commenting,
    editComment,
    saveComment,
    closeComment,
    listOpen,
    setListOpen,
  };
}

export type ReviewAnnotationController = ReturnType<typeof useReviewAnnotations>;

function withoutRegion(all: ReviewAnnotations, questionId: string, index: number) {
  const next = (all[questionId] ?? []).filter((_, i) => i !== index);
  const rest = { ...all };
  if (next.length > 0) rest[questionId] = next;
  else delete rest[questionId];
  return rest;
}
