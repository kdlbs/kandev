import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PortListSection } from "./port-forward-list";

const backend = vi.hoisted(() => ({ url: "" }));
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: backend.url }),
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// @covers AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.9
describe.each([
  ["HTTPS", "https://dashboard.example:8443/base"],
  ["HTTP", "http://dashboard.example:8443/base"],
])("forwarded port URLs with an %s backend", (_scheme, baseUrl) => {
  it("displays, opens and copies the dedicated and proxy URLs", async () => {
    backend.url = baseUrl;
    const copy = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
    const openBrowserPanel = vi.fn();
    render(
      <TooltipProvider>
        <PortListSection
          detectedPorts={[]}
          manualPorts={[]}
          sessionId="session-1"
          loading={false}
          loaded
          onRefresh={vi.fn()}
          activeTunnels={new Map([[3000, 49152]])}
          pendingTunnels={new Set()}
          onTunnelStart={vi.fn()}
          onTunnelStop={vi.fn()}
          onOpenBrowserPanel={openBrowserPanel}
        />
      </TooltipProvider>,
    );

    const tunnelUrl = "http://dashboard.example:49152/";
    const proxyUrl = `${baseUrl}/port-proxy/session-1/3000/`;
    const tunnelRow = screen.getByText(tunnelUrl).parentElement!;
    const proxyRow = screen.getByText(proxyUrl).parentElement!;

    expect(
      within(tunnelRow).getByRole("link", { name: "Open in new tab" }).getAttribute("href"),
    ).toBe(tunnelUrl);
    expect(
      within(proxyRow).getByRole("link", { name: "Open in new tab" }).getAttribute("href"),
    ).toBe(proxyUrl);
    fireEvent.click(within(tunnelRow).getByRole("button", { name: "Copy URL" }));
    await waitFor(() => expect(copy).toHaveBeenCalledWith(tunnelUrl));
    fireEvent.click(within(proxyRow).getByRole("button", { name: "Copy URL" }));
    await waitFor(() => expect(copy).toHaveBeenCalledWith(proxyUrl));
    expect(within(tunnelRow).queryByRole("button", { name: "Open in browser panel" })).toBeNull();
    fireEvent.click(within(proxyRow).getByRole("button", { name: "Open in browser panel" }));
    expect(openBrowserPanel).toHaveBeenCalledWith(proxyUrl);
  });
});
