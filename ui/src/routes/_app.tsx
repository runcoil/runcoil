import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";

import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/app-sidebar";
import { NotFound } from "@/components/not-found";
import { FullPageRouteError } from "@/components/route-error";
import { meQueryOptions } from "@/features/auth/queries";
import { isUnauthenticatedError } from "@/lib/api-errors";

export const Route = createFileRoute("/_app")({
  beforeLoad: async ({ context, location }) => {
    try {
      await context.queryClient.ensureQueryData(meQueryOptions);
    } catch (error) {
      if (isUnauthenticatedError(error)) {
        throw redirect({ to: "/login", search: { redirect: location.href } });
      }

      throw error;
    }
  },
  component: AppLayout,
  notFoundComponent: NotFound,
  errorComponent: FullPageRouteError,
});

function AppLayout() {
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-12 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
        </header>
        <div className="flex flex-1 flex-col p-6">
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
