import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  extractSentryShortId,
  SENTRY_SHORT_ID_EXTRACT_RE,
  SENTRY_SHORT_ID_RE,
} from "@/components/sentry/sentry-issue-common";
import { useSentryInstances } from "@/hooks/domains/sentry/use-sentry-availability";
import { getSentryIssue } from "@/lib/api/domains/sentry-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import type { SentryConfig } from "@/lib/types/sentry";
import { TaskExternalLinkDialog } from "./task-external-link-dialog";

vi.mock("@/lib/api/domains/sentry-api", () => ({ getSentryIssue: vi.fn() }));
vi.mock("@/lib/api/domains/kanban-api", () => ({ updateTask: vi.fn() }));
vi.mock("@/hooks/domains/sentry/use-sentry-availability", () => ({
  useSentryInstances: vi.fn(),
}));

const instance: SentryConfig = {
  id: "instance-1",
  workspaceId: "workspace-1",
  name: "Test instance",
  authMethod: "auth_token",
  url: "https://sentry.io",
  hasSecret: true,
  lastOk: true,
  lastError: "",
  createdAt: "",
  updatedAt: "",
};

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

function renderSentryDialog(title = "Fix login") {
  vi.mocked(useSentryInstances).mockReturnValue({
    loading: false,
    instances: [instance],
    healthy: [instance],
    available: true,
    state: "single",
  });
  const onOpenChange = vi.fn();
  render(
    <StateProvider>
      <ToastProvider>
        <TaskExternalLinkDialog
          open
          onOpenChange={onOpenChange}
          provider="sentry"
          task={{ id: "task-1", title }}
          workspaceId="workspace-1"
        />
      </ToastProvider>
    </StateProvider>,
  );
  return onOpenChange;
}

async function submit(input: string) {
  fireEvent.change(screen.getByTestId("task-external-link-input"), { target: { value: input } });
  await waitFor(() =>
    expect((screen.getByTestId("task-external-link-submit") as HTMLButtonElement).disabled).toBe(
      false,
    ),
  );
  fireEvent.click(screen.getByTestId("task-external-link-submit"));
}

describe("Sentry short IDs", () => {
  it("recognizes the entire provider alphabet and decoder aliases", () => {
    for (const character of "0123456789ABCDEFGHJKMNPQRSTVWXYZ OIL".replace(" ", "")) {
      const id = `WEB-1${character}`;
      expect(SENTRY_SHORT_ID_RE.test(id), id).toBe(true);
      expect(SENTRY_SHORT_ID_RE.test(id.toLowerCase()), id).toBe(true);
      expect(id.match(SENTRY_SHORT_ID_EXTRACT_RE)?.[0], id).toBe(id);
      expect(extractSentryShortId(id.toLowerCase()), id).toBe(id);
    }
  });

  it("preserves slug separators and complete token boundaries", () => {
    expect(extractSentryShortId("https://example.test/issues/FOO_BAR-WEB-1A/")).toBe(
      "FOO_BAR-WEB-1A",
    );
    expect(extractSentryShortId("WEB-123")).toBe("WEB-123");
    expect(SENTRY_SHORT_ID_RE.test("WEB-1AU")).toBe(false);
    expect("WEB-1AU".match(SENTRY_SHORT_ID_EXTRACT_RE)).toBeNull();
    expect(extractSentryShortId("WEB-1AU")).toBeNull();
    expect(extractSentryShortId("WEB-123U")).toBeNull();
    expect(extractSentryShortId("WEB-1A-U")).toBeNull();
    expect(extractSentryShortId("https://my-org.sentry.io/issues/WEB-1A/")).toBe("WEB-1A");
    expect(
      extractSentryShortId("https://sentry.io/organizations/my-org/issues/WEB-1A-U/"),
    ).toBeNull();
    expect(extractSentryShortId("https://my-org.sentry.io/issues/WEB-A/")).toBe("WEB-A");
    expect(extractSentryShortId("https://my-org.sentry.io/issues/")).toBeNull();
    expect(extractSentryShortId("WEB-")).toBeNull();
    expect(extractSentryShortId(null)).toBeNull();
  });
});

describe("link a Sentry issue", () => {
  it("replaces a previous base32 prefix with the resolved canonical key", async () => {
    vi.mocked(getSentryIssue).mockResolvedValue({ shortId: "WEB-1B" } as never);
    vi.mocked(updateTask).mockResolvedValue({ id: "task-1", title: "WEB-1B: Fix login" } as never);
    const onOpenChange = renderSentryDialog("WEB-1A: Fix login");
    await submit("WEB-1B");
    await waitFor(() =>
      expect(getSentryIssue).toHaveBeenCalledWith("workspace-1", "instance-1", "WEB-1B"),
    );
    await waitFor(() =>
      expect(updateTask).toHaveBeenCalledWith("task-1", { title: "WEB-1B: Fix login" }),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it.each([
    { input: "WEB-1A", key: "WEB-1A", canonical: "WEB-1A" },
    { input: "WEB-123", key: "WEB-123", canonical: "WEB-123" },
    {
      input: "https://sentry.io/organizations/my-org/issues/WEB-1A/",
      key: "WEB-1A",
      canonical: "WEB-1A",
    },
    { input: "https://my-org.sentry.io/issues/WEB-1A/", key: "WEB-1A", canonical: "WEB-1A" },
    {
      input: "https://sentry.io/organizations/acme/issues/123456/",
      key: "123456",
      canonical: "WEB-1A",
    },
    {
      input: "https://sentry.io/organizations/my-org/issues/123456/",
      key: "123456",
      canonical: "WEB-1A",
    },
  ])("submits $input through the selected instance", async ({ input, key, canonical }) => {
    vi.mocked(getSentryIssue).mockResolvedValue({ shortId: canonical } as never);
    vi.mocked(updateTask).mockResolvedValue({
      id: "task-1",
      title: `${canonical}: Fix login`,
    } as never);
    const onOpenChange = renderSentryDialog();
    await submit(input);
    await waitFor(() =>
      expect(getSentryIssue).toHaveBeenCalledWith("workspace-1", "instance-1", key),
    );
    await waitFor(() =>
      expect(updateTask).toHaveBeenCalledWith("task-1", { title: `${canonical}: Fix login` }),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("leaves the task untouched when the provider rejects the issue", async () => {
    vi.mocked(getSentryIssue).mockRejectedValue(new Error("Issue not found"));
    const onOpenChange = renderSentryDialog();
    await submit("WEB-1A");
    expect((await screen.findByTestId("task-external-link-error")).textContent).toBe(
      "Issue not found",
    );
    expect(updateTask).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it.each([
    "WEB-1AU",
    "WEB-1A-U",
    "https://my-org.sentry.io/issues/WEB-1A-U/",
    "https://my-org.sentry.io/issues/",
  ])("does not send invalid %s to the provider", async (input) => {
    const onOpenChange = renderSentryDialog();
    await submit(input);
    expect(await screen.findByTestId("task-external-link-error")).toBeTruthy();
    expect(getSentryIssue).not.toHaveBeenCalled();
    expect(updateTask).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});
