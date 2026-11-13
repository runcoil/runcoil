import { describe, expect, it } from "vite-plus/test";

import {
  ApiError,
  apiErrors,
  formatErrorMessage,
  isRetryableError,
  isUnauthenticatedError,
  readApiError,
} from "@/lib/api-errors";

describe("apiErrors", () => {
  it("keeps the well-formed items of an error body", () => {
    const body = {
      errors: [
        { code: "invalid_field", message: "current password is wrong", field: "currentPassword" },
        { code: "internal", message: "something broke" },
        { code: 42, message: "not an item" },
      ],
    };

    expect(apiErrors(body)).toEqual([
      { code: "invalid_field", message: "current password is wrong", field: "currentPassword" },
      { code: "internal", message: "something broke" },
    ]);
  });

  it("returns nothing for values that are not an error body", () => {
    expect(apiErrors(undefined)).toEqual([]);
    expect(apiErrors("boom")).toEqual([]);
    expect(apiErrors({ errors: "boom" })).toEqual([]);
  });
});

describe("isUnauthenticatedError", () => {
  it("is true for an unauthenticated error body", () => {
    expect(
      isUnauthenticatedError({ errors: [{ code: "unauthenticated", message: "not signed in" }] }),
    ).toBe(true);
  });

  it("is false for other codes and for anything else", () => {
    expect(isUnauthenticatedError({ errors: [{ code: "permission_denied", message: "no" }] })).toBe(
      false,
    );
    expect(isUnauthenticatedError(new Error("network down"))).toBe(false);
    expect(isUnauthenticatedError(null)).toBe(false);
  });
});

describe("readApiError", () => {
  it("keeps the status and the error items of a JSON body", async () => {
    const response = new Response(
      JSON.stringify({ errors: [{ code: "not_found", message: "task not found" }] }),
      { status: 404 },
    );

    const error = await readApiError(response);

    expect(error.status).toBe(404);
    expect(error.errors).toEqual([{ code: "not_found", message: "task not found" }]);
    expect(apiErrors(error)).toEqual(error.errors);
    expect(error.message).toBe("task not found");
  });

  it("has no items for a body that is not JSON", async () => {
    const error = await readApiError(new Response("Bad Gateway", { status: 502 }));

    expect(error.status).toBe(502);
    expect(error.errors).toEqual([]);
  });
});

describe("isRetryableError", () => {
  it("retries network errors and server errors", () => {
    expect(isRetryableError(new TypeError("Failed to fetch"))).toBe(true);
    expect(isRetryableError(new ApiError(500, []))).toBe(true);
    expect(isRetryableError(new ApiError(503, []))).toBe(true);
  });

  it("never retries client errors or anything else", () => {
    expect(isRetryableError(new ApiError(400, []))).toBe(false);
    expect(isRetryableError(new ApiError(401, []))).toBe(false);
    expect(isRetryableError(new ApiError(404, []))).toBe(false);
    expect(isRetryableError(new Error("boom"))).toBe(false);
    expect(isRetryableError({ errors: [] })).toBe(false);
  });
});

describe("formatErrorMessage", () => {
  it("upper-cases the first letter", () => {
    expect(formatErrorMessage("current password is wrong")).toBe("Current password is wrong");
    expect(formatErrorMessage("Already capitalised")).toBe("Already capitalised");
    expect(formatErrorMessage("")).toBe("");
  });
});
