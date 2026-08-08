import * as React from "react"
import { MessageSquareText } from "lucide-react"
import { Link } from "react-router-dom"

import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { buttonVariants } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
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
  const deferredQuery = React.useDeferredValue(query)
  const sessions = React.useMemo(() => {
    const normalized = deferredQuery.trim().toLocaleLowerCase()
    return (state?.sessions ?? []).filter((session) =>
      `${session.execution.id} ${session.execution.sessionId} ${session.agentName} ${session.issueIdentifier} ${session.issueTitle}`
        .toLocaleLowerCase()
        .includes(normalized)
    )
  }, [deferredQuery, state?.sessions])

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Runtime observability"
        title="Sessions"
        description="检查 Agent 会话状态、模型用量与完整对话记录。"
      />
      <Card>
        <CardHeader className="gap-4 md:flex-row md:items-end md:justify-between">
          <div>
            <CardTitle>AgentCore 会话</CardTitle>
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
                    <Link
                      to={`/sessions/${session.execution.id}`}
                      className={buttonVariants({
                        size: "sm",
                        variant: "outline",
                      })}
                    >
                      <MessageSquareText data-icon="inline-start" />
                      查看对话
                    </Link>
                  </TableCell>
                </TableRow>
              ))}
              {sessions.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={7}
                    className="h-32 text-center text-sm text-muted-foreground"
                  >
                    没有匹配的会话
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}
