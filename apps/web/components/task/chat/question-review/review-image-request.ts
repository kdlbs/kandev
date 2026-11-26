import { createContext, useCallback, useState } from "react";

/** A request from the answer form to show one image on the stage. */
export type ImageRequest = { imageId: string; key: number };

/** Lets controls outside the stage (the answer form) ask it to show an image. */
export const ShowImageContext = createContext<((imageId: string) => void) | null>(null);

/** Each call makes a new request, so asking again for the same image still moves the stage. */
export function useImageRequest() {
  const [request, setRequest] = useState<ImageRequest | null>(null);
  const show = useCallback(
    (imageId: string) => setRequest((current) => ({ imageId, key: (current?.key ?? 0) + 1 })),
    [],
  );
  return { request, show };
}
