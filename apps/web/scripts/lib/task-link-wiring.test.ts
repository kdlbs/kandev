import path from "node:path";

import { ESLint } from "eslint";
import { describe, expect, it } from "vitest";

const WEB_DIR = path.resolve(import.meta.dirname, "../..");
const eslint = new ESLint({ cwd: WEB_DIR });

describe("task link eslint wiring", () => {
  it("guards a new production component in an arbitrary directory", async () => {
    const [result] = await eslint.lintText(
      `export function NewTaskLink({ taskId }: { taskId: string }) {
  return <a href={\`/t/\${taskId}\`} />;
}
`,
      { filePath: path.join(WEB_DIR, "components/new-surface/task-link-fixture.tsx") },
    );

    expect(result.messages).toEqual([
      expect.objectContaining({
        ruleId: "task-links/no-task-link-bypass",
        severity: 2,
      }),
    ]);
  });
});
