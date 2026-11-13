import type { FieldValues, Path, UseFormSetError } from "react-hook-form";

import { errorResponse, formatErrorMessage } from "@/lib/api-errors";

export function setServerErrors<T extends FieldValues>(
  setError: UseFormSetError<T>,
  error: unknown,
  fields: readonly Path<T>[],
  fallback: string,
) {
  const body = errorResponse(error);
  if (!body) {
    setError("root", { message: fallback });
    return;
  }

  const params = body.params ?? [];
  let rootMessage = params.length === 0 ? body.message : undefined;
  for (const param of params) {
    const field = fields.find((name) => name === param.field);
    if (field) {
      setError(field, { message: formatErrorMessage(param.message) });
    } else {
      rootMessage ??= param.message;
    }
  }

  if (rootMessage !== undefined) {
    setError("root", { message: formatErrorMessage(rootMessage) });
  }
}
