import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkflowSyncSection } from "./workflow-sync-section";
import {
  config,
  Providers,
  transport,
} from "@/hooks/domains/settings/workflow-sync.lifetime.test-helpers";

const { fetchJson } = vi.hoisted(() => ({ fetchJson: vi.fn() }));
vi.mock("@/lib/api/client", async (original) => ({
  ...(await original<typeof import("@/lib/api/client")>()),
  fetchJson,
}));

const BRANCH = "workflow-sync-branch-input";
const DIRECTORY = "workflow-sync-directory-input";
const URL = "workflow-sync-url-input";
const SAVE = "workflow-sync-save";
const DIALOG = "workflow-sync-dialog";
const RETRY_BRANCH = "retry-current";
const RETRY_DIRECTORY = "retry-directory";
let wire: ReturnType<typeof transport>;
beforeEach(() => {
  wire = transport();
  fetchJson.mockImplementation(wire.fetch);
});
afterEach(async () => {
  cleanup();
  await act(async () => wire.drain());
  vi.restoreAllMocks();
});

function SourceSettings() {
  const [open, setOpen] = useState(true);
  return (
    <Providers>
      <button data-testid="source-reopen" onClick={() => setOpen(true)}>
        reopen source
      </button>
      <button data-testid="source-close" onClick={() => setOpen(false)}>
        close source
      </button>
      <WorkflowSyncSection workspaceId="A" dialogOpen={open} onDialogOpenChange={setOpen} />
    </Providers>
  );
}

