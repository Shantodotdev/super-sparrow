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

/** Builds an ApiError from the server's `{error, field}` body text, falling back to the status. */
export function apiErrorFromText(status: number, text: string, request: string): ApiError {
  const fallback = `${request} failed with status ${status}`;
  try {
    const body: unknown = JSON.parse(text);
    if (body && typeof body === "object" && "error" in body && typeof body.error === "string") {
      const field = "field" in body && typeof body.field === "string" ? body.field : undefined;
      return new ApiError(status, body.error || fallback, field);
    }
  } catch {
    // The body is not JSON, such as a proxy's error page; the status is all there is.
  }
  return new ApiError(status, fallback);
}

/** Reads the server's error body from a response. */
async function errorFrom(response: Response, request: string): Promise<ApiError> {
  return apiErrorFromText(response.status, await response.text().catch(() => ""), request);
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

/** Sends a JSON mutation and accepts empty successful responses. */
export async function apiWrite<T>(
  path: string,
  method: string,
  body?: unknown,
  fetcher: typeof fetch = fetch,
): Promise<T> {
  const response = await fetcher(path, {
    method,
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  if (!response.ok) throw await errorFrom(response, `${method} ${path}`);
  return response.status === 204 ? (undefined as T) : ((await response.json()) as T);
}

/** What an upload reports while it runs and how it can be stopped. */
export type UploadOptions = {
  signal?: AbortSignal;
  /** Called with the fraction of the body sent, from 0 to 1. */
  onProgress?: (fraction: number) => void;
};

/** Where and how to send a file's bytes: a signed URL, its method, and the headers it was signed for. */
export type UploadTarget = { url: string; method: string; headers: Record<string, string> };

/**
 * Sends a file as the raw request body to a signed URL, such as one on object storage. The file
 * is handed to the browser as it is, never read into memory, and no cookies are sent.
 * XMLHttpRequest reports upload progress, which fetch does not. Aborting through the signal
 * rejects with an AbortError.
 */
export function apiPutFile(
  target: UploadTarget,
  file: Blob,
  { signal, onProgress }: UploadOptions = {},
): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("The upload was canceled", "AbortError"));
      return;
    }

    const request = new XMLHttpRequest();
    request.open(target.method, target.url);
    request.withCredentials = false;
    for (const [name, value] of Object.entries(target.headers))
      request.setRequestHeader(name, value);

    request.upload.onprogress = (event) => {
      if (event.lengthComputable && event.total > 0) onProgress?.(event.loaded / event.total);
    };
    request.onload = () => {
      if (request.status >= 200 && request.status < 300) {
        resolve();
        return;
      }
      reject(new Error(`The file upload failed with status ${request.status}`));
    };
    request.onerror = () => reject(new Error("The connection failed during the upload"));
    request.ontimeout = () => reject(new Error("The upload timed out"));
    request.onabort = () => reject(new DOMException("The upload was canceled", "AbortError"));
    signal?.addEventListener("abort", () => request.abort(), { once: true });

    request.send(file);
  });
}
