/** Select only a built-in scheme before React mounts; installed data needs authenticated inventory. */
export function bootstrapTheme() {
  let preference: string | null = null;
  try {
    preference = localStorage.getItem("theme");
  } catch {
    /* Storage can be unavailable in restricted browsers. */
  }
  const system = matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  const base = preference === "dark" || preference === "light" ? preference : system;
  document.documentElement.classList.add(base);
  document.documentElement.style.colorScheme = base;
}
