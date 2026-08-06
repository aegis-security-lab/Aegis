import { LayoutGrid } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"

export function TaskBoardPage() {
  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        eyebrow="Board"
        title="Board"
        description="以看板方式查看该任务下的 Issue 流转。"
      />
      <div className="flex min-h-64 items-center justify-center rounded-xl border bg-muted/20">
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <LayoutGrid />
            </EmptyMedia>
            <EmptyTitle>看板视图尚未开放</EmptyTitle>
            <EmptyDescription>
              Board 页面会在后续版本接入，当前可先使用 Home 和 Issues。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </div>
    </div>
  )
}
