import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const touchState = vi.hoisted(() => ({ enabled: true }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchState.enabled,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: false }),
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
});
