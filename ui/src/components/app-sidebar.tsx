import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useMatch, useMatchRoute } from "@tanstack/react-router";
import { CalendarClock, ListChecks, Play, Server } from "lucide-react";

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";
import { ProjectSwitcher } from "@/components/project-switcher";
import { UserMenu } from "@/components/user-menu";
import { Wordmark } from "@/components/wordmark";
import { meQueryOptions } from "@/features/auth/queries";

const projectNav = [
  { title: "Runs", to: "/projects/$projectId/runs", icon: Play },
  { title: "Tasks", to: "/projects/$projectId/tasks", icon: ListChecks },
  { title: "Schedules", to: "/projects/$projectId/schedules", icon: CalendarClock },
  { title: "Workers", to: "/projects/$projectId/workers", icon: Server },
] as const;

export function AppSidebar() {
  const { data: me } = useSuspenseQuery(meQueryOptions);
  const projectId = useMatch({
    from: "/_app/projects/$projectId",
    shouldThrow: false,
    select: (match) => match.params.projectId,
  });
  const matchRoute = useMatchRoute();
  const current =
    projectId === undefined
      ? me.projects[0]
      : me.projects.find((project) => project.id === projectId);

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link to="/">
                <div className="flex aspect-square size-8 items-center justify-center">
                  <img src="/runcoil-mark.svg" alt="" className="size-6" />
                </div>
                <Wordmark className="h-8! w-auto!" />
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        <ProjectSwitcher projects={me.projects} current={current} />
      </SidebarHeader>
      <SidebarContent>
        {current && (
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                {projectNav.map((item) => (
                  <SidebarMenuItem key={item.to}>
                    <SidebarMenuButton
                      asChild
                      isActive={Boolean(
                        matchRoute({ to: item.to, params: { projectId: current.id }, fuzzy: true }),
                      )}
                      tooltip={item.title}
                    >
                      <Link to={item.to} params={{ projectId: current.id }}>
                        <item.icon />
                        <span>{item.title}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        )}
      </SidebarContent>
      <SidebarFooter>
        <UserMenu user={me.user} />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
