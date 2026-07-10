import { describe, expect, it } from "vitest";
import { resolveStartupPromptForManualDialog } from "./startup-prompt";

describe("resolveStartupPromptForManualDialog", () => {
  it("returns empty string for empty prompt", () => {
    expect(resolveStartupPromptForManualDialog("", "Fix bug")).toBe("");
  });

  it("substitutes TASK_TITLE inline", () => {
    expect(resolveStartupPromptForManualDialog("Start with {{TASK_TITLE}}.", "Refactor")).toBe(
      "Start with Refactor.",
    );
  });

  it("drops lines with unresolved TICKET_URL", () => {
    const prompt = "Read {{TICKET_URL}} carefully.\nThen begin work on {{TASK_TITLE}}.";
    expect(resolveStartupPromptForManualDialog(prompt, "Refactor billing")).toBe(
      "Then begin work on Refactor billing.",
    );
  });

  it("drops lines with unresolved TICKET_ID", () => {
    expect(
      resolveStartupPromptForManualDialog("Pick up {{TICKET_ID}}.\nDone.", "X"),
    ).toBe("Done.");
  });

  it("collapses whitespace after dropping ticket-only line", () => {
    const prompt = "Read {{TICKET_URL}}.\n\nStart with {{TASK_TITLE}}.";
    expect(resolveStartupPromptForManualDialog(prompt, "Refactor")).toBe(
      "Start with Refactor.",
    );
  });

  it("returns empty string when every line references a ticket", () => {
    const prompt = "Read {{TICKET_URL}}.\nAssignee: {{TICKET_ID}}.";
    expect(resolveStartupPromptForManualDialog(prompt, "X")).toBe("");
  });

  it("preserves plain lines untouched", () => {
    expect(resolveStartupPromptForManualDialog("Just start work.", "X")).toBe("Just start work.");
  });

  it("normalizes CRLF line endings to LF", () => {
    const prompt = "Read {{TICKET_URL}}.\r\nStart with {{TASK_TITLE}}.";
    expect(resolveStartupPromptForManualDialog(prompt, "Refactor")).toBe(
      "Start with Refactor.",
    );
  });

  it("does not leave stray CR characters in resolved output", () => {
    const prompt = "Start with {{TASK_TITLE}}.\r\nSecond line.";
    expect(resolveStartupPromptForManualDialog(prompt, "X")).toBe(
      "Start with X.\nSecond line.",
    );
  });
});
