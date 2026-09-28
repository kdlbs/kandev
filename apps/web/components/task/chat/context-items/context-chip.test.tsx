import React from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const touchState = vi.hoisted(() => ({ enabled: true }));
const pointerState = vi.hoisted(() => ({ isFinePointer: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchState.enabled,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => pointerState,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => (key === "common:open" ? "Open" : key) }),
}));
vi.mock("@tabler/icons-react", () => ({
  IconListCheck: () => <svg aria-hidden="true" />,
  IconFile: () => <svg aria-hidden="true" />,
  IconMessageDots: () => <svg aria-hidden="true" />,
  IconPhoto: () => <svg aria-hidden="true" />,
  IconAt: () => <svg aria-hidden="true" />,
  IconGitPullRequest: () => <svg aria-hidden="true" />,
  IconRoute: () => <svg aria-hidden="true" />,
  IconX: () => <svg aria-hidden="true" />,
  IconPinFilled: () => <svg aria-hidden="true" />,
}));
vi.mock("@kandev/ui/drawer", () => {
  const DrawerContext = React.createContext({
    open: false,
    onOpenChange: (_value: boolean) => {},
  });
  return {
    Drawer: ({
      open,
      onOpenChange,
      children,
    }: {
      open: boolean;
      onOpenChange: (value: boolean) => void;
      children: React.ReactNode;
    }) => (
      <DrawerContext.Provider value={{ open, onOpenChange }}>{children}</DrawerContext.Provider>
    ),
    DrawerTrigger: ({
      children,
    }: {
      children: React.ReactElement<{ onClick?: (event: React.MouseEvent) => void }>;
    }) => {
      const { onOpenChange } = React.useContext(DrawerContext);
      return React.cloneElement(children, {
        onClick: (event: React.MouseEvent) => {
          children.props.onClick?.(event);
          onOpenChange(true);
        },
      });
    },
    DrawerContent: ({ children }: { children: React.ReactNode }) => {
      const { open } = React.useContext(DrawerContext);
      return open ? <div>{children}</div> : null;
    },
    DrawerHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    DrawerTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
    DrawerDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
  };
});

import { ContextChip } from "./context-chip";

beforeEach(() => {
  touchState.enabled = true;
  pointerState.isFinePointer = false;
});

afterEach(cleanup);

describe("ContextChip coarse-pointer actions", () => {
  it("opens the preview drawer and exposes the open action", () => {
    const onClick = vi.fn();
    render(
      <ContextChip kind="file" label="app.ts" preview={<div>Preview</div>} onClick={onClick} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "app.ts" }));
    expect(screen.getByText("Preview")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Open" }));

    expect(onClick).toHaveBeenCalledOnce();
  });

  it("opens a source-independent collection without a navigation action", () => {
    render(
      <ContextChip
        kind="preview-feedback"
        label="3 preview feedback items"
        preview={<div>Shared feedback</div>}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "3 preview feedback items" }));

    expect(screen.getByText("Shared feedback")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Open" })).toBeNull();
  });
});

describe("ContextChip fine-pointer previews", () => {
  it("opens an actionable collection on click when no navigation action exists", () => {
    touchState.enabled = false;
    pointerState.isFinePointer = true;
    render(
      <ContextChip
        kind="preview-feedback"
        label="3 preview feedback items"
        preview={<div>Shared feedback</div>}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "3 preview feedback items" }));

    expect(screen.getByText("Shared feedback")).toBeTruthy();
  });
});
