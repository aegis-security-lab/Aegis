import * as React from "react"
import {
  Check,
  ClipboardCheck,
  RefreshCw,
  ShieldAlert,
  Terminal,
  X,
} from "lucide-react"
import { Link } from "react-router-dom"
import { toast } from "sonner"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Textarea } from "@/components/ui/textarea"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { resolveApproval } from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"

const approvalLabels = {
  tool_call: "工具调用",
  issue_rework: "Issue 返工",
  validation_review: "人工验收",
} as const

export function ApprovalsPage() {
  const { state, refresh } = useAppState()
  const [busy, setBusy] = React.useState<string | null>(null)
  const [reviewText, setReviewText] = React.useState<Record<string, string>>({})
  const approvals = state?.approvals ?? []
  const pending = approvals.filter((approval) => approval.status === "pending")
  const history = approvals.filter(
    (approval) =>
      approval.status === "approved" || approval.status === "rejected"
  )

  const decide = async (id: string, approved: boolean) => {
    const approval = pending.find((item) => item.id === id)
    const content = reviewText[id] ?? ""
    if (approval?.type === "validation_review" && !content.trim()) {
      toast.error("请填写人工审阅内容")
      return
    }
    setBusy(id)
    try {
      await resolveApproval(id, approved, content)
      await refresh()
      toast.success(approved ? "已批准" : "已拒绝")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "审批失败")
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Human in the loop"
        title="审批中心"
      />
      {pending.length === 0 ? (
        <Card>
          <CardContent className="py-12">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <ClipboardCheck />
                </EmptyMedia>
                <EmptyTitle>没有待处理审批</EmptyTitle>
                <EmptyDescription>
                  当前没有等待人工决定的工具调用或 Issue 返工请求。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          {pending.map((approval) => {
            const issue = state?.issues.find(
              (item) => item.id === approval.issueId
            )
            const Icon = approval.type === "issue_rework" ? RefreshCw : Terminal
            return (
              <Card key={approval.id} className="border-amber-300/60">
                <CardHeader>
                  <div className="flex items-start gap-3">
                    <span className="flex size-10 items-center justify-center rounded-xl bg-amber-100 text-amber-700">
                      <ShieldAlert className="size-5" />
                    </span>
                    <div className="min-w-0">
                      <div className="mb-1 flex flex-wrap items-center gap-2">
                        <Badge variant="outline">
                          <Icon />
                          {approvalLabels[approval.type] ?? approval.type}
                        </Badge>
                      </div>
                      <CardTitle className="text-base">
                        {approval.title}
                      </CardTitle>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {issue?.identifier} · {formatTime(approval.createdAt)}
                      </p>
                    </div>
                  </div>
                </CardHeader>
                <CardContent>
                  <pre className="max-h-64 overflow-auto rounded-xl border bg-muted/35 p-4 text-xs whitespace-pre-wrap">
                    {approval.detail}
                  </pre>
                  {approval.type === "validation_review" ? (
                    <Textarea
                      className="mt-4"
                      value={reviewText[approval.id] ?? ""}
                      onChange={(event) =>
                        setReviewText((current) => ({
                          ...current,
                          [approval.id]: event.target.value,
                        }))
                      }
                      placeholder="填写人工审阅意见、验收依据或需要返工的具体内容…"
                      rows={5}
                    />
                  ) : null}
                  <div className="mt-4 flex justify-between">
                    <Button
                      variant="ghost"
                      render={<Link to={`/issues/${approval.issueId}`} />}
                    >
                      查看 Issue
                    </Button>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        disabled={busy === approval.id}
                        onClick={() => void decide(approval.id, false)}
                      >
                        <X />
                        拒绝
                      </Button>
                      <Button
                        disabled={busy === approval.id}
                        onClick={() => void decide(approval.id, true)}
                      >
                        <Check />
                        批准
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}
      {history.length > 0 ? (
        <section>
          <h2 className="mb-3 text-sm font-medium">最近记录</h2>
          <Card className="divide-y overflow-hidden p-0">
            {history.slice(0, 20).map((approval) => (
              <div key={approval.id} className="flex items-center gap-4 p-4">
                <StatusBadge status={approval.status} />
                <Badge variant="outline">
                  {approvalLabels[approval.type] ?? approval.type}
                </Badge>
                <p className="min-w-0 flex-1 truncate text-sm">
                  {approval.title}
                </p>
                <span className="text-xs text-muted-foreground">
                  {formatTime(approval.resolvedAt ?? approval.createdAt)}
                </span>
              </div>
            ))}
          </Card>
        </section>
      ) : null}
    </div>
  )
}
