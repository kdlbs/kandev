import { describe, expect, it } from "vitest";
import { pipelineSummary } from "./mr-overview-section";

describe("pipelineSummary", () => {
  it("does not present unknown job totals as zero passing out of zero", () => {
    expect(
      pipelineSummary({
        id: 1,
        iid: 2,
        status: "running",
        source: "merge_request_event",
        ref: "main",
        sha: "abc123",
        web_url: "",
        jobs_total: 0,
        jobs_passing: 0,
      }),
    ).toBe("running");
  });
});
