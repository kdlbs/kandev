import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  MCPAttachmentHistory,
  MCPAttachmentServer,
} from "@/lib/state/slices/session-runtime/types";
import { McpIndicator } from "./mcp-indicator";

const testCopy = vi.hoisted(() => ({
  thirdPartyCatalogUnavailable: "Kandev does not inspect tools from this server.",
}));
const MCP_STATUS_TRIGGER_TEST_ID = "mcp-status-trigger";

const responsiveMock = vi.hoisted(() => ({
  breakpoint: "desktop" as "mobile" | "tablet" | "desktop",
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: responsiveMock.breakpoint,
    isMobile: responsiveMock.breakpoint === "mobile",
    isTablet: responsiveMock.breakpoint === "tablet",
    isDesktop: responsiveMock.breakpoint !== "mobile",
    isCompactDesktop: false,
    isFullDesktop: responsiveMock.breakpoint === "desktop",
    isFinePointer: responsiveMock.breakpoint !== "tablet",
    usesDesktopWorkbench: responsiveMock.breakpoint !== "mobile",
  }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) => {
      const labels: Record<string, string> = {
        "task:mcpExplorerTitle": "MCP servers",
        "task:mcpExplorerDescription": "Review MCP connection status and available tools.",
        "task:mcpServerList": "Servers",
        "task:mcpStatusActive": "Active",
        "task:mcpStatusDelivered": "Delivered, connection unverified",
        "task:mcpToolCatalog": "Tool catalog",
        "task:mcpToolCatalogUnavailable": "The Kandev tool catalog is unavailable.",
        "task:mcpThirdPartyCatalogUnavailable": testCopy.thirdPartyCatalogUnavailable,
        "task:mcpToolCount": `Tools: ${values?.count ?? 0}`,
        "task:mcpStoredToolCount": `Showing ${values?.count ?? 0}`,
        "task:mcpNoTools": "No tools were reported.",
        "task:mcpBackToServers": "Back to servers",
        "task:mcpClose": "Close",
        "task:mcpTransport": "Transport",
        "task:mcpTarget": "Target",
        "task:mcpConnectionId": "Connection ID",
        "task:mcpStatusUnknown": "Unknown",
        "task:mcpToolCatalogTruncated": "Only the first tools are shown.",
        "task:mcpToolCatalogNotLoaded": "The tool catalog is not loaded yet.",
        "task:mcpNoServers": "No MCP servers are configured.",
        "task:showMcpConnectionStatus": "Show MCP connection status",
      };
      return labels[key] ?? key;
    },
  }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@kandev/ui/dialog", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  const DialogContext = React.createContext<{
    open: boolean;
    onOpenChange: (open: boolean) => void;
  } | null>(null);
  return {
    Dialog: ({
      children,
      open,
      onOpenChange,
    }: {
      children: React.ReactNode;
      open: boolean;
      onOpenChange: (open: boolean) => void;
    }) => (
      <DialogContext.Provider value={{ open, onOpenChange }}>{children}</DialogContext.Provider>
    ),
    DialogTrigger: ({ children }: { children: React.ReactElement }) => {
      const context = React.useContext(DialogContext);
      return React.cloneElement(children as React.ReactElement<{ onClick?: () => void }>, {
        onClick: () => context?.onOpenChange(true),
      });
    },
    DialogContent: ({
      children,
      enterConfirms: _enterConfirms,
      ...props
    }: React.HTMLAttributes<HTMLDivElement> & { enterConfirms?: boolean }) => {
      const context = React.useContext(DialogContext);
      return context?.open ? (
        <div role="dialog" {...props}>
          {children}
        </div>
      ) : null;
    },
    DialogHeader: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
      <div {...props}>{children}</div>
    ),
    DialogTitle: ({ children, ...props }: React.HTMLAttributes<HTMLHeadingElement>) => (
      <h2 {...props}>{children}</h2>
    ),
    DialogDescription: ({ children, ...props }: React.HTMLAttributes<HTMLParagraphElement>) => (
      <p {...props}>{children}</p>
    ),
  };
});

vi.mock("@kandev/ui/drawer", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  const DrawerContext = React.createContext<{
    open: boolean;
    onOpenChange: (open: boolean) => void;
  } | null>(null);
  return {
    Drawer: ({
      children,
      open,
      onOpenChange,
    }: {
      children: React.ReactNode;
      open: boolean;
      onOpenChange: (open: boolean) => void;
    }) => (
      <DrawerContext.Provider value={{ open, onOpenChange }}>{children}</DrawerContext.Provider>
    ),
    DrawerTrigger: ({ children }: { children: React.ReactElement }) => {
      const context = React.useContext(DrawerContext);
      return React.cloneElement(children as React.ReactElement<{ onClick?: () => void }>, {
        onClick: () => context?.onOpenChange(true),
      });
    },
    DrawerContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => {
      const context = React.useContext(DrawerContext);
      return context?.open ? (
        <div role="dialog" {...props}>
          {children}
        </div>
      ) : null;
    },
    DrawerHeader: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
      <div {...props}>{children}</div>
    ),
    DrawerTitle: ({ children, ...props }: React.HTMLAttributes<HTMLHeadingElement>) => (
      <h2 {...props}>{children}</h2>
    ),
    DrawerDescription: ({ children, ...props }: React.HTMLAttributes<HTMLParagraphElement>) => (
      <p {...props}>{children}</p>
    ),
  };
});

