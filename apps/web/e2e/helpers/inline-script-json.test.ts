import { describe, expect, it } from "vitest";
import { serializeInlineScriptJSON } from "./inline-script-json";

describe("serializeInlineScriptJSON", () => {
  it("round-trips boot values without allowing data to close an inline script", () => {
    const payload = {
      initialState: { workspaceName: '</script><script>alert("fixture")</script>' },
      path: "a<b&c>d",
    };
    const json = serializeInlineScriptJSON(payload);
    expect(json).not.toContain("<");
    expect(JSON.parse(json)).toEqual(payload);
  });
});
