import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";

export function PagePlaceholder({ title }: { title: string }) {
  return (
    <Empty className="border">
      <EmptyHeader>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>This page is not available yet.</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}
