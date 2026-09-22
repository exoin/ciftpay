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
  // Respect NEXT_PUBLIC_API_URL first, then fallback to NEXT_PUBLIC_API_BASE_URL
  const configured =
    process.env.NEXT_PUBLIC_API_URL ||
    process.env.NEXT_PUBLIC_API_BASE_URL;

  if (typeof window === "undefined") {
    return process.env.API_BASE_URL || configured || "http://localhost:8080";
  }

  // If NEXT_PUBLIC_API_URL is explicitly set to a remote/specified host, use it directly
  if (configured && !configured.includes("localhost") && !configured.includes("127.0.0.1")) {
    return configured;
  }

  // If accessed directly on localhost, use configured or localhost:8080
  if (window.location.hostname === "localhost" || window.location.hostname === "127.0.0.1") {
    return configured || "http://localhost:8080";
  }

  // When accessing via a mobile device or local network IP (e.g. http://192.168.x.x:3000),
  // dynamically talk to the same host on port 8080 so the phone does not query its own loopback
  const protocol = window.location.protocol === "https:" ? "https:" : "http:";
  return `${protocol}//${window.location.hostname}:8080`;
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

let csrfRefreshPromise: Promise<string | null> | null = null;

/**
 * Fetches the active session's CSRF token from the Go backend.
 * Deduplicates in-flight calls to avoid stampeding the API.
 */
export async function fetchCsrfToken(): Promise<string | null> {
  if (typeof window === "undefined") return null;
  if (csrfRefreshPromise) return csrfRefreshPromise;

  csrfRefreshPromise = (async () => {
    try {
      const res = await fetch(`${apiBaseUrl()}/csrf`, {
        method: "GET",
        credentials: "include",
        headers: { Accept: "application/json" },
      });
      if (res.ok) {
        const body = (await res.json()) as { csrf_token?: string };
        if (body.csrf_token) {
          setCsrfToken(body.csrf_token);
          return body.csrf_token;
        }
      }
      return null;
    } catch {
      return null;
    } finally {
      csrfRefreshPromise = null;
    }
  })();

  return csrfRefreshPromise;
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

/**
 * Resilient fetch wrapper:
 * 1. Clones mutating requests before dispatch so body streams remain reusable.
 * 2. If a mutating request fails with 403 CSRF error, refreshes the CSRF token and seamlessly retries once.
 */
export async function resilientFetch(request: Request): Promise<Response> {
  const isMutating = request.method !== "GET" && request.method !== "HEAD";
  const retryCandidate = isMutating ? request.clone() : null;

  const response = await fetch(request);

  if (response.status === 403 && isMutating && retryCandidate) {
    const cloned = response.clone();
    let isCsrf = false;
    try {
      const data = await cloned.json();
      const msg = data?.error?.message ?? "";
      if (typeof msg === "string" && msg.toLowerCase().includes("csrf")) {
        isCsrf = true;
      }
    } catch {
      // not JSON
    }

    if (isCsrf) {
      const newToken = await fetchCsrfToken();
      if (newToken) {
        const headers = new Headers(retryCandidate.headers);
        headers.set("X-CSRF-Token", newToken);
        const retryRequest = new Request(retryCandidate, {
          headers,
          ...(typeof window === "undefined" ? { duplex: "half" } : {}),
        });
        return await fetch(retryRequest);
      }
    }
  }

  return response;
}

/** Adds the session cookie, CSRF token on mutations (auto-retrieves if missing) and the active org header. */
const authMiddleware: Middleware = {
  async onRequest({ request }) {
    if (request.method !== "GET" && request.method !== "HEAD") {
      let csrf = getCsrfToken();
      if (!csrf) {
        csrf = await fetchCsrfToken();
      }
      if (csrf) request.headers.set("X-CSRF-Token", csrf);
    }
    const org = getActiveOrgId();
    if (org && !request.headers.has("X-Org-Id")) request.headers.set("X-Org-Id", org);
    return request;
  },
};

export const api = createClient<paths>({
  baseUrl: apiBaseUrl(),
  credentials: "include",
  fetch: resilientFetch,
});
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
export async function rawPatch<T>(path: string, body?: unknown): Promise<T> {
  const headers = new Headers({ "Content-Type": "application/json" });
  let csrf = getCsrfToken();
  if (!csrf) {
    csrf = await fetchCsrfToken();
  }
  if (csrf) headers.set("X-CSRF-Token", csrf);
  const org = getActiveOrgId();
  if (org) headers.set("X-Org-Id", org);
  const req = new Request(`${apiBaseUrl()}${path}`, {
    method: "PATCH",
    credentials: "include",
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const res = await resilientFetch(req);
  return readJson<T>(res);
}

export async function rawPost<T>(path: string, body?: unknown): Promise<T> {
  const headers = new Headers({ "Content-Type": "application/json" });
  let csrf = getCsrfToken();
  if (!csrf) {
    csrf = await fetchCsrfToken();
  }
  if (csrf) headers.set("X-CSRF-Token", csrf);
  const org = getActiveOrgId();
  if (org) headers.set("X-Org-Id", org);
  const req = new Request(`${apiBaseUrl()}${path}`, {
    method: "POST",
    credentials: "include",
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const res = await resilientFetch(req);
  return readJson<T>(res);
}

/**
 * Multipart POST (file uploads). openapi-fetch JSON-encodes bodies, so this
 * mirrors its auth behaviour by hand: cookie, CSRF token and active org header.
 * Content-Type is left unset so the browser adds the multipart boundary.
 */
export async function postForm<T>(path: string, form: FormData): Promise<T> {
  const headers = new Headers();
  let csrf = getCsrfToken();
  if (!csrf) {
    csrf = await fetchCsrfToken();
  }
  if (csrf) headers.set("X-CSRF-Token", csrf);
  const org = getActiveOrgId();
  if (org) headers.set("X-Org-Id", org);
  const req = new Request(`${apiBaseUrl()}${path}`, {
    method: "POST",
    credentials: "include",
    headers,
    body: form,
  });
  const res = await resilientFetch(req);
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
