export function safeRedirect(target: string | undefined): string {
  if (target === undefined || !target.startsWith("/")) {
    return "/";
  }

  if (target.startsWith("//") || target.startsWith("/\\") || /^\/login(?:[/?#]|$)/.test(target)) {
    return "/";
  }

  return target;
}

export function loginPath(target: string): string {
  return `/login?redirect=${encodeURIComponent(target)}`;
}
