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

/**
 * Sends a file as the raw request body and returns the JSON response. The file is handed to the
 * browser as it is, never read into memory. XMLHttpRequest reports upload progress, which fetch
 * does not. Aborting through the signal rejects with an AbortError.
 */
export function apiUploadFile<T>(
  path: string,
  file: Blob,
  { signal, onProgress }: UploadOptions = {},
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("The upload was canceled", "AbortError"));
      return;
    }

    const request = new XMLHttpRequest();
    const description = `POST ${path.split("?")[0]}`;
    request.open("POST", path);
    request.responseType = "text";
    request.setRequestHeader("Accept", "application/json");
    request.setRequestHeader("Content-Type", "application/octet-stream");

    request.upload.onprogress = (event) => {
      if (event.lengthComputable && event.total > 0) onProgress?.(event.loaded / event.total);
    };
    request.onload = () => {
      if (request.status >= 200 && request.status < 300) {
        try {
          resolve(JSON.parse(request.responseText) as T);
        } catch {
          reject(new Error("The server sent an unreadable response"));
        }
        return;
      }
      reject(apiErrorFromText(request.status, request.responseText, description));
    };
    request.onerror = () => reject(new Error("The connection failed during the upload"));
    request.ontimeout = () => reject(new Error("The upload timed out"));
    request.onabort = () => reject(new DOMException("The upload was canceled", "AbortError"));
    signal?.addEventListener("abort", () => request.abort(), { once: true });

    request.send(file);
  });
}
