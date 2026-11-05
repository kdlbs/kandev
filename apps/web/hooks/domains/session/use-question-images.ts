import { useCallback, useEffect, useState } from "react";
import type { QuestionImage } from "@/lib/types/http";
import { fetchQuestionImage, questionImageErrorCode } from "@/lib/api/domains/question-image-api";

import { scheduleQuestionImage } from "./question-image-scheduler";

export type QuestionImageState =
  | { kind: "loading" }
  | { kind: "loaded"; url: string }
  | { kind: "error"; code: string };

type Snapshot = { key: string; states: Record<string, QuestionImageState> };

export function useQuestionImages(
  pendingId: string,
  questionId: string,
  images: readonly QuestionImage[],
  active: boolean,
  owner: string,
) {
  const [attempt, setAttempt] = useState(0);
  const [snapshot, setSnapshot] = useState<Snapshot>({ key: "", states: {} });
  const imageKey = JSON.stringify(images);
  const key = JSON.stringify([pendingId, questionId, imageKey, active, owner, attempt]);
  useEffect(() => {
    if (!active || !pendingId || !questionId) return;
    const refs = JSON.parse(imageKey) as QuestionImage[];
    const controller = new AbortController();
    const urls: string[] = [];
    let cursor = 0;
    const update = (id: string, state: QuestionImageState) => {
      if (controller.signal.aborted) return;
      setSnapshot((previous) => ({
        key,
        states: { ...(previous.key === key ? previous.states : {}), [id]: state },
      }));
    };
    const worker = async () => {
      while (cursor < refs.length && !controller.signal.aborted) {
        const image = refs[cursor++];
        update(image.id, { kind: "loading" });
        try {
          const blob = await scheduleQuestionImage(
            JSON.stringify([pendingId, questionId, owner]),
            controller.signal,
            () => fetchQuestionImage(pendingId, questionId, image.id, controller.signal),
          );
          if (controller.signal.aborted) return;
          const url = URL.createObjectURL(blob);
          urls.push(url);
          update(image.id, { kind: "loaded", url });
        } catch (error) {
          update(image.id, { kind: "error", code: questionImageErrorCode(error) });
        }
      }
    };
    void worker();
    void worker();
    return () => {
      controller.abort();
      urls.forEach((url) => URL.revokeObjectURL(url));
      setSnapshot({ key: "", states: {} });
    };
  }, [active, pendingId, questionId, imageKey, key, owner]);
  const retry = useCallback(() => setAttempt((value) => value + 1), []);
  return { states: snapshot.key === key ? snapshot.states : {}, retry };
}