const attachmentHistory: MCPAttachmentHistory = {
  version: 1,
  current: {
    attachment_attempt_id: "attempt-1",
    started_at: "2026-08-16T00:00:00Z",
    servers: [
      {
        name: "kandev",
        source: "kandev",
        status: "active",
        tools_listed_at: "2026-08-16T00:01:00Z",
        tools: [{ name: "create_task_kandev", description: "Create a task" }],
        tool_count: 1,
      },
      {
        name: "filesystem",
        source: "profile",
        status: "delivered",
        summary: "Connection delivered",
      },
    ],
  },
};

function historyForServers(servers: MCPAttachmentServer[]): MCPAttachmentHistory {
  return {
    ...attachmentHistory,
    current: { ...attachmentHistory.current, servers },
  };
}

function renderOpenExplorer(servers: MCPAttachmentServer[]) {
  render(
    <McpIndicator
      mcpServers={servers.map((server) => server.name)}
      attachmentHistory={historyForServers(servers)}
    />,
  );
  fireEvent.click(screen.getByTestId(MCP_STATUS_TRIGGER_TEST_ID));
}

afterEach(() => cleanup());

beforeEach(() => {
  responsiveMock.breakpoint = "desktop";
});

describe("MCP server explorer", () => {
  it("opens a wide desktop explorer and changes the detail server", () => {
    render(
      <McpIndicator mcpServers={["kandev", "filesystem"]} attachmentHistory={attachmentHistory} />,
    );

    fireEvent.click(screen.getByTestId(MCP_STATUS_TRIGGER_TEST_ID));
    expect(screen.getByRole("dialog").getAttribute("data-testid")).toBe("mcp-server-explorer");
    expect(screen.getByText("create_task_kandev")).toBeTruthy();
    fireEvent.click(screen.getByTestId("mcp-server-row-filesystem"));
    expect(screen.getByText(testCopy.thirdPartyCatalogUnavailable)).toBeTruthy();
    expect(screen.queryByText("Create a task")).toBeNull();

    fireEvent.click(screen.getByTestId("mcp-explorer-close"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps the selected server while live attachment evidence updates", () => {
    const { rerender } = render(
      <McpIndicator mcpServers={["kandev", "filesystem"]} attachmentHistory={attachmentHistory} />,
    );

    fireEvent.click(screen.getByTestId(MCP_STATUS_TRIGGER_TEST_ID));
    fireEvent.click(screen.getByTestId("mcp-server-row-filesystem"));
    expect(screen.getByText(testCopy.thirdPartyCatalogUnavailable)).toBeTruthy();

    const updatedHistory: MCPAttachmentHistory = {
      ...attachmentHistory,
      current: {
        ...attachmentHistory.current,
        servers: attachmentHistory.current.servers?.map((server) =>
          server.name === "filesystem" ? { ...server, status: "connected" } : server,
        ),
      },
    };
    rerender(
      <McpIndicator mcpServers={["kandev", "filesystem"]} attachmentHistory={updatedHistory} />,
    );

    expect(screen.getByText(testCopy.thirdPartyCatalogUnavailable)).toBeTruthy();
  });

  it("explains an unloaded Kandev catalog", () => {
    renderOpenExplorer([{ name: "kandev", source: "kandev", status: "unknown" }]);
    expect(screen.getByText("The tool catalog is not loaded yet.")).toBeTruthy();
  });

  it("explains an unavailable Kandev catalog", () => {
    renderOpenExplorer([{ name: "kandev", source: "kandev", status: "failed" }]);
    expect(screen.getByText("The Kandev tool catalog is unavailable.")).toBeTruthy();
  });

  it("explains an empty Kandev catalog", () => {
    renderOpenExplorer([
      {
        name: "kandev",
        source: "kandev",
        status: "active",
        tools_listed_at: "2026-08-16T00:01:00Z",
        tools: [],
        tool_count: 0,
      },
    ]);
    expect(screen.getByText("No tools were reported.")).toBeTruthy();
  });

  it("explains a truncated Kandev catalog", () => {
    renderOpenExplorer([
      {
        name: "kandev",
        source: "kandev",
        status: "active",
        tools_listed_at: "2026-08-16T00:01:00Z",
        tools: [{ name: "create_task_kandev", description: "Create a task" }],
        tool_count: 129,
        tool_catalog_truncated: true,
      },
    ]);
    expect(screen.getByText("Showing 1")).toBeTruthy();
    expect(screen.getByText("Only the first tools are shown.")).toBeTruthy();
  });

  it("uses a phone list-to-detail flow with a visible Back control", () => {
    responsiveMock.breakpoint = "mobile";
    render(
      <McpIndicator mcpServers={["kandev", "filesystem"]} attachmentHistory={attachmentHistory} />,
    );

    fireEvent.click(screen.getByTestId(MCP_STATUS_TRIGGER_TEST_ID));
    expect(screen.getByTestId("mcp-server-list")).toBeTruthy();
    fireEvent.click(screen.getByTestId("mcp-server-row-kandev"));
    expect(screen.getByTestId("mcp-server-detail")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Back to servers" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Back to servers" }));
    expect(screen.getByTestId("mcp-server-list")).toBeTruthy();
  });
});
