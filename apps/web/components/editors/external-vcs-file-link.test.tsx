import { cleanup, render, renderHook, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ExternalVcsFileURL } from "@/lib/utils/external-vcs-file-url";

const mocks = vi.hoisted(() => ({
  link: null as ExternalVcsFileURL | null,
}));

vi.mock("@/hooks/domains/workspace/use-external-vcs-file-link", () => ({
  useExternalVcsFileLink: () => mocks.link,
}));

vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => ({ files: { "src/app.ts": { status: "modified" } } }),
  useSessionGitStatusByRepo: () => [
    {
      repository_name: "frontend",
      status: { files: { "src/app.ts": { status: "renamed", old_path: "src/old.ts" } } },
    },
  ],
}));

import { ExternalVcsFileLink, useExternalVcsFileStatus } from "./external-vcs-file-link";

function renderLink(size: "xs" | "sm" | "touch" = "xs") {
  return render(
    <TooltipProvider>
      <ExternalVcsFileLink filePath="src/app.ts" size={size} />
    </TooltipProvider>,
  );
}

afterEach(() => {
  cleanup();
  mocks.link = null;
});

describe("ExternalVcsFileLink", () => {
  it("selects file status from the exact repository", () => {
    const { result } = renderHook(() =>
      useExternalVcsFileStatus("src/app.ts", "session-1", "frontend"),
    );

    expect(result.current).toEqual({ status: "renamed", old_path: "src/old.ts" });
  });

  it.each([
    ["github", "GitHub", "github-provider-icon"],
    ["gitlab", "GitLab", "gitlab-provider-icon"],
    ["azure_devops", "Azure DevOps", "azure-devops-icon"],
  ] as const)("renders a branded, safe new-tab action for %s", (provider, label, iconTestId) => {
    mocks.link = {
      provider,
      url: `https://example.com/${provider}/file`,
      revision: "main",
      path: "src/app.ts",
    };

    renderLink();

    const link = screen.getByRole("link", { name: `Open file in ${label}` });
    expect(link.getAttribute("href")).toBe(mocks.link.url);
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    expect(link.getAttribute("title")).toBe(`Open file in ${label}`);
    expect(screen.getByTestId(iconTestId)).not.toBeNull();
  });

  it.each([
    ["xs", "size-5"],
    ["sm", "size-6"],
    ["touch", "size-11"],
  ] as const)("uses the %s action size", (size, expectedClass) => {
    mocks.link = {
      provider: "github",
      url: "https://github.com/acme/web/blob/main/src/app.ts",
      revision: "main",
      path: "src/app.ts",
    };

    renderLink(size);

    expect(screen.getByRole("link").classList.contains(expectedClass)).toBe(true);
  });

  it("renders nothing when no safe external URL resolves", () => {
    renderLink();
    expect(screen.queryByRole("link")).toBeNull();
  });
});
