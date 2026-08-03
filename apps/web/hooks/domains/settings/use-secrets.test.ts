import { describe, expect, it } from "vitest";
import { filterGlobalSecrets } from "./use-secrets";

describe("filterGlobalSecrets", () => {
  it("keeps only global entries for shared profile selectors", () => {
    expect(
      filterGlobalSecrets([
        {
          id: "global",
          name: "Global",
          has_value: true,
          scope: "global",
          created_at: "",
          updated_at: "",
        },
        {
          id: "workspace",
          name: "Workspace",
          has_value: true,
          scope: "workspace",
          created_at: "",
          updated_at: "",
        },
        { id: "legacy", name: "Legacy", has_value: true, created_at: "", updated_at: "" },
      ]),
    ).toMatchObject([{ id: "global" }, { id: "legacy" }]);
  });
});
