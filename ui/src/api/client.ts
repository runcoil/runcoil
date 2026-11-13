import type { RegisteredRouter } from "@tanstack/react-router";
import createFetchClient from "openapi-fetch";
import createClient from "openapi-react-query";

import type { paths } from "@/api/schema";
import { apiBasePath, unauthenticatedRedirect } from "@/api/unauthenticated";
import { readApiError } from "@/lib/api-errors";
import { queryClient } from "@/lib/query-client";

let router: RegisteredRouter | undefined;

export function connectRouter(value: RegisteredRouter) {
  router = value;
}

export const client = createFetchClient<paths>({ baseUrl: apiBasePath });

client.use({
  async onResponse({ request, response }) {
    if (response.ok) {
      return;
    }

    if (response.status === 401 && router) {
      const redirect = unauthenticatedRedirect(
        new URL(request.url).pathname,
        router.latestLocation,
        queryClient.getQueryData(api.queryOptions("get", "/me").queryKey) !== undefined,
      );
      if (redirect !== undefined) {
        queryClient.clear();
        void router.navigate({ to: "/login", search: { redirect } });
      }
    }

    throw await readApiError(response);
  },
});

export const api = createClient(client);
