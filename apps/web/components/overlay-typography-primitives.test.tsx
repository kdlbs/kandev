import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { AlertDialog, AlertDialogDescription, AlertDialogTitle } from "@kandev/ui/alert-dialog";
import { Dialog, DialogDescription, DialogTitle } from "@kandev/ui/dialog";
import { Drawer, DrawerDescription, DrawerTitle } from "@kandev/ui/drawer";
import { Sheet, SheetDescription, SheetTitle } from "@kandev/ui/sheet";

afterEach(() => cleanup());

const primitiveFamilies = ["alert", "alert-dialog", "dialog", "drawer", "sheet"] as const;
const BALANCED_TEXT_CLASS = "text-balance";
const PRETTY_TEXT_CLASS = "text-pretty";
const WORD_CONTAINMENT_CLASS = "wrap-anywhere";
const ZERO_MIN_WIDTH_CLASS = "min-w-0";

function verifyConsumerWrappingOverrides() {
  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.3, AC-UI-SURFACE-TEXT-HIERARCHY-001.4
  render(
    <div>
      <section>
        <AlertTitle data-testid="alert-override-title" className="text-pretty" />
        <AlertDescription data-testid="alert-override-description" className="text-balance" />
      </section>
      <AlertDialog>
        <section>
          <AlertDialogTitle data-testid="alert-dialog-override-title" className="text-pretty" />
          <AlertDialogDescription
            data-testid="alert-dialog-override-description"
            className="text-balance"
          />
        </section>
      </AlertDialog>
      <Dialog>
        <section>
          <DialogTitle data-testid="dialog-override-title" className="text-pretty" />
          <DialogDescription data-testid="dialog-override-description" className="text-balance" />
        </section>
      </Dialog>
      <Drawer>
        <section>
          <DrawerTitle data-testid="drawer-override-title" className="text-pretty" />
          <DrawerDescription data-testid="drawer-override-description" className="text-balance" />
        </section>
      </Drawer>
      <Sheet>
        <section>
          <SheetTitle data-testid="sheet-override-title" className="text-pretty" />
          <SheetDescription data-testid="sheet-override-description" className="text-balance" />
        </section>
      </Sheet>
    </div>,
  );

  for (const name of primitiveFamilies) {
    const title = screen.getByTestId(`${name}-override-title`);
    const description = screen.getByTestId(`${name}-override-description`);
    expect(title.classList.contains(PRETTY_TEXT_CLASS)).toBe(true);
    expect(title.classList.contains(BALANCED_TEXT_CLASS)).toBe(false);
    expect(title.classList.contains(WORD_CONTAINMENT_CLASS)).toBe(true);
    expect(description.classList.contains(BALANCED_TEXT_CLASS)).toBe(true);
    expect(description.classList.contains(PRETTY_TEXT_CLASS)).toBe(false);
    expect(description.classList.contains(WORD_CONTAINMENT_CLASS)).toBe(true);
  }
}

describe("surface typography primitives", () => {
  it("uses semantic wrapping defaults for every shared surface family", () => {
    // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.1, AC-UI-SURFACE-TEXT-HIERARCHY-001.3
    render(
      <div>
        <section data-testid="alert-surface">
          <AlertTitle data-testid="alert-title">A title with a long value</AlertTitle>
          <AlertDescription data-testid="alert-description">
            A description with a long unbroken value for containment.
          </AlertDescription>
        </section>
        <AlertDialog>
          <section data-testid="alert-dialog-surface">
            <AlertDialogTitle data-testid="alert-dialog-title">
              A title with a long value
            </AlertDialogTitle>
            <AlertDialogDescription data-testid="alert-dialog-description">
              A description with a long unbroken value for containment.
            </AlertDialogDescription>
          </section>
        </AlertDialog>
        <Dialog>
          <section data-testid="dialog-surface">
            <DialogTitle data-testid="dialog-title">A title with a long value</DialogTitle>
            <DialogDescription data-testid="dialog-description">
              A description with a long unbroken value for containment.
            </DialogDescription>
          </section>
        </Dialog>
        <Drawer>
          <section data-testid="drawer-surface">
            <DrawerTitle data-testid="drawer-title">A title with a long value</DrawerTitle>
            <DrawerDescription data-testid="drawer-description">
              A description with a long unbroken value for containment.
            </DrawerDescription>
          </section>
        </Drawer>
        <Sheet>
          <section data-testid="sheet-surface">
            <SheetTitle data-testid="sheet-title">A title with a long value</SheetTitle>
            <SheetDescription data-testid="sheet-description">
              A description with a long unbroken value for containment.
            </SheetDescription>
          </section>
        </Sheet>
      </div>,
    );

    for (const name of primitiveFamilies) {
      const title = screen.getByTestId(`${name}-title`);
      const description = screen.getByTestId(`${name}-description`);
      expect(title.classList.contains(BALANCED_TEXT_CLASS)).toBe(true);
      expect(title.classList.contains(ZERO_MIN_WIDTH_CLASS)).toBe(true);
      expect(title.classList.contains(WORD_CONTAINMENT_CLASS)).toBe(true);
      expect(description.classList.contains(PRETTY_TEXT_CLASS)).toBe(true);
      expect(description.classList.contains(ZERO_MIN_WIDTH_CLASS)).toBe(true);
      expect(description.classList.contains(WORD_CONTAINMENT_CLASS)).toBe(true);
      expect(description.classList.contains(BALANCED_TEXT_CLASS)).toBe(false);
      expect(description.classList.contains("md:text-pretty")).toBe(false);
    }
  });

  it("lets consumers override the shared wrapping default", () => {
    verifyConsumerWrappingOverrides();
  });
});
