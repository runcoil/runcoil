import { describe, expect, it } from "vite-plus/test";

import { loginPath, safeRedirect } from "@/lib/redirect";

describe("safeRedirect", () => {
  it("keeps an in-app path", () => {
    expect(safeRedirect("/projects/abc/runs?status=running")).toBe(
      "/projects/abc/runs?status=running",
    );
  });

  it("falls back to the home page for anything that leaves the app or loops to login", () => {
    expect(safeRedirect(undefined)).toBe("/");
    expect(safeRedirect("https://example.com/")).toBe("/");
    expect(safeRedirect("//example.com")).toBe("/");
    expect(safeRedirect("/\\example.com")).toBe("/");
    expect(safeRedirect("/login?redirect=/")).toBe("/");
  });
});

describe("loginPath", () => {
  it("encodes the path and its query for the redirect parameter", () => {
    expect(loginPath("/account?tab=1")).toBe("/login?redirect=%2Faccount%3Ftab%3D1");
  });
});
