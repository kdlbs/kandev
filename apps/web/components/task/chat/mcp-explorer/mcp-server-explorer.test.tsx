import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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
const CREATE_TASK_DESCRIPTION = "Create a task";
const BACK_TO_TOOLS = "Back to tools";
const TOOLS_OBSERVED_AT = "2026-08-16T00:01:00Z";
const MCP_TOOL_DETAIL_TEST_ID = "mcp-tool-detail";

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
        "task:mcpBackToTools": BACK_TO_TOOLS,
        "task:mcpClose": "Close",
        "task:mcpConnectionDetails": "Connection details",
        "task:mcpArguments": "Arguments",
        "task:mcpRequired": "Required",
        "task:mcpOptional": "Optional",
        "task:mcpNoArguments": "No arguments",
        "task:mcpSchemaTooLarge": "Schema too large to display",
        "task:mcpJsonSchema": "JSON schema",
        "task:mcpTokenEstimate": `~${values?.count ?? 0} tokens`,
        "task:mcpTokenEstimateHelp": "Estimated with o200k_base. Actual usage varies by model.",
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
  TooltipContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
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
      showCloseButton = true,
      ...props
    }: React.HTMLAttributes<HTMLDivElement> & {
      enterConfirms?: boolean;
      showCloseButton?: boolean;
    }) => {
      const context = React.useContext(DialogContext);
      return context?.open ? (
        <div role="dialog" {...props}>
          {children}
          {showCloseButton && (
            <button type="button" aria-label="Close" onClick={() => context.onOpenChange(false)} />
          )}
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
        tools_listed_at: TOOLS_OBSERVED_AT,
        tool_token_estimator: "o200k_base:mcp-tool-json-v1",
        tools: [
          {
            name: "create_task_kandev",
            description: CREATE_TASK_DESCRIPTION,
            estimated_tokens: 42,
            input_schema: {
              type: "object",
              properties: {
                title: { type: "string", description: "Short task title" },
              },
              required: ["title"],
            },
          },
        ],
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
  it("shows the rich status tooltip and one desktop close control", () => {
    render(
      <McpIndicator mcpServers={["kandev", "filesystem"]} attachmentHistory={attachmentHistory} />,
    );

    const tooltip = screen.getByTestId("mcp-status-popover");
    expect(within(tooltip).getByText("kandev")).toBeTruthy();
    expect(within(tooltip).getByText("Active")).toBeTruthy();
    expect(within(tooltip).getByText("Connection delivered")).toBeTruthy();

    fireEvent.click(screen.getByTestId(MCP_STATUS_TRIGGER_TEST_ID));
    const dialog = screen.getByRole("dialog");
    expect(dialog.getAttribute("data-testid")).toBe("mcp-server-explorer");
    expect(within(dialog).getAllByRole("button", { name: "Close" })).toHaveLength(1);
  });

  it("opens a tool page and returns focus to its list row", async () => {
    render(
      <McpIndicator mcpServers={["kandev", "filesystem"]} attachmentHistory={attachmentHistory} />,
    );
    fireEvent.click(screen.getByTestId(MCP_STATUS_TRIGGER_TEST_ID));
    expect(screen.getByText("create_task_kandev")).toBeTruthy();
    expect(screen.getByText("~42 tokens")).toBeTruthy();
    expect(screen.queryByText(CREATE_TASK_DESCRIPTION)).toBeNull();

    screen.getByTestId("mcp-tool-list-scroll").scrollTop = 173;
    fireEvent.click(screen.getByRole("button", { name: /create_task_kandev/ }));
    expect(screen.getByText(CREATE_TASK_DESCRIPTION)).toBeTruthy();
    expect(screen.getByText("Short task title")).toBeTruthy();
    expect(screen.getByText("Required")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: BACK_TO_TOOLS }));
    const toolRow = screen.getByRole("button", { name: /create_task_kandev/ });
    await waitFor(() => expect(document.activeElement).toBe(toolRow));
    expect(screen.getByTestId("mcp-tool-list-scroll").scrollTop).toBe(173);

    fireEvent.click(screen.getByTestId("mcp-server-row-filesystem"));
    expect(screen.getByText(testCopy.thirdPartyCatalogUnavailable)).toBeTruthy();
    expect(screen.queryByText(CREATE_TASK_DESCRIPTION)).toBeNull();

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

  it("returns to the fallback server tools when the selected server disappears", () => {
    const { rerender } = render(
      <McpIndicator mcpServers={["kandev", "filesystem"]} attachmentHistory={attachmentHistory} />,
    );
    fireEvent.click(screen.getByTestId(MCP_STATUS_TRIGGER_TEST_ID));
    fireEvent.click(screen.getByTestId("mcp-server-row-filesystem"));
    expect(screen.getByText(testCopy.thirdPartyCatalogUnavailable)).toBeTruthy();

    const kandev = attachmentHistory.current.servers?.[0];
    if (!kandev) throw new Error("missing test server");
    rerender(
      <McpIndicator mcpServers={["kandev"]} attachmentHistory={historyForServers([kandev])} />,
    );
    expect(screen.getByTestId("mcp-tool-row-create_task_kandev")).toBeTruthy();
    expect(screen.queryByTestId(MCP_TOOL_DETAIL_TEST_ID)).toBeNull();
  });
});

