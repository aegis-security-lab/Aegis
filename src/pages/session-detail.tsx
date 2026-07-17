import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import {
  ArrowLeft,
  Bot,
  Clock3,
  Coins,
  ExternalLink,
  MessagesSquare,
  SquareTerminal,
} from "lucide-react"
import { Link, useParams } from "react-router-dom"

import { ExecutionEvents } from "@/components/execution-events"
import { PageHeader } from "@/components/page-header"
import { SessionConversation } from "@/components/session-conversation"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { Spinner } from "@/components/ui/spinner"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { fetchSessionDetail } from "@/lib/api"
import { formatDuration, formatTime, formatTokens } from "@/lib/format"
import { useAppState } from "@/lib/state"
import type { SessionDetail } from "@/types"

export function SessionDetailPage() {
  const { executionId } = useParams()
  const { state } = useAppState()
  const [detail, setDetail] = React.useState<SessionDetail | null>(null)
  const [error, setError] = React.useState<string | null>(null)

  const load = React.useCallback(async () => {
    if (!executionId) return
    try {
      setDetail(await fetchSessionDetail(executionId))
      setError(null)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Session 读取失败")
    }
  }, [executionId])

  React.useEffect(() => {
    void load()
  }, [load, state?.updatedAt])

  if (error && !detail) {
    return (
      <Empty className="min-h-96">
        <EmptyHeader>
          <EmptyTitle>无法打开 Session</EmptyTitle>
          <EmptyDescription>{error}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button
            variant="outline"
            render={<Link to="/sessions" />}
            nativeButton={false}
          >
            <ArrowLeft data-icon="inline-start" />
            返回 Sessions
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  if (!detail) {
    return (
      <div className="flex min-h-96 items-center justify-center">
        <Spinner className="size-5 text-muted-foreground" />
      </div>
    )
  }

  const { execution } = detail.session
  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow={`${detail.session.issueIdentifier} · Session detail`}
        title={detail.session.agentName}
        description={execution.sessionId}
        actions={
          <>
            <Button
              variant="ghost"
              render={<Link to="/sessions" />}
              nativeButton={false}
            >
              <ArrowLeft data-icon="inline-start" />
              返回
            </Button>
            <Button
              variant="outline"
              render={<Link to={`/issues/${execution.issueId}`} />}
              nativeButton={false}
            >
              <ExternalLink data-icon="inline-start" />
              查看 Issue
            </Button>
          </>
        }
      />

      <div className="flex flex-wrap items-center gap-2">
        <StatusBadge status={execution.status} />
        <Badge variant="secondary">{execution.kind}</Badge>
        <Badge variant="outline">
          {execution.provider} / {execution.model}
        </Badge>
        <Badge variant="outline">{formatTokens(execution.tokens)} tokens</Badge>
        <Badge variant="outline">${execution.cost.toFixed(4)}</Badge>
      </div>

      <Tabs defaultValue="conversation" className="min-h-0">
        <TabsList variant="line">
          <TabsTrigger value="conversation">
            <MessagesSquare />
            对话 {detail.messages.length}
          </TabsTrigger>
          <TabsTrigger value="events">
            <SquareTerminal />
            事件 {detail.events.length}
          </TabsTrigger>
          <TabsTrigger value="metadata">运行信息</TabsTrigger>
        </TabsList>

        <TabsContent value="conversation" className="pt-4">
          <Card className="h-[calc(100svh-17rem)] min-h-[520px] gap-0 py-0">
            <CardHeader className="border-b py-4">
              <CardTitle className="flex items-center gap-2">
                <Bot className="size-4" />
                {detail.session.issueTitle}
              </CardTitle>
              <CardDescription>
                这里展示该 Pi Session 中实际持久化的用户与 AI 消息。
              </CardDescription>
            </CardHeader>
            <CardContent className="min-h-0 flex-1 p-0">
              <SessionConversation
                messages={detail.messages}
                agentName={detail.session.agentName}
                issueIdentifier={detail.session.issueIdentifier}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="events" className="pt-4">
          <ExecutionEvents
            events={detail.events}
            issues={state?.issues ?? []}
          />
        </TabsContent>

        <TabsContent value="metadata" className="pt-4">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <MetaCard
              icon={Bot}
              title="运行身份"
              items={[
                ["Agent", detail.session.agentName],
                ["Execution", execution.id],
                ["Session", execution.sessionId],
              ]}
            />
            <MetaCard
              icon={SquareTerminal}
              title="模型配置"
              items={[
                ["Provider", execution.provider],
                ["Model", execution.model],
                ["Thinking", execution.thinking],
                ["Current tool", execution.currentTool || "—"],
              ]}
            />
            <MetaCard
              icon={Coins}
              title="资源用量"
              items={[
                ["Tokens", formatTokens(execution.tokens)],
                ["Cost", `$${execution.cost.toFixed(4)}`],
                ["Messages", String(detail.messages.length)],
                ["Approvals", String(detail.approvals.length)],
              ]}
            />
            <MetaCard
              icon={Clock3}
              title="时间"
              items={[
                ["Started", formatTime(execution.startedAt)],
                ["Updated", formatTime(execution.updatedAt)],
                ["Finished", formatTime(execution.finishedAt)],
                [
                  "Duration",
                  formatDuration(execution.startedAt, execution.finishedAt),
                ],
              ]}
            />
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function MetaCard({
  icon: Icon,
  title,
  items,
}: {
  icon: React.ComponentType<{ className?: string }>
  title: string
  items: Array<[string, string]>
}) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Icon className="size-4" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="flex flex-col gap-3">
          {items.map(([label, value]) => (
            <div key={label} className="flex flex-col gap-1">
              <dt className="text-xs text-muted-foreground">{label}</dt>
              <dd className="font-mono text-xs break-all">{value || "—"}</dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  )
}
