import { Link } from "@tanstack/react-router";
import { ChevronsUpDown } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import type { components } from "@/api/schema";

type ProjectAccess = components["schemas"]["ProjectAccess"];

type ProjectSwitcherProps = {
  projects: ProjectAccess[];
  current: ProjectAccess | undefined;
};

export function ProjectSwitcher({ projects, current }: ProjectSwitcherProps) {
  const { isMobile } = useSidebar();

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg" className="data-[state=open]:bg-sidebar-accent">
              <Avatar className="size-8 rounded-md">
                <AvatarFallback className="rounded-md bg-primary text-primary-foreground">
                  {current?.name.charAt(0).toUpperCase() ?? "?"}
                </AvatarFallback>
              </Avatar>
              <span className="grid flex-1 text-left leading-tight">
                <span className="truncate font-medium">{current?.name ?? "No project"}</span>
                {current && (
                  <span className="truncate text-xs text-sidebar-foreground/70">
                    {current.role}
                  </span>
                )}
              </span>
              <ChevronsUpDown className="ml-auto" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="start"
            side={isMobile ? "bottom" : "right"}
            sideOffset={4}
            className="min-w-56 rounded-lg"
          >
            <DropdownMenuLabel>Projects</DropdownMenuLabel>
            {projects.length === 0 && <DropdownMenuItem disabled>No projects</DropdownMenuItem>}
            {projects.map((project) => (
              <DropdownMenuItem key={project.id} asChild>
                <Link to="/projects/$projectId/runs" params={{ projectId: project.id }}>
                  {project.name}
                </Link>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