describe("MCP explorer catalog states and touch navigation", () => {
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
        tools_listed_at: TOOLS_OBSERVED_AT,
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
        tools_listed_at: TOOLS_OBSERVED_AT,
        tools: [{ name: "create_task_kandev", description: CREATE_TASK_DESCRIPTION }],
        tool_count: 129,
        tool_catalog_truncated: true,
      },
    ]);
    expect(screen.getByText("Showing 1")).toBeTruthy();
    expect(screen.getByText("Only the first tools are shown.")).toBeTruthy();
  });

  it("distinguishes tools without arguments from schemas removed by limits", () => {
    renderOpenExplorer([
      {
        name: "kandev",
        source: "kandev",
        status: "active",
        tools_listed_at: TOOLS_OBSERVED_AT,
        tools: [{ name: "no_arguments" }, { name: "large_schema", input_schema_truncated: true }],
      },
    ]);
    fireEvent.click(screen.getByRole("button", { name: /no_arguments/ }));
    expect(screen.getByText("No arguments")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: BACK_TO_TOOLS }));
    fireEvent.click(screen.getByRole("button", { name: /large_schema/ }));
    expect(screen.getByText("Schema too large to display")).toBeTruthy();
  });

  it("uses a phone server-to-tools-to-tool flow with two Back actions", () => {
    responsiveMock.breakpoint = "mobile";
    render(
      <McpIndicator mcpServers={["kandev", "filesystem"]} attachmentHistory={attachmentHistory} />,
    );

    fireEvent.click(screen.getByTestId(MCP_STATUS_TRIGGER_TEST_ID));
    expect(screen.getByTestId("mcp-server-list")).toBeTruthy();
    fireEvent.click(screen.getByTestId("mcp-server-row-kandev"));
    expect(screen.getByTestId("mcp-tool-list")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Back to servers" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /create_task_kandev/ }));
    expect(screen.getByTestId(MCP_TOOL_DETAIL_TEST_ID)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: BACK_TO_TOOLS }));
    expect(screen.getByTestId("mcp-tool-list")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Back to servers" }));
    expect(screen.getByTestId("mcp-server-list")).toBeTruthy();
  });

  it("returns to the tools page when live evidence removes the selected tool", () => {
    const { rerender } = render(
      <McpIndicator mcpServers={["kandev"]} attachmentHistory={attachmentHistory} />,
    );
    fireEvent.click(screen.getByTestId(MCP_STATUS_TRIGGER_TEST_ID));
    fireEvent.click(screen.getByRole("button", { name: /create_task_kandev/ }));
    expect(screen.getByTestId(MCP_TOOL_DETAIL_TEST_ID)).toBeTruthy();

    const kandev = attachmentHistory.current.servers?.[0];
    if (!kandev) throw new Error("missing test server");
    rerender(
      <McpIndicator
        mcpServers={["kandev"]}
        attachmentHistory={historyForServers([{ ...kandev, tools: [], tool_count: 0 }])}
      />,
    );
    expect(screen.queryByTestId(MCP_TOOL_DETAIL_TEST_ID)).toBeNull();
    expect(screen.getByTestId("mcp-tool-list")).toBeTruthy();
  });
});
