import { describe, expect, it } from "vite-plus/test";

import { unauthenticatedRedirect } from "@/api/unauthenticated";

const runs = { pathname: "/projects/abc/runs", href: "/projects/abc/runs?status=failed" };

describe("unauthenticatedRedirect", () => {
  it("sends other calls to login with the current location as the redirect", () => {
    expect(unauthenticatedRedirect("/api/v1/projects/abc/runs", runs)).toBe(
      "/projects/abc/runs?status=failed",
    );
    expect(unauthenticatedRedirect("/api/v1/me/password", runs)).toBe(
      "/projects/abc/runs?status=failed",
    );
  });

  it("leaves the calls that the route guards and auth forms handle", () => {
    expect(unauthenticatedRedirect("/api/v1/me", runs)).toBeUndefined();
    expect(unauthenticatedRedirect("/api/v1/auth/login", runs)).toBeUndefined();
    expect(unauthenticatedRedirect("/api/v1/auth/logout", runs)).toBeUndefined();
  });

  it("does nothing while on the login page", () => {
    const login = { pathname: "/login", href: "/login?redirect=%2F" };

    expect(unauthenticatedRedirect("/api/v1/projects/abc/runs", login)).toBeUndefined();
  });
});
