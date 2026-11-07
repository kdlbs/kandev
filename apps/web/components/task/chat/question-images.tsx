"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { QuestionImage } from "@/lib/types/http";
import {
  useQuestionImageOwner,
  useQuestionImages,
} from "@/hooks/domains/session/use-question-images";
import { QuestionImageViewer } from "./question-image-viewer";

type Props = {
  pendingId: string;
  questionId: string;
  sessionId: string;
  images?: QuestionImage[];
  historical?: boolean;
  enabled?: boolean;
  onInspectionChange?: (open: boolean) => void;
};

export function imageErrorKey(code: string) {
  switch (code) {
    case "not_found":
      return "task:questionImageNotFound";
    case "unsupported":
      return "task:questionImageUnsupported";
    case "invalid_image":
      return "task:questionImageInvalid";
    case "too_large":
      return "task:questionImageTooLarge";
    default:
      return "task:questionImageUnavailable";
  }
}

export function QuestionImages(props: Props) {
  if (props.images === undefined || (Array.isArray(props.images) && props.images.length === 0))
    return null;
  return <QuestionImagesContent {...props} />;
}

function QuestionImagesContent(props: Props) {
  const { t } = useTranslation();
  const { owner, authenticated } = useQuestionImageOwner(props.sessionId);
  const [expanded, setExpanded] = useState(false);
  const images = Array.isArray(props.images)
    ? props.images
        .filter((image) => image && typeof image.id === "string" && typeof image.alt === "string")
        .slice(0, 4)
    : [];
  const active =
    props.enabled !== false &&
    (!props.historical || expanded) &&
    authenticated &&
    Boolean(props.sessionId);
  const { states, retry } = useQuestionImages(
    props.pendingId,
    props.questionId,
    images,
    active,
    owner,
  );
  if (props.images === undefined || (Array.isArray(props.images) && props.images.length === 0))
    return null;
  if (images.length === 0)
    return (
      <p className="mb-3 text-xs text-muted-foreground">{t("task:questionImageUnavailable")}</p>
    );
  return (
    <div className="mb-3 space-y-3" data-testid="question-images">
      {props.historical && (
        <Button
          type="button"
          variant="outline"
          className="h-7 max-md:h-12 [@media(pointer:coarse)]:h-12"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          {t(expanded ? "task:questionImageHide" : "task:questionImageShow")}
        </Button>
      )}
      {active && (
        <div className="flex flex-wrap items-start gap-3" data-testid="question-images-row">
          {images.map((image) => {
            const state = states[image.id];
            return (
              <figure key={image.id} className="flex max-w-full min-w-0 flex-col gap-1">
                {/* The caption takes the thumbnail's width instead of widening the row. */}
                <figcaption className="w-0 min-w-full break-words text-xs text-muted-foreground">
                  {image.alt}
                </figcaption>
                {state?.kind === "loaded" ? (
                  <QuestionImageViewer
                    key={state.url}
                    src={state.url}
                    alt={image.alt}
                    onInspectionChange={props.onInspectionChange}
                  />
                ) : (
                  <div className="flex min-h-24 w-40 max-w-full flex-col items-start justify-center gap-2 rounded-md bg-muted/40 p-3">
                    <p role="status" className="text-xs text-muted-foreground">
                      {t(
                        state?.kind === "error"
                          ? imageErrorKey(state.code)
                          : "task:questionImageLoading",
                      )}
                    </p>
                    {state?.kind === "error" && (
                      <Button
                        type="button"
                        variant="outline"
                        className="h-7 max-md:h-12 [@media(pointer:coarse)]:h-12"
                        onClick={retry}
                      >
                        {t("task:questionImageRetry")}
                      </Button>
                    )}
                  </div>
                )}
              </figure>
            );
          })}
        </div>
      )}
    </div>
  );
}
