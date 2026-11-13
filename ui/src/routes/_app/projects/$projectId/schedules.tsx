import { createFileRoute } from "@tanstack/react-router";

import { PagePlaceholder } from "@/components/page-placeholder";

export const Route = createFileRoute("/_app/projects/$projectId/schedules")({
  component: SchedulesPage,
});

function SchedulesPage() {
  return <PagePlaceholder title="Schedules" />;
}
