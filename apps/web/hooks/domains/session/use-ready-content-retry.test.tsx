import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TurnFileChange } from "@/lib/types/turn-changes";
import { useReadyContentRetry } from "./use-ready-content-retry";

const notStoredError = new Error("not stored");

function file(
  availability: TurnFileChange["content_availability"],
  synthetic = false,
): TurnFileChange {
  return {
    id: "file-1",
    repository_change_id: "repo-1",
    checkout_id: "checkout-1",
    path: "src/main.ts",
    kind: "modified",
    content_availability: availability,
    synthetic,
  };
}

type RetryProps = {
  selected: TurnFileChange;
  whitespace: boolean;
  error: unknown;
  loading: boolean;
  repositorySettled: boolean;
};

function useRetry(props: RetryProps, retry: () => void) {
  return useReadyContentRetry({
    selectedFile: props.selected,
    contentError: props.error,
    contentLoading: props.loading,
    whitespace: props.whitespace,
    repositorySettled: props.repositorySettled,
    retry,
  });
}

describe("useReadyContentRetry", () => {
  it("retries a synthetic pending placeholder once its repository settles", () => {
    const retry = vi.fn();
    const { rerender } = renderHook((props: RetryProps) => useRetry(props, retry), {
      initialProps: {
        selected: file("pending", true),
        whitespace: false,
        error: notStoredError,
        loading: false,
        repositorySettled: false,
      },
    });
    expect(retry).not.toHaveBeenCalled();

    rerender({
      selected: file("pending", true),
      whitespace: false,
      error: notStoredError,
      loading: false,
      repositorySettled: true,
    });
    expect(retry).toHaveBeenCalledTimes(1);
    rerender({
      selected: file("pending", true),
      whitespace: false,
      error: notStoredError,
      loading: false,
      repositorySettled: true,
    });
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it("keeps the readiness retry for a canonical ready file", () => {
    const retry = vi.fn();
    const { rerender } = renderHook((props: RetryProps) => useRetry(props, retry), {
      initialProps: {
        selected: file("ready"),
        whitespace: false,
        error: notStoredError,
        loading: false,
        repositorySettled: false,
      },
    });
    expect(retry).toHaveBeenCalledTimes(1);
    rerender({
      selected: file("ready"),
      whitespace: false,
      error: notStoredError,
      loading: false,
      repositorySettled: false,
    });
    expect(retry).toHaveBeenCalledTimes(1);
    rerender({
      selected: file("ready"),
      whitespace: true,
      error: notStoredError,
      loading: false,
      repositorySettled: false,
    });
    expect(retry).toHaveBeenCalledTimes(2);
  });

  it("ignores loading, unavailable, and error-free states", () => {
    const retry = vi.fn();
    const { rerender } = renderHook((props: RetryProps) => useRetry(props, retry), {
      initialProps: {
        selected: file("ready"),
        whitespace: false,
        error: new Error("loading") as unknown,
        loading: true,
        repositorySettled: true,
      },
    });
    expect(retry).not.toHaveBeenCalled();
    rerender({
      selected: file("ready"),
      whitespace: false,
      error: null,
      loading: false,
      repositorySettled: true,
    });
    rerender({
      selected: file("unavailable"),
      whitespace: false,
      error: new Error("gone"),
      loading: false,
      repositorySettled: true,
    });
    expect(retry).not.toHaveBeenCalled();
  });
});
