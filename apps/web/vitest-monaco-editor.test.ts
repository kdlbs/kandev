import { describe, expect, it } from "vitest";
import * as monacoImport from "monaco-editor";

const monaco = monacoImport as typeof monacoImport & {
  __KANDEV_VITEST_STUB__?: boolean;
};
const typeScriptLanguages = monaco.languages.typescript as unknown as {
  javascriptDefaults?: unknown;
  typescriptDefaults?: unknown;
};

describe("Vitest Monaco boundary", () => {
  it("uses the unit-test stub with the required runtime contract", () => {
    expect({
      isStub: monaco.__KANDEV_VITEST_STUB__,
      uri: monaco.Uri.parse("file:///workspace/source.ts").toString(),
      hasTypeScriptDefaults: Boolean(typeScriptLanguages.typescriptDefaults),
      hasJavaScriptDefaults: Boolean(typeScriptLanguages.javascriptDefaults),
    }).toEqual({
      isStub: true,
      uri: "file:///workspace/source.ts",
      hasTypeScriptDefaults: true,
      hasJavaScriptDefaults: true,
    });
  });
});
