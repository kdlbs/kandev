import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { Drawer, DrawerContent, DrawerNested, DrawerTitle } from "@kandev/ui/drawer";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";

// @covers AC-UI-MOBILE-TASK-NAVIGATION-001.10, AC-UI-MOBILE-TASK-NAVIGATION-001.11
const originalPointerEvents = document.body.style.pointerEvents;
afterEach(() => {
  document.body.style.pointerEvents = originalPointerEvents;
});

function Menu({ modal }: { modal?: boolean }) {
  return (
    <DropdownMenu modal={modal}>
      <DropdownMenuTrigger>Open choices</DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem>Choose destination</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

it.each(["", "auto"])(
  "restores '%s' after disposing a drawer with its implicit dropdown retained on exit",
  async (pointerStyle) => {
    document.body.style.pointerEvents = pointerStyle;
    const styles = document.createElement("style");
    styles.textContent = `[data-slot="dropdown-menu-content"] { animation-name: menu-in; }
    [data-slot="dropdown-menu-content"][data-state="closed"] { animation-name: menu-out; }`;
    document.head.append(styles);
    const view = render(
      <Drawer open>
        <DrawerContent aria-describedby={undefined}>
          <DrawerTitle>Navigation</DrawerTitle>
          <Menu />
        </DrawerContent>
      </Drawer>,
    );
    try {
      await waitFor(() => expect(document.body.style.pointerEvents).toBe("none"));
      fireEvent.keyDown(screen.getByRole("button", { name: "Open choices" }), { key: "Enter" });
      const item = await screen.findByRole("menuitem", { name: "Choose destination" });
      const menu = screen.getByRole("menu");
      fireEvent.animationStart(menu, { animationName: "menu-in" });
      fireEvent.click(item);
      expect(menu.getAttribute("data-state")).toBe("closed");
      expect(menu.isConnected).toBe(true);
      expect(document.body.style.pointerEvents).toBe("none");
      view.unmount();
      await waitFor(() => expect(document.body.style.pointerEvents).toBe(pointerStyle));
    } finally {
      view.unmount();
      styles.remove();
    }
  },
);

for (const nested of [false, true]) {
  it.each([
    { drawerModal: true, menuModal: undefined, drawerPointer: "auto" },
    { drawerModal: true, menuModal: true, drawerPointer: "none" },
    { drawerModal: true, menuModal: false, drawerPointer: "auto" },
    { drawerModal: false, menuModal: undefined, drawerPointer: "none" },
    { drawerModal: false, menuModal: true, drawerPointer: "none" },
    { drawerModal: false, menuModal: false, drawerPointer: "auto" },
  ])(
    `preserves pointer ownership with nested=${nested}, drawer=$drawerModal, menu=$menuModal`,
    async ({ drawerModal, menuModal, drawerPointer }) => {
      const Root = nested ? DrawerNested : Drawer;
      const content = (
        <Root open modal={drawerModal}>
          <DrawerContent aria-describedby={undefined} data-testid="owning-drawer">
            <DrawerTitle>Navigation</DrawerTitle>
            <Menu modal={menuModal} />
          </DrawerContent>
        </Root>
      );
      const view = render(nested ? <Drawer open>{content}</Drawer> : content);
      try {
        fireEvent.keyDown(screen.getByRole("button", { name: "Open choices" }), { key: "Enter" });
        await screen.findByRole("menuitem", { name: "Choose destination" });
        await waitFor(() =>
          expect(screen.getByTestId("owning-drawer").style.pointerEvents).toBe(drawerPointer),
        );
      } finally {
        view.unmount();
      }
    },
  );
}
