import { describe, expect, it } from "vitest";

import { shouldSkipInitialTasksFetch } from "./tasks-page-fetch-policy";

describe("shouldSkipInitialTasksFetch", () => {
  it("skips the first default-page fetch when boot data is present", () => {
    expect(
      shouldSkipInitialTasksFetch({
        hasInitialData: true,
        alreadySkipped: false,
        pageIndex: 0,
        debouncedQuery: "",
        showArchived: false,
      }),
    ).toBe(true);
  });

  it("does not skip after filters or pagination changed", () => {
    expect(
      shouldSkipInitialTasksFetch({
        hasInitialData: true,
        alreadySkipped: false,
        pageIndex: 1,
        debouncedQuery: "",
        showArchived: false,
      }),
    ).toBe(false);
    expect(
      shouldSkipInitialTasksFetch({
        hasInitialData: true,
        alreadySkipped: false,
        pageIndex: 0,
        debouncedQuery: "search",
        showArchived: false,
      }),
    ).toBe(false);
  });
});
