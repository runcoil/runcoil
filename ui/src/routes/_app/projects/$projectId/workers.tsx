import { createFileRoute } from "@tanstack/react-router";

import { PagePlaceholder } from "@/components/page-placeholder";

export const Route = createFileRoute("/_app/projects/$projectId/workers")({
  component: WorkersPage,
});

function WorkersPage() {
  return <PagePlaceholder title="Workers" />;
}
