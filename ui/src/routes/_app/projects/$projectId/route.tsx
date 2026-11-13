import { createFileRoute, notFound } from "@tanstack/react-router";

import { meQueryOptions } from "@/features/auth/queries";

export const Route = createFileRoute("/_app/projects/$projectId")({
  beforeLoad: async ({ context, params }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions);
    const project = me.projects.find((item) => item.id === params.projectId);

    if (project === undefined) {
      throw notFound();
    }

    return { project };
  },
});
