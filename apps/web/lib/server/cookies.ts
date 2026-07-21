type CookieValue = {
  name: string;
  value: string;
};

export type CookieStore = {
  get(name: string): CookieValue | undefined;
  getAll(): CookieValue[];
  toString(): string;
};

function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function parseCookieHeader(raw: string): CookieValue[] {
  if (!raw) return [];
  return raw
    .split(";")
    .map((entry) => entry.trim())
    .filter(Boolean)
    .map((entry) => {
      const [name, ...rest] = entry.split("=");
      return {
        name: safeDecode(name),
        value: safeDecode(rest.join("=")),
      };
    });
}

export async function readCookies(): Promise<CookieStore> {
  const raw = typeof document === "undefined" ? "" : document.cookie;
  const cookies = parseCookieHeader(raw);
  return {
    get(name: string) {
      return cookies.find((cookie) => cookie.name === name);
    },
    getAll() {
      return cookies;
    },
    toString() {
      return raw;
    },
  };
}
