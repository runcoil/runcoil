export const apiBasePath = "/api/v1";

const guardedPaths = new Set(["/auth/login", "/auth/logout"]);

type Location = {
  pathname: string;
  href: string;
};

export function unauthenticatedRedirect(
  requestPath: string,
  location: Location,
  signedIn: boolean,
): string | undefined {
  const path = requestPath.startsWith(apiBasePath)
    ? requestPath.slice(apiBasePath.length)
    : requestPath;

  if (
    guardedPaths.has(path) ||
    (path === "/me" && !signedIn) ||
    /^\/login(?:[/?#]|$)/.test(location.pathname)
  ) {
    return undefined;
  }

  return location.href;
}
