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
