import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activateLocale } from "@/lib/i18n";
import { TechnicalDetails } from "./chat/messages/action-message-details";

const DETAILS_LABEL = "Technical details";
const COPY_LABEL = "Copy details";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("safe recovery detail disclosure", () => {
  it("copies only displayed sanitized text and announces success", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    const { container } = render(
      <TechnicalDetails>{"token=synthetic-private\nconnection refused"}</TechnicalDetails>,
    );
    expect(container.textContent).not.toContain("synthetic-private");
    fireEvent.click(screen.getByText(DETAILS_LABEL));
    fireEvent.click(screen.getByRole("button", { name: COPY_LABEL }));
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(container.querySelector("pre")!.textContent),
    );
    await waitFor(() => expect(screen.getByRole("status").textContent).toMatch(/copied/i));
  });

  it("keeps selectable safe text when clipboard access fails", async () => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
    });
    render(<TechnicalDetails>{"connection refused"}</TechnicalDetails>);
    fireEvent.click(screen.getByText(DETAILS_LABEL));
    fireEvent.click(screen.getByRole("button", { name: COPY_LABEL }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toMatch(/could not copy/i));
    expect(screen.getByText("connection refused")).toBeTruthy();
  });
});

it("updates detail controls and copy feedback when the locale changes", async () => {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
  render(<TechnicalDetails>connection refused</TechnicalDetails>);
  fireEvent.click(screen.getByText(DETAILS_LABEL));
  fireEvent.click(screen.getByRole("button", { name: COPY_LABEL }));
  await waitFor(() => expect(screen.getByRole("status").textContent).toMatch(/copied/i));
  try {
    await act(async () => {
      await activateLocale("pt-pt");
    });
    expect(screen.getByRole("button", { name: "Copiar detalhes" })).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe("Detalhes copiados");
    expect(screen.getByText("connection refused")).toBeTruthy();
  } finally {
    await act(async () => {
      await activateLocale("en");
    });
  }
});

it("copies sanitized details through the shared insecure-context fallback", async () => {
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: undefined });
  const copied: string[] = [];
  const original = Object.getOwnPropertyDescriptor(document, "execCommand");
  Object.defineProperty(document, "execCommand", {
    configurable: true,
    value: () => {
      copied.push((document.activeElement as HTMLTextAreaElement).value);
      return true;
    },
  });
  try {
    render(<TechnicalDetails>{"token=synthetic-private\nconnection refused"}</TechnicalDetails>);
    fireEvent.click(screen.getByText(DETAILS_LABEL));
    fireEvent.click(screen.getByRole("button", { name: COPY_LABEL }));
    await waitFor(() => expect(copied).toEqual(["***\nconnection refused"]));
    expect(screen.getByRole("status").textContent).toMatch(/copied/i);
  } finally {
    if (original) Object.defineProperty(document, "execCommand", original);
    else Reflect.deleteProperty(document, "execCommand");
  }
});
