import { createFileRoute, redirect } from "@tanstack/react-router";

import { meQueryOptions } from "@/features/auth/queries";

export const Route = createFileRoute("/_app/")({
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions);
    const first = me.projects[0];

    if (first === undefined) {
      throw redirect({ to: "/account" });
    }

    throw redirect({ to: "/projects/$projectId/runs", params: { projectId: first.id } });
  },
});
