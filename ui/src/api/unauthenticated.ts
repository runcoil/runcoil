export const apiBasePath = "/api/v1";

const guardedPaths = new Set(["/auth/login", "/auth/logout", "/me"]);

type Location = {
  pathname: string;
  href: string;
};

export function unauthenticatedRedirect(
  requestPath: string,
  location: Location,
): string | undefined {
  const path = requestPath.startsWith(apiBasePath)
    ? requestPath.slice(apiBasePath.length)
    : requestPath;

  if (guardedPaths.has(path) || /^\/login(?:[/?#]|$)/.test(location.pathname)) {
    return undefined;
  }

  return location.href;
}
