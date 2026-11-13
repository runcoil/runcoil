export type ApiErrorItem = {
  code: string;
  message: string;
  field?: string;
};

function isApiErrorItem(value: unknown): value is ApiErrorItem {
  return (
    typeof value === "object" &&
    value !== null &&
    "code" in value &&
    typeof value.code === "string" &&
    "message" in value &&
    typeof value.message === "string" &&
    (!("field" in value) || value.field === undefined || typeof value.field === "string")
  );
}

export function apiErrors(error: unknown): ApiErrorItem[] {
  if (typeof error !== "object" || error === null || !("errors" in error)) {
    return [];
  }

  const list: unknown = error.errors;
  if (!Array.isArray(list)) {
    return [];
  }

  const items: readonly unknown[] = list;
  return items.filter(isApiErrorItem);
}

export function isUnauthenticatedError(error: unknown): boolean {
  return apiErrors(error).some((item) => item.code === "unauthenticated");
}

export class ApiError extends Error {
  readonly status: number;
  readonly errors: ApiErrorItem[];

  constructor(status: number, errors: ApiErrorItem[]) {
    super(errors[0]?.message ?? `Request failed with status ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.errors = errors;
  }
}

export async function readApiError(response: Response): Promise<ApiError> {
  const text = await response.text();
  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    body = undefined;
  }

  return new ApiError(response.status, apiErrors(body));
}

export function isRetryableError(error: unknown): boolean {
  if (error instanceof ApiError) {
    return error.status >= 500;
  }

  return error instanceof TypeError;
}

export function formatErrorMessage(message: string): string {
  return message.charAt(0).toUpperCase() + message.slice(1);
}
