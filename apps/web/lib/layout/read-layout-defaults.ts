import { cookies } from "next/headers";
import type { Layout } from "react-resizable-panels";

export const readLayoutDefaults = async () => {
  const cookieStore = await cookies();
  const defaults: Record<string, Layout> = {};
  const cookieList =
    typeof (cookieStore as { getAll?: () => { name: string; value: string }[] }).getAll ===
    "function"
      ? (cookieStore as { getAll: () => { name: string; value: string }[] }).getAll()
      : (() => {
          const raw = cookieStore.toString?.() ?? "";
          if (!raw) return [];
          return raw
            .split(";")
            .map((entry) => entry.trim())
            .filter(Boolean)
            .map((entry) => {
              const [name, ...rest] = entry.split("=");
              return { name, value: rest.join("=") };
            });
        })();

  for (const cookie of cookieList) {
    if (!cookie.name.startsWith("layout:")) continue;
    const id = decodeURIComponent(cookie.name.slice("layout:".length));
    try {
      const parsed = JSON.parse(decodeURIComponent(cookie.value));
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        defaults[id] = parsed as Layout;
      }
    } catch {
      // Ignore malformed layout cookies.
    }
  }

  return defaults;
};
