import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { GetTaskPlanRenderer, UpdateTaskPlanRenderer } from "./document-renderers";

function plan(content: string | undefined) {
  return renderToStaticMarkup(
    <GetTaskPlanRenderer args={{ task_id: "t1" }} result={{ content }} status="complete" />,
  );
}

function updatedPlan(content: string | undefined) {
  return renderToStaticMarkup(
    <UpdateTaskPlanRenderer args={{ task_id: "t1" }} result={{ content }} status="complete" />,
  );
}

// The plan/document summary carries two counts. i18next selects a plural form on
// `count` alone, so lines own `count` and the character total rides along as a
// plain interpolated value. English must stay byte-identical.
describe("document summary", () => {
  it("uses the singular line label for single-line content", () => {
    expect(plan("one line")).toContain("1 line · 8 chars");
  });

  it("uses the plural line label for multi-line content", () => {
    expect(plan("a\nb\nc")).toContain("3 lines · 5 chars");
  });

  it("keeps 'chars' out of the plural decision", () => {
    // A single character with several lines must still pluralise on lines only,
    // matching the pre-i18n string exactly.
    expect(plan("\n\n")).toContain("3 lines · 2 chars");
  });

  it("falls back to the empty label when there is no content", () => {
    expect(updatedPlan(undefined)).toContain("empty");
  });
});
