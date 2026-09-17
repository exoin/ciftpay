import createClient, { type Middleware } from "openapi-fetch";
import type { components, paths } from "./schema";

export type Schemas = components["schemas"];
export type ApiError = Schemas["Error"]["error"];

/**
 * The Go API serves the OpenAPI paths at its root (no `/v1` prefix in Phase 0).
 * In the browser we hit the published port; on the server we use the compose
 * service name.
 */
export function apiBaseUrl(): string {
  if (typeof window === "undefined") {
    return process.env.API_BASE_URL ?? process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
  }
  return process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
}

const CSRF_STORAGE_KEY = "ciftpay.csrf";

export function setCsrfToken(token: string | null) {
  if (typeof window === "undefined") return;
  if (token) window.sessionStorage.setItem(CSRF_STORAGE_KEY, token);
  else window.sessionStorage.removeItem(CSRF_STORAGE_KEY);
}

export function getCsrfToken(): string | null {
  if (typeof window === "undefined") return null;
  return window.sessionStorage.getItem(CSRF_STORAGE_KEY);
}

const ORG_STORAGE_KEY = "ciftpay.org";

export function setActiveOrgId(id: string | null) {
  if (typeof window === "undefined") return;
  if (id) window.localStorage.setItem(ORG_STORAGE_KEY, id);
  else window.localStorage.removeItem(ORG_STORAGE_KEY);
}

export function getActiveOrgId(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(ORG_STORAGE_KEY);
}

/** Adds the session cookie, CSRF token on mutations and the active org header. */
const authMiddleware: Middleware = {
  async onRequest({ request }) {
    if (request.method !== "GET" && request.method !== "HEAD") {
      const csrf = getCsrfToken();
      if (csrf) request.headers.set("X-CSRF-Token", csrf);
    }
    const org = getActiveOrgId();
    if (org && !request.headers.has("X-Org-Id")) request.headers.set("X-Org-Id", org);
    return request;
  },
};

export const api = createClient<paths>({ baseUrl: apiBaseUrl(), credentials: "include" });
api.use(authMiddleware);

/** Server-side client (server components, route handlers). No cookies. */
export function serverApi() {
  return createClient<paths>({ baseUrl: apiBaseUrl(), cache: "no-store" });
}

/**
 * Untyped GET for endpoints the Go API serves but the OpenAPI contract does not
 * yet describe (logged in docs/runbooks/local-dev.md as contract debt).
 */
export async function rawGet<T>(path: string): Promise<T> {
  const headers = new Headers();
  const org = getActiveOrgId();
  if (org) headers.set("X-Org-Id", org);
  const res = await fetch(`${apiBaseUrl()}${path}`, { credentials: "include", headers });
  return readJson<T>(res);
}

/**
 * Untyped JSON POST for endpoints the Go API serves but the OpenAPI contract
 * does not yet describe (same contract debt as rawGet; see
 * docs/runbooks/local-dev.md).
 */
export async function rawPost<T>(path: string, body?: unknown): Promise<T> {
  const headers = new Headers({ "Content-Type": "application/json" });
  const csrf = getCsrfToken();
  if (csrf) headers.set("X-CSRF-Token", csrf);
  const org = getActiveOrgId();
  if (org) headers.set("X-Org-Id", org);
  const res = await fetch(`${apiBaseUrl()}${path}`, { method: "POST", credentials: "include", headers, body: body === undefined ? undefined : JSON.stringify(body) });
  return readJson<T>(res);
}

/**
 * Multipart POST (file uploads). openapi-fetch JSON-encodes bodies, so this
 * mirrors its auth behaviour by hand: cookie, CSRF token and active org header.
 * Content-Type is left unset so the browser adds the multipart boundary.
 */
export async function postForm<T>(path: string, form: FormData): Promise<T> {
  const headers = new Headers();
  const csrf = getCsrfToken();
  if (csrf) headers.set("X-CSRF-Token", csrf);
  const org = getActiveOrgId();
  if (org) headers.set("X-Org-Id", org);
  const res = await fetch(`${apiBaseUrl()}${path}`, { method: "POST", credentials: "include", headers, body: form });
  return readJson<T>(res);
}

async function readJson<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let body: Schemas["Error"] | undefined;
    try {
      body = (await res.json()) as Schemas["Error"];
    } catch {
      /* not json */
    }
    throw new ApiRequestError(res.status, body?.error);
  }
  return (await res.json()) as T;
}

export class ApiRequestError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, unknown>;
  constructor(status: number, err?: ApiError) {
    super(err?.message ?? `Request failed (${status})`);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = err?.code ?? (status === 401 ? "unauthenticated" : "internal");
    this.details = err?.details;
  }
}

/** Unwraps an openapi-fetch result into data or throws ApiRequestError. */
export function unwrap<T>(res: { data?: T; error?: unknown; response: Response }): T {
  if (res.error !== undefined || !res.response.ok) {
    const body = res.error as Schemas["Error"] | undefined;
    throw new ApiRequestError(res.response.status, body?.error);
  }
  return res.data as T;
}
