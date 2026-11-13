import { describe, expect, it } from "vite-plus/test";

import {
  ApiError,
  errorResponse,
  formatErrorMessage,
  isRetryableError,
  isUnauthenticatedError,
  parseErrorResponse,
  readApiError,
} from "@/lib/api-errors";

describe("parseErrorResponse", () => {
  it("keeps an error body with its params", () => {
    const body = {
      code: "invalid_request",
      message: "current password is wrong",
      params: [{ field: "currentPassword", message: "current password is wrong" }],
    };

    expect(parseErrorResponse(body)).toEqual(body);
  });

  it("keeps an error body without params", () => {
    expect(parseErrorResponse({ code: "internal", message: "something broke" })).toEqual({
      code: "internal",
      message: "something broke",
    });
  });

  it("returns nothing for values that are not an error body", () => {
    expect(parseErrorResponse(undefined)).toBeUndefined();
    expect(parseErrorResponse("boom")).toBeUndefined();
    expect(parseErrorResponse({ message: "no code" })).toBeUndefined();
    expect(parseErrorResponse({ code: "missing_field", message: "old code" })).toBeUndefined();
    expect(
      parseErrorResponse({ errors: [{ code: "internal", message: "old shape" }] }),
    ).toBeUndefined();
    expect(
      parseErrorResponse({ code: "invalid_request", message: "bad", params: [{ field: 1 }] }),
    ).toBeUndefined();
  });
});

describe("errorResponse", () => {
  it("reads the body of an ApiError", () => {
    const body = { code: "not_found", message: "task not found" } as const;

    expect(errorResponse(new ApiError(404, body))).toEqual(body);
    expect(errorResponse(new ApiError(502))).toBeUndefined();
  });

  it("reads a raw error body", () => {
    expect(errorResponse({ code: "permission_denied", message: "no" })).toEqual({
      code: "permission_denied",
      message: "no",
    });
  });
});

describe("isUnauthenticatedError", () => {
  it("is true for an unauthenticated error", () => {
    expect(isUnauthenticatedError({ code: "unauthenticated", message: "not signed in" })).toBe(
      true,
    );
    expect(
      isUnauthenticatedError(
        new ApiError(401, { code: "unauthenticated", message: "not signed in" }),
      ),
    ).toBe(true);
  });

  it("is false for other codes and for anything else", () => {
    expect(isUnauthenticatedError({ code: "permission_denied", message: "no" })).toBe(false);
    expect(isUnauthenticatedError(new ApiError(401))).toBe(false);
    expect(isUnauthenticatedError(new Error("network down"))).toBe(false);
    expect(isUnauthenticatedError(null)).toBe(false);
  });
});

describe("readApiError", () => {
  it("keeps the status and the body of a JSON error", async () => {
    const body = {
      code: "invalid_request",
      message: "login is required",
      params: [
        { field: "login", message: "login is required" },
        { field: "password", message: "password is required" },
      ],
    };
    const response = new Response(JSON.stringify(body), { status: 400 });

    const error = await readApiError(response);

    expect(error.status).toBe(400);
    expect(error.body).toEqual(body);
    expect(error.message).toBe("login is required");
  });

  it("has no body for a body that is not JSON", async () => {
    const error = await readApiError(new Response("Bad Gateway", { status: 502 }));

    expect(error.status).toBe(502);
    expect(error.body).toBeUndefined();
    expect(error.message).toBe("Request failed with status 502");
  });
});

describe("isRetryableError", () => {
  it("retries network errors and server errors", () => {
    expect(isRetryableError(new TypeError("Failed to fetch"))).toBe(true);
    expect(isRetryableError(new ApiError(500))).toBe(true);
    expect(isRetryableError(new ApiError(503))).toBe(true);
  });

  it("never retries client errors or anything else", () => {
    expect(isRetryableError(new ApiError(400))).toBe(false);
    expect(isRetryableError(new ApiError(401))).toBe(false);
    expect(isRetryableError(new ApiError(404, { code: "not_found", message: "gone" }))).toBe(false);
    expect(isRetryableError(new Error("boom"))).toBe(false);
    expect(isRetryableError({ code: "internal", message: "boom" })).toBe(false);
  });
});

describe("formatErrorMessage", () => {
  it("upper-cases the first letter", () => {
    expect(formatErrorMessage("current password is wrong")).toBe("Current password is wrong");
    expect(formatErrorMessage("Already capitalised")).toBe("Already capitalised");
    expect(formatErrorMessage("")).toBe("");
  });
});
