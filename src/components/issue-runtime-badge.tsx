import {
  CircleCheck,
  CircleDotDashed,
  CirclePause,
  CircleX,
  TriangleAlert,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Spinner } from "@/components/ui/spinner"
import { issueRuntimeLabel } from "@/lib/issue-runtime"
import type { IssueRuntimeView } from "@/types"

export function IssueRuntimeBadge({ runtime }: { runtime: IssueRuntimeView }) {
  const label = issueRuntimeLabel(runtime)
  if (runtime.kind === "running") {
    return (
      <Badge className="h-5 px-1.5 text-[10px]">
        <Spinner data-icon="inline-start" />
        {label}
      </Badge>
    )
  }
  if (runtime.kind === "waiting") {
    return (
      <Badge variant="outline" className="h-5 px-1.5 text-[10px] text-warning">
        <CirclePause data-icon="inline-start" />
        {label}
      </Badge>
    )
  }
  if (runtime.kind === "failed") {
    return (
      <Badge variant="destructive" className="h-5 px-1.5 text-[10px]">
        <TriangleAlert data-icon="inline-start" />
        {label}
      </Badge>
    )
  }
  if (runtime.kind === "completed") {
    return (
      <Badge variant="secondary" className="h-5 px-1.5 text-[10px]">
        <CircleCheck data-icon="inline-start" />
        {label}
      </Badge>
    )
  }
  if (runtime.kind === "cancelled") {
    return (
      <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
        <CircleX data-icon="inline-start" />
        {label}
      </Badge>
    )
  }
  return (
    <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
      <CircleDotDashed data-icon="inline-start" />
      {label}
    </Badge>
  )
}
