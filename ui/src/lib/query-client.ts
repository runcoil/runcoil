import { QueryClient } from "@tanstack/react-query";

import { isRetryableError } from "@/lib/api-errors";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => failureCount < 2 && isRetryableError(error),
    },
  },
});
