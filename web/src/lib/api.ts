/**
 * An API response with a status outside 2xx. Its message is the server's error message, which
 * says what went wrong in words a person can act on, and field names the input at fault when the
 * server reports one.
 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly field?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** Reads the server's `{error, field}` body, falling back to the status when there is none. */
async function errorFrom(response: Response, request: string): Promise<ApiError> {
  const fallback = `${request} failed with status ${response.status}`;
  try {
    const body: unknown = await response.json();
    if (body && typeof body === "object" && "error" in body && typeof body.error === "string") {
      const field = "field" in body && typeof body.field === "string" ? body.field : undefined;
      return new ApiError(response.status, body.error || fallback, field);
    }
  } catch {
    // The body is not JSON, such as a proxy's error page; the status is all there is.
  }
  return new ApiError(response.status, fallback);
}

/**
 * Fetches `path` from the Go API on the same origin and returns its JSON body.
 * Sends the session cookie and throws an `ApiError` for any non-2xx response.
 */
export async function apiGet<T>(path: string, fetcher: typeof fetch = fetch): Promise<T> {
  const response = await fetcher(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });

  if (!response.ok) throw await errorFrom(response, `GET ${path}`);

  return (await response.json()) as T;
}
