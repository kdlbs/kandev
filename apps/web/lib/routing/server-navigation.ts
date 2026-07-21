export function redirect(url: string): never {
  if (typeof window !== "undefined") {
    window.location.assign(url);
  }
  throw new Error(`Redirect to ${url}`);
}

export function notFound(): never {
  throw new Error("Route not found");
}