async function mount(width = 1024) {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: width });
  window.dispatchEvent(new Event("resize"));
  render(<SourceSettings />);
  await waitFor(() => expect((screen.getByTestId(SAVE) as HTMLButtonElement).disabled).toBe(false));
}
function input(id: string) {
  return screen.getByTestId(id) as HTMLInputElement;
}
function edit(id: string, value: string) {
  fireEvent.change(input(id), { target: { value } });
}
function save() {
  const response = wire.hold("POST");
  const previous = wire.requests.filter((r) => r.method === "POST").length;
  fireEvent.click(screen.getByTestId(SAVE));
  expect(wire.requests.filter((r) => r.method === "POST")).toHaveLength(previous + 1);
  return response;
}

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.2, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.7
describe("real workflow source Save acknowledgement", () => {
  it.each([390, 1024])(
    "retains newer branch and keeps the real dialog open after an older accepted Save at %s",
    async (width) => {
      await mount(width);
      edit(BRANCH, "submitted-branch");
      const accepted = save();
      expect(JSON.parse(wire.requests.find((r) => r.method === "POST")!.body!).branch).toBe(
        "submitted-branch",
      );
      expect(input(BRANCH).disabled).toBe(false);
      edit(BRANCH, "newer-intended-branch");
      await act(async () => accepted.resolve(config("A", { branch: "submitted-branch" })));
      const stayedOpen = screen.queryByTestId(DIALOG) !== null;
      if (!stayedOpen) fireEvent.click(screen.getByTestId("source-reopen"));
      expect.soft(input(BRANCH).value).toBe("newer-intended-branch");
      expect.soft(stayedOpen).toBe(true);
    },
  );

  it.each([
    [DIRECTORY, "newer/workflow-definitions"],
    [URL, "https://github.com/another/source/tree/next/workflows"],
    [URL, "not a repository"],
    [URL, "git@github.com:team/A.git"],
    ["workflow-sync-interval-input", "601"],
  ])("retains raw %s=%s and current validity after accepted Save", async (id, value) => {
    await mount();
    const accepted = save();
    edit(id, value);
    await act(async () => accepted.resolve(config("A")));
    expect(screen.getByTestId(DIALOG)).toBeTruthy();
    expect(input(id).value).toBe(value);
    expect((screen.getByTestId(SAVE) as HTMLButtonElement).disabled).toBe(
      value === "not a repository",
    );
  });

  it("retains provider and polling edits made in the actual dialog", async () => {
    await mount();
    const accepted = save();
    fireEvent.mouseDown(screen.getByRole("tab", { name: "GitLab" }), { button: 0, ctrlKey: false });
    expect(screen.getByRole("tab", { name: "GitLab" }).getAttribute("aria-selected")).toBe("true");
    edit(URL, "group/sub/project");
    expect(input(URL).getAttribute("aria-invalid")).toBe("false");
    fireEvent.click(screen.getByTestId("workflow-sync-poll-toggle"));
    await act(async () => accepted.resolve(config("A")));
    expect(screen.getByTestId(DIALOG)).toBeTruthy();
    expect(input(URL).value).toBe("group/sub/project");
    expect(screen.getByTestId("workflow-sync-poll-toggle").getAttribute("aria-checked")).toBe(
      "false",
    );
    const next = save();
    const payload = JSON.parse(wire.requests.filter((r) => r.method === "POST")[1].body!);
    expect(payload).toMatchObject({
      provider: "gitlab",
      project_path: "group/sub/project",
      poll_enabled: false,
    });
    expect(payload).not.toHaveProperty("repo_owner");
    await act(async () => next.resolve(config("A", { ...payload, repo_owner: "", repo_name: "" })));
    expect(screen.queryByTestId(DIALOG)).toBeNull();
  });
});

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.3, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.4
describe("ordinary adoption and next Save", () => {
  it.each([390, 1024])(
    "unchanged normalized success closes and reopens canonical values at %s",
    async (width) => {
      await mount(width);
      edit(URL, "git@github.com:team/A.git");
      edit(BRANCH, " release ");
      edit(DIRECTORY, " workflows/ ");
      const accepted = save();
      await act(async () =>
        accepted.resolve(config("A", { branch: "release", path: "workflows" })),
      );
      expect(screen.queryByTestId(DIALOG)).toBeNull();
      fireEvent.click(screen.getByTestId("source-reopen"));
      expect(input(BRANCH).value).toBe("release");
      expect(input(DIRECTORY).value).toBe("workflows");
      expect(input(URL).value).toBe("https://github.com/team/A");
    },
  );

  it("keeps an old-baseline revert and sends the retained branch on the next Save", async () => {
    await mount();
    edit(BRANCH, "submitted");
    const accepted = save();
    edit(BRANCH, "main");
    await act(async () => accepted.resolve(config("A", { branch: "submitted" })));
    expect(input(BRANCH).value).toBe("main");
    expect(screen.getByTestId(DIALOG)).toBeTruthy();
    const next = save();
    expect(JSON.parse(wire.requests.filter((r) => r.method === "POST")[1].body!).branch).toBe(
      "main",
    );
    await act(async () => next.resolve(config("A")));
    expect(screen.queryByTestId(DIALOG)).toBeNull();
  });

  it("closes after edits return to the exact raw submission", async () => {
    await mount();
    edit(BRANCH, " submitted ");
    const accepted = save();
    edit(BRANCH, "away");
    edit(BRANCH, " submitted ");
    await act(async () => accepted.resolve(config("A", { branch: "submitted" })));
    expect(screen.queryByTestId(DIALOG)).toBeNull();
  });

  it("closes a first Save without treating its config publication as a newer edit", async () => {
    wire.configs.delete("A");
    render(<SourceSettings />);
    await waitFor(() => expect(input(URL).disabled).toBe(false));
    edit(URL, "https://github.com/team/first");
    const accepted = save();
    await act(async () => accepted.resolve(config("A", { repo_name: "first" })));
    expect(screen.queryByTestId(DIALOG)).toBeNull();
  });
});

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.5, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-003.6
describe("current failure and explicit dismissal", () => {
  it("current rejection preserves raw controls and retries after invalid-link correction", async () => {
    await mount();
    const rejected = save();
    edit(URL, "invalid current link");
    edit(BRANCH, RETRY_BRANCH);
    edit(DIRECTORY, RETRY_DIRECTORY);
    await act(async () => rejected.reject(new Error("current-save-rejected")));
    expect(screen.getByTestId(DIALOG)).toBeTruthy();
    expect(input(URL).value).toBe("invalid current link");
    expect(input(BRANCH).value).toBe(RETRY_BRANCH);
    expect(input(DIRECTORY).value).toBe(RETRY_DIRECTORY);
    expect((screen.getByTestId(SAVE) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("toast-container").textContent).toContain("current-save-rejected");
    edit(URL, "https://github.com/team/retry");
    const retried = save();
    expect(JSON.parse(wire.requests.filter((r) => r.method === "POST")[1].body!)).toMatchObject({
      repo_name: "retry",
      branch: RETRY_BRANCH,
      path: RETRY_DIRECTORY,
    });
    await act(async () =>
      retried.resolve(
        config("A", { repo_name: "retry", branch: RETRY_BRANCH, path: RETRY_DIRECTORY }),
      ),
    );
    expect(screen.queryByTestId(DIALOG)).toBeNull();
  });

  it("does not dismiss a reopened dialog and preserves its newer draft", async () => {
    await mount();
    const accepted = save();
    edit(BRANCH, "keep-on-reopen");
    fireEvent.click(screen.getByTestId("source-close"));
    fireEvent.click(screen.getByTestId("source-reopen"));
    await act(async () => accepted.resolve(config("A")));
    expect(screen.getByTestId(DIALOG)).toBeTruthy();
    expect(input(BRANCH).value).toBe("keep-on-reopen");
  });
});
