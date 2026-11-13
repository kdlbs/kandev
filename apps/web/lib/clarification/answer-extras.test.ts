import { describe, expect, it } from "vitest";
import type {
  ClarificationAnswer,
  ClarificationAnswerAnnotation,
  ClarificationAnswerAttachment,
} from "@/lib/types/http";
import { withAnswerExtras } from "./answer-extras";

const region: ClarificationAnswerAnnotation = {
  image_id: "img-1",
  x: 1,
  y: 2,
  width: 3,
  height: 4,
  image_width: 10,
  image_height: 10,
  comment: "Too tight",
};
const file: ClarificationAnswerAttachment = {
  attachment_id: "file-1",
  type: "image",
  name: "a.png",
  mime_type: "image/png",
  size_bytes: 1,
  delivery_mode: "prompt",
};

describe("withAnswerExtras", () => {
  it("adds ready files and current regions", () => {
    const answer: ClarificationAnswer = { question_id: "q1", selected_options: ["a"] };
    expect(withAnswerExtras(answer, [file], [region])).toEqual({
      ...answer,
      attachments: [file],
      annotations: [region],
    });
  });

  it("drops regions saved on an earlier attempt when the current list is empty", () => {
    const answer: ClarificationAnswer = { question_id: "q1", annotations: [region] };
    expect(withAnswerExtras(answer, [], [])).not.toHaveProperty("annotations");
    expect(withAnswerExtras(answer, [], undefined)).not.toHaveProperty("annotations");
  });

  it("returns the same answer when there is nothing to add or remove", () => {
    const answer: ClarificationAnswer = { question_id: "q1", custom_text: "ok" };
    expect(withAnswerExtras(answer, [], undefined)).toBe(answer);
  });

  it("reports regions on images over the size limit in a scaled pixel space", () => {
    const big = { ...region, image_width: 8000, image_height: 6000 };
    const [sent] = withAnswerExtras({ question_id: "q1" }, [], [big]).annotations!;
    expect(sent.image_width * sent.image_height).toBeLessThanOrEqual(16_000_000);
  });
});
