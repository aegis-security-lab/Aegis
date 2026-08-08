import { Skeleton } from "@/components/ui/skeleton"

export function PageSkeleton() {
  return (
    <div
      className="flex min-h-64 w-full flex-col gap-5"
      aria-label="正在加载页面"
    >
      <div className="flex items-end justify-between border-b border-border/60 pb-4">
        <div className="flex flex-col gap-2">
          <Skeleton className="h-2.5 w-24" />
          <Skeleton className="h-7 w-40" />
        </div>
        <Skeleton className="h-7 w-20 rounded-full" />
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1.35fr)_minmax(18rem,.65fr)]">
        <Skeleton className="h-56 w-full rounded-xl" />
        <Skeleton className="h-56 w-full rounded-xl" />
      </div>
    </div>
  )
}
