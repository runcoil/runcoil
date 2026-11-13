import { useQueryErrorResetBoundary } from "@tanstack/react-query";
import { useRouter, type ErrorComponentProps } from "@tanstack/react-router";
import { CircleAlert } from "lucide-react";
import { useEffect } from "react";

import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { formatErrorMessage } from "@/lib/api-errors";

export function RouteError({ error }: ErrorComponentProps) {
  const router = useRouter();
  const queryErrorResetBoundary = useQueryErrorResetBoundary();

  useEffect(() => {
    queryErrorResetBoundary.reset();
  }, [queryErrorResetBoundary]);

  return (
    <Empty>
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <CircleAlert />
        </EmptyMedia>
        <EmptyTitle>Something went wrong</EmptyTitle>
        <EmptyDescription>
          {error instanceof Error && error.message
            ? formatErrorMessage(error.message)
            : "The page could not be loaded."}
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button onClick={() => void router.invalidate()}>Try again</Button>
      </EmptyContent>
    </Empty>
  );
}

export function FullPageRouteError(props: ErrorComponentProps) {
  return (
    <main className="grid min-h-svh place-items-center p-6">
      <RouteError {...props} />
    </main>
  );
}
