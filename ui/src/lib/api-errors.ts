import { z } from "zod";

import type { components } from "@/api/schema";

export type ErrorResponse = components["schemas"]["ErrorResponse"];
export type ErrorParam = components["schemas"]["ErrorParam"];

const errorResponseSchema = z.object({
  code: z.string(),
  message: z.string(),
  params: z.array(z.object({ field: z.string(), message: z.string() })).optional(),
}) satisfies z.ZodType<ErrorResponse>;

export class ApiError extends Error {
  readonly status: number;
  readonly body: ErrorResponse | undefined;

  constructor(status: number, body?: ErrorResponse) {
    super(body?.message ?? `Request failed with status ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

export function parseErrorResponse(value: unknown): ErrorResponse | undefined {
  const result = errorResponseSchema.safeParse(value);
  return result.success ? result.data : undefined;
}

export function errorResponse(error: unknown): ErrorResponse | undefined {
  return error instanceof ApiError ? error.body : parseErrorResponse(error);
}

export function isUnauthenticatedError(error: unknown): boolean {
  return errorResponse(error)?.code === "unauthenticated";
}

export async function readApiError(response: Response): Promise<ApiError> {
  const text = await response.text();
  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    body = undefined;
  }

  return new ApiError(response.status, parseErrorResponse(body));
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
