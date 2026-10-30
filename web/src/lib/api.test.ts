import { describe, expect, test } from "bun:test";
import { ApiError, apiGet } from "./api";

function respondWith(status: number, body: unknown): typeof fetch {
  return (async () => Response.json(body, { status })) as unknown as typeof fetch;
}

describe("apiGet", () => {
  test("returns the JSON body of a successful response", async () => {
    const body = await apiGet<{ status: string }>(
      "/api/health",
      respondWith(200, { status: "ok" }),
    );

    expect(body).toEqual({ status: "ok" });
  });

  test("throws an ApiError with the status of a failed response", async () => {
    const request = apiGet("/api/health", respondWith(503, { status: "unavailable" }));

    await expect(request).rejects.toBeInstanceOf(ApiError);
    await expect(request).rejects.toMatchObject({ status: 503 });
  });

  test("keeps the server's error message and the field it names", async () => {
    const request = apiGet(
      "/api/issues",
      respondWith(422, { error: "title is required", field: "title" }),
    );

    await expect(request).rejects.toMatchObject({
      status: 422,
      message: "title is required",
      field: "title",
    });
  });

  test("falls back to the status when the body is not the server's error", async () => {
    const fetcher = (async () =>
      new Response("Bad gateway", { status: 502 })) as unknown as typeof fetch;

    await expect(apiGet("/api/health", fetcher)).rejects.toMatchObject({
      status: 502,
      message: "GET /api/health failed with status 502",
    });
  });
});

describe("apiWrite", () => {
  test("sends JSON and the session cookie and accepts an empty delete response", async () => {
    const { apiWrite } = await import("./api");
    let options: RequestInit | undefined;
    const fetcher = (async (_path: unknown, init?: RequestInit) => {
      options = init;
      return new Response(null, { status: 204 });
    }) as typeof fetch;
    await apiWrite("/api/tokens/token-id", "DELETE", undefined, fetcher);
    expect(options?.credentials).toBe("same-origin");
    expect(options?.method).toBe("DELETE");
    expect(options?.body).toBeUndefined();
  });

  test("preserves conflict details so an unsaved plan can be recovered", async () => {
    const { apiWrite } = await import("./api");
    await expect(
      apiWrite(
        "/api/issues/WP-1",
        "PATCH",
        { plan: "changed", plan_revision: 1 },
        respondWith(409, { error: "plan changed", field: "plan_revision" }),
      ),
    ).rejects.toMatchObject({ status: 409, message: "plan changed", field: "plan_revision" });
  });
});
