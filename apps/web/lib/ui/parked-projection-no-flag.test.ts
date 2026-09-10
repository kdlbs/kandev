import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// Web-side mirror of internal/orchestrator/parked_projection_no_flag_test.go
// (AC-35, round-3 code review): every rendering surface this feature touches
// must project parked_on_background_work on its own terms and must not read
// the (unrelated, older) claudeBackgroundPromptHandoff feature flag to decide
// whether to render it.
function source(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

const FORBIDDEN = ["claudeBackgroundPromptHandoff", "claudeBackgroundPromptHandoffEnabled"];

// Every non-test file this feature's rendering path touches (spec.md's
// E2E / rendering surfaces touched: sidebar, board, /tasks row, session
// switcher, reopen menu, mobile). Listed explicitly, mirroring the backend
// test's own no-glob convention — a new call site must be appended here
// rather than exempted.
const RENDERING_SOURCE_FILES = [
  "../../components/kanban-card-content.tsx",
  "../../app/tasks/rich-task-list-row.tsx",
  "../../components/task/task-item.tsx",
  "../../components/task/sessions-dropdown.tsx",
  "../../components/task/session-reopen-menu.tsx",
  "../../components/task/mobile/mobile-sessions-section.tsx",
  "./state-icons.tsx",
];

describe("parked-on-background-work rendering path never reads the unrelated background-prompt-handoff flag", () => {
  it.each(RENDERING_SOURCE_FILES)("%s", (path) => {
    const contents = source(path);
    for (const needle of FORBIDDEN) {
      expect(contents).not.toContain(needle);
    }
  });
});
