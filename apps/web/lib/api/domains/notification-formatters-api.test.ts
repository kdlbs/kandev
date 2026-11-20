import { beforeEach, expect, it, vi } from "vitest";
import { fetchJson } from "../client";
import {
  getNotificationFormatter,
  selectNotificationFormatter,
} from "./notification-formatters-api";

vi.mock("../client", () => ({ fetchJson: vi.fn() }));
beforeEach(() => vi.clearAllMocks());

it("encodes the owned provider and retains the observed activation revision", async () => {
  vi.mocked(fetchJson).mockResolvedValue({ binding: null, formatters: [], events: [] });
  await getNotificationFormatter("mail/id");
  expect(fetchJson).toHaveBeenCalledWith(
    "/api/v1/notification-providers/mail%2Fid/formatter",
    undefined,
  );
  await selectNotificationFormatter("mail/id", "revision-a", null);
  expect(fetchJson).toHaveBeenLastCalledWith("/api/v1/notification-providers/mail%2Fid/formatter", {
    init: {
      method: "PUT",
      body: JSON.stringify({ expected_revision: "revision-a", binding: null }),
    },
  });
});
