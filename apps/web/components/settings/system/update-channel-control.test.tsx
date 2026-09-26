import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";

import type { UpdatesChannel, UpdatesResponse } from "@/lib/types/system";
import { SettingsSaveProvider, useSettingsSaveContributor } from "../settings-save-provider";
import { useUpdateChannelDraft } from "./update-channel-control";

type Deferred = {
  promise: Promise<void>;
  resolve: () => void;
};

function deferred(): Deferred {
  let resolve: () => void = () => undefined;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function BlockingContributor({
  pending,
  onSaveStarted,
}: {
  pending: Deferred;
  onSaveStarted: () => void;
}) {
  const [isDirty, setIsDirty] = useState(true);
  useSettingsSaveContributor({
    id: "blocking-save",
    order: 0,
    revision: 1,
    isDirty,
    save: async () => {
      onSaveStarted();
      await pending.promise;
      setIsDirty(false);
    },
    discard: () => setIsDirty(false),
  });
  return null;
}

function ChannelDraft({
  saveChannel,
}: {
  saveChannel: (channel: UpdatesChannel) => Promise<UpdatesResponse>;
}) {
  const { draft, isDirty, setDraft } = useUpdateChannelDraft(
    {
      current: "v1.0.0",
      latest: "v1.0.1",
      latest_url: "https://example.test/v1.0.1",
      latest_checked_at: "2026-08-01T00:00:00Z",
      update_available: true,
      channel: "stable",
      channel_editable: true,
      channel_unsupported_reason: "",
      install: { running_as_service: true, managed_service: true },
      apply_supported: true,
    },
    saveChannel,
  );
  return (
    <>
      <button type="button" onClick={() => setDraft("nightly")}>
        Nightly
      </button>
      <button type="button" onClick={() => setDraft("stable")}>
        Stable
      </button>
      <output data-testid="channel-draft" data-dirty={isDirty}>
        {draft}
      </output>
    </>
  );
}

afterEach(cleanup);

it("preserves an edit made before its queued contributor starts saving", async () => {
  const pending = deferred();
  const onSaveStarted = vi.fn();
  const saveChannel = vi.fn().mockResolvedValue({
    channel: "nightly",
  } as UpdatesResponse);
  render(
    <SettingsSaveProvider>
      <BlockingContributor pending={pending} onSaveStarted={onSaveStarted} />
      <ChannelDraft saveChannel={saveChannel} />
    </SettingsSaveProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Nightly" }));
  fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
  await waitFor(() => expect(onSaveStarted).toHaveBeenCalledOnce());
  expect(saveChannel).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: "Stable" }));
  await act(async () => pending.resolve());
  await waitFor(() => expect(saveChannel).toHaveBeenCalledWith("nightly"));

  expect(screen.getByTestId("channel-draft").textContent).toBe("stable");
  expect(screen.getByTestId("channel-draft").getAttribute("data-dirty")).toBe("true");
});
