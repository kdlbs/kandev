import path from "node:path";

import { ESLint } from "eslint";
import { describe, expect, it } from "vitest";

const WEB_DIR = path.resolve(import.meta.dirname, "../..");
const eslint = new ESLint({
  cwd: WEB_DIR,
  overrideConfigFile: path.join(WEB_DIR, "eslint.i18n.config.mjs"),
});

describe("focused i18n eslint config", () => {
  it("resolves external inline rules while keeping literal detection active", async () => {
    const [result] = await eslint.lintText(
      `// eslint-disable-next-line react-hooks/exhaustive-deps
export const content = <div>Visible copy</div>;
`,
      { filePath: path.join(WEB_DIR, "components/i18n-config-fixture.tsx") },
    );

    expect(result.messages).toEqual([
      expect.objectContaining({
        ruleId: "i18next/no-literal-string",
        severity: 2,
      }),
    ]);
  });

  it("preserves the test-file ignore", async () => {
    const results = await eslint.lintText(`export const content = <div>Test fixture copy</div>;`, {
      filePath: path.join(WEB_DIR, "components/i18n-config-fixture.test.tsx"),
      warnIgnored: false,
    });

    expect(results).toEqual([]);
  });
});
