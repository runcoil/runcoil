import { describe, expect, it, vi } from "vite-plus/test";

import { ApiError } from "@/lib/api-errors";
import { setServerErrors } from "@/lib/form-errors";

type Values = { currentPassword: string; newPassword: string };

const fields = ["currentPassword", "newPassword"] as const;
const fallback = "Could not change the password. Try again.";

function apply(error: unknown) {
  const setError = vi.fn();
  setServerErrors<Values>(setError, error, fields, fallback);
  return setError.mock.calls;
}

describe("setServerErrors", () => {
  it("puts each param on its form field", () => {
    const error = new ApiError(400, {
      code: "invalid_request",
      message: "current password is wrong",
      params: [
        { field: "currentPassword", message: "current password is wrong" },
        { field: "newPassword", message: "use a digit" },
      ],
    });

    expect(apply(error)).toEqual([
      ["currentPassword", { message: "Current password is wrong" }],
      ["newPassword", { message: "Use a digit" }],
    ]);
  });

  it("puts a param for another field on the root", () => {
    const error = new ApiError(400, {
      code: "invalid_request",
      message: "bad request",
      params: [
        { field: "currentPassword", message: "current password is wrong" },
        { field: "body", message: "body is not JSON" },
      ],
    });

    expect(apply(error)).toEqual([
      ["currentPassword", { message: "Current password is wrong" }],
      ["root", { message: "Body is not JSON" }],
    ]);
  });

  it("puts the message on the root when there are no params", () => {
    const error = new ApiError(403, { code: "permission_denied", message: "not allowed" });

    expect(apply(error)).toEqual([["root", { message: "Not allowed" }]]);
  });

  it("falls back when the error has no body", () => {
    expect(apply(new ApiError(502))).toEqual([["root", { message: fallback }]]);
    expect(apply(new TypeError("Failed to fetch"))).toEqual([["root", { message: fallback }]]);
  });
});
