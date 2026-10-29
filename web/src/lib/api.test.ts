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
});
