import { describe, expect, it } from "vitest";
import { resolveTaskRowPresentation } from "./task-row-presentation";

describe("resolveTaskRowPresentation", () => {
  it("keeps the configured order while hiding disabled details", () => {
    expect(
      resolveTaskRowPresentation(
        {
          detailsEnabled: true,
          detailOrder: ["pull_request_number", "repository", "relative_time"],
          visibleDetails: ["repository", "relative_time"],
          trailing: "git_changes",
        },
        { showRepository: true },
      ),
    ).toMatchObject({
      detailOrder: ["repository", "relative_time"],
      showRepository: true,
      showRelativeTime: true,
      showPullRequestNumber: false,
    });
  });

  it("suppresses repository metadata when the view groups by repository", () => {
    const resolved = resolveTaskRowPresentation(undefined, { showRepository: false });
    expect(resolved.showRepository).toBe(false);
    expect(resolved.showPullRequestNumber).toBe(true);
  });

  it("moves relative time to the trailing slot without duplicating it", () => {
    const resolved = resolveTaskRowPresentation(
      {
        detailsEnabled: true,
        detailOrder: ["relative_time", "repository", "pull_request_number"],
        visibleDetails: ["relative_time", "repository", "pull_request_number"],
        trailing: "relative_time",
      },
      { showRepository: true },
    );
    expect(resolved.showRelativeTime).toBe(false);
    expect(resolved.detailOrder).toContain("relative_time");
    expect(resolved.trailing).toBe("relative_time");
  });
});
