/** An API response with a status outside 2xx. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
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

  if (!response.ok) {
    throw new ApiError(response.status, `GET ${path} failed with status ${response.status}`);
  }

  return (await response.json()) as T;
}
