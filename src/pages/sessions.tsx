import * as React from "react"
import { MessageSquareText } from "lucide-react"
import { Link } from "react-router-dom"

import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { SearchInput } from "@/components/ui/search-input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatTokens } from "@/lib/format"
import { useAppState } from "@/lib/state"

export function SessionsPage() {
  const { state } = useAppState()
  const [query, setQuery] = React.useState("")
  const sessions = (state?.sessions ?? []).filter((session) =>
    `${session.execution.id} ${session.execution.sessionId} ${session.agentName} ${session.issueIdentifier} ${session.issueTitle}`
      .toLowerCase()
      .includes(query.toLowerCase())
  )

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Runtime observability"
        title="Sessions"
        description="查看每次 AgentCore Execution 的完整对话、工具事件和运行信息。"
      />
      <Card>
        <CardHeader className="gap-4 md:flex-row md:items-end md:justify-between">
          <div>
            <CardTitle>AgentCore 会话</CardTitle>
            <CardDescription>
              每个 Issue 中的每个 Agent 对应一个当前 AgentCore
              Session，多次执行会继续同一段对话。
            </CardDescription>
          </div>
          <SearchInput
            value={query}
            onChange={setQuery}
            placeholder="搜索 Agent、Issue、Session…"
          />
        </CardHeader>
        <CardContent className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Agent / Session</TableHead>
                <TableHead>Issue</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>模型</TableHead>
                <TableHead>用量</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {sessions.map((session) => (
                <TableRow key={session.execution.id}>
                  <TableCell>
                    <p className="text-sm font-medium">{session.agentName}</p>
                    <p className="max-w-52 truncate font-mono text-xs text-muted-foreground">
                      {session.execution.sessionId}
                    </p>
                  </TableCell>
                  <TableCell>
                    <p className="font-mono text-xs">
                      {session.issueIdentifier}
                    </p>
                    <p className="max-w-64 truncate text-sm">
                      {session.issueTitle}
                    </p>
                  </TableCell>
                  <TableCell>
                    <span className="text-xs">{session.execution.kind}</span>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={session.execution.status} />
                  </TableCell>
                  <TableCell>
                    <p className="text-sm">{session.execution.model}</p>
                    <p className="text-xs text-muted-foreground">
                      {session.execution.provider}
                    </p>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatTokens(session.execution.tokens)}
                    <br />${session.execution.cost.toFixed(4)}
                  </TableCell>
                  <TableCell>
                    <Button
                      size="sm"
                      variant="outline"
                      render={<Link to={`/sessions/${session.execution.id}`} />}
                      nativeButton={false}
                    >
                      <MessageSquareText data-icon="inline-start" />
                      查看对话
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}
