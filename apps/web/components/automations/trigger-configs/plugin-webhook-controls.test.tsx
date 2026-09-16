import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { PluginWebhookControls } from "./plugin-webhook-controls";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import type { AutomationTrigger } from "@/lib/types/automation";

const { setError } = vi.hoisted(() => ({ setError: vi.fn() }));
vi.mock("@/hooks/use-plugin-webhook-controls", () => ({
  useWebhookControls: () => ({
    binding: { id: "b" },
    receipts: [],
    busy: false,
    error: false,
    secret: null,
    saved: true,
    act: vi.fn(),
    refresh: vi.fn(),
    url: "http://localhost/api/hook",
    setError,
    setSecret: vi.fn(),
  }),
}));
vi.mock("@/lib/utils/copy-to-clipboard", () => ({
  copyToClipboard: vi.fn().mockResolvedValue(false),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
it("reports a resolved clipboard failure", async () => {
  render(<PluginWebhookControls trigger={{ id: "t" } as AutomationTrigger} dirty={false} />);
  fireEvent.click(screen.getByRole("button", { name: "automations:pluginWebhookCopy" }));
  await waitFor(() => expect(setError).toHaveBeenCalledWith(true));
  expect(copyToClipboard).toHaveBeenCalledWith("http://localhost/api/hook");
});
