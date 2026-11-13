import { describe, expect, it } from "vite-plus/test";

import { unauthenticatedRedirect } from "@/api/unauthenticated";

const runs = { pathname: "/projects/abc/runs", href: "/projects/abc/runs?status=failed" };

describe("unauthenticatedRedirect", () => {
  it("sends other calls to login with the current location as the redirect", () => {
    expect(unauthenticatedRedirect("/api/v1/projects/abc/runs", runs, true)).toBe(
      "/projects/abc/runs?status=failed",
    );
    expect(unauthenticatedRedirect("/api/v1/me/password", runs, true)).toBe(
      "/projects/abc/runs?status=failed",
    );
  });

  it("leaves the first /me load to the route guard", () => {
    expect(unauthenticatedRedirect("/api/v1/me", runs, false)).toBeUndefined();
  });

  it("sends a /me refetch to login once the user was signed in", () => {
    expect(unauthenticatedRedirect("/api/v1/me", runs, true)).toBe(
      "/projects/abc/runs?status=failed",
    );
  });

  it("leaves the calls that the auth forms handle", () => {
    expect(unauthenticatedRedirect("/api/v1/auth/login", runs, false)).toBeUndefined();
    expect(unauthenticatedRedirect("/api/v1/auth/logout", runs, true)).toBeUndefined();
  });

  it("does nothing while on the login page", () => {
    const login = { pathname: "/login", href: "/login?redirect=%2F" };

    expect(unauthenticatedRedirect("/api/v1/projects/abc/runs", login, true)).toBeUndefined();
    expect(unauthenticatedRedirect("/api/v1/me", login, true)).toBeUndefined();
  });
});
