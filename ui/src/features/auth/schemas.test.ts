import { describe, expect, it } from "vite-plus/test";

import { changePasswordSchema, passwordSchema } from "@/features/auth/schemas";

describe("passwordSchema", () => {
  it.each(["Secret-123", "Mật-khẩu1", "Secret 123"])("accepts %s", (password) => {
    expect(passwordSchema.safeParse(password).success).toBe(true);
  });

  it.each([
    ["Secr3t!", "at least 8"],
    ["secret-123", "upper-case"],
    ["SECRET-123", "lower-case"],
    ["Secretabc-", "digit"],
    ["Secret1234", "special"],
  ])("rejects %s, which has no %s rule met", (password) => {
    expect(passwordSchema.safeParse(password).success).toBe(false);
  });

  it("counts characters, not UTF-16 units", () => {
    expect(passwordSchema.safeParse("Aa1-😀😀😀").success).toBe(false);
    expect(passwordSchema.safeParse("Aa1-😀😀😀😀").success).toBe(true);
  });
});

describe("changePasswordSchema", () => {
  const base = {
    currentPassword: "admin",
    newPassword: "Secret-123",
    confirmPassword: "Secret-123",
  };

  it("accepts a matching confirmation", () => {
    expect(changePasswordSchema.safeParse(base).success).toBe(true);
  });

  it("reports a mismatch on the confirmation field", () => {
    const result = changePasswordSchema.safeParse({ ...base, confirmPassword: "Secret-124" });

    expect(result.success).toBe(false);
    expect(result.error?.issues[0]?.path).toEqual(["confirmPassword"]);
  });
});
