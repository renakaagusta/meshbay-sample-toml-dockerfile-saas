export type ItemStatus = "queued" | "processed";

export type Item = {
  id: number;
  name: string;
  status: ItemStatus;
  created_at: string;
  processed_at: string | null;
};

export type Stats = {
  queued: number;
  processed: number;
  total: number;
};

export type ApiHealth = {
  status: string;
  version: string;
  database: string;
  redis: string;
};

/**
 * API_URL is a runtime value: it is read on the server per request, never
 * inlined at build.
 *
 * Meshbay names a manifest service `<project>-<service>` on the private
 * network, and it injects the project slug as MESHBAY_PROJECT, so the api is
 * reachable at `http://<project>-api.internal:8080` without the project slug
 * being hard-coded here or in meshbay.toml.
 */
export function apiBaseUrl(): string {
  // Bracket access keeps these honest runtime lookups: Next must not fold a
  // build-time value into the server bundle.
  const configured = (process.env["API_URL"] ?? "").trim();
  if (configured !== "") {
    return configured.replace(/\/+$/, "");
  }
  const project = (process.env["MESHBAY_PROJECT"] ?? "").trim();
  if (project !== "") {
    return `http://${project}-api.internal:8080`;
  }
  return "http://localhost:8080";
}

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function readError(response: Response): Promise<string> {
  const text = await response.text();
  try {
    const payload = JSON.parse(text) as { error?: string };
    return payload.error ?? text;
  } catch {
    return text || `HTTP ${response.status}`;
  }
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    cache: "no-store",
    headers: { "content-type": "application/json", ...(init?.headers ?? {}) },
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) {
    throw new ApiError(response.status, await readError(response));
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export function listItems(): Promise<Item[]> {
  return call<Item[]>("/api/items");
}

export function getItem(itemID: string | number): Promise<Item> {
  return call<Item>(`/api/items/${itemID}`);
}

export function createItem(name: string): Promise<Item> {
  return call<Item>("/api/items", { method: "POST", body: JSON.stringify({ name }) });
}

export function updateItem(itemID: string | number, name: string): Promise<Item> {
  return call<Item>(`/api/items/${itemID}`, { method: "PUT", body: JSON.stringify({ name }) });
}

export function deleteItem(itemID: string | number): Promise<void> {
  return call<void>(`/api/items/${itemID}`, { method: "DELETE" });
}

export function loadStats(): Promise<Stats> {
  return call<Stats>("/api/stats");
}

export function readHealth(): Promise<ApiHealth> {
  return call<ApiHealth>("/health");
}

/**
 * Forwards a browser request to the api as-is (status and body), so the form
 * never talks to the api directly and API_URL stays on the server.
 */
export async function forward(path: string, init?: RequestInit): Promise<Response> {
  let upstream: Response;
  try {
    upstream = await fetch(`${apiBaseUrl()}${path}`, {
      ...init,
      cache: "no-store",
      headers: { "content-type": "application/json", ...(init?.headers ?? {}) },
      signal: AbortSignal.timeout(15000),
    });
  } catch (error) {
    return Response.json(
      { error: `api unreachable at ${apiBaseUrl()}: ${error instanceof Error ? error.message : "unknown"}` },
      { status: 502 },
    );
  }
  const body = await upstream.text();
  return new Response(body, {
    status: upstream.status,
    headers: { "content-type": upstream.headers.get("content-type") ?? "application/json" },
  });
}
