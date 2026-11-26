import { act, renderHook } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { useThemeImport } from "./use-theme-import";
import { prepareThemeFile, type ThemeReview } from "@/lib/api/domains/themes-api";
vi.mock("@/lib/api/domains/themes-api", () => ({
  prepareThemeFile: vi.fn(),
  prepareThemeURL: vi.fn(),
  commitThemeReview: vi.fn(),
}));
it("discards preparation after reset", async () => {
  let resolve!: (review: ThemeReview) => void;
  vi.mocked(prepareThemeFile).mockReturnValue(
    new Promise((r) => {
      resolve = r;
    }),
  );
  const { result } = renderHook(() => useThemeImport());
  let pending!: Promise<void>;
  act(() => {
    pending = result.current.prepare(new File(["{}"], "theme.json"));
  });
  act(() => result.current.reset());
  await act(async () => {
    resolve({ id: "late" } as ThemeReview);
    await pending;
  });
  expect(result.current.review).toBeNull();
  expect(result.current.busy).toBe(false);
});
