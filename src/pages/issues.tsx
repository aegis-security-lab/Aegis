import * as React from "react"
import { ExternalLink, List, Search, Workflow } from "lucide-react"
import { Link } from "react-router-dom"
import { toast } from "sonner"

import { IssueTree } from "@/components/issue-tree"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { updateIssue } from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import type { Issue, IssueStatus } from "@/types"

const statuses: IssueStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "in_review",
  "blocked",
  "done",
  "cancelled",
]

export function IssuesPage() {
  const { state, refresh } = useAppState()
  const [query, setQuery] = React.useState("")
  const [filter, setFilter] = React.useState("all")
  const allIssues = state?.issues ?? []
  const matched = allIssues.filter(
    (issue) =>
      (filter === "all" || issue.status === filter) &&
      `${issue.identifier} ${issue.title}`
        .toLowerCase()
        .includes(query.toLowerCase())
  )
  const treeIssues = includeAncestors(matched, allIssues)

  const change = async (id: string, status: IssueStatus) => {
    try {
      await updateIssue(id, { status })
      await refresh()
      toast.success("Issue 状态已更新")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "更新失败")
    }
  }

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Shared work"
        title="Issues"
        description="层级树表示任务拆解，blocks 表示执行依赖；父 Issue 会在子树完成后进入汇总执行。"
      />

      <div className="flex flex-col gap-3 sm:flex-row">
        <div className="relative max-w-md flex-1">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-9"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索编号或标题…"
          />
        </div>
        <Select
          value={filter}
          onValueChange={(value) => setFilter(String(value))}
        >
          <SelectTrigger className="w-full sm:w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部状态</SelectItem>
            {statuses.map((status) => (
              <SelectItem key={status} value={status}>
                {status}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Tabs defaultValue="tree">
        <div className="mb-3 flex items-center justify-between gap-3">
          <TabsList>
            <TabsTrigger value="tree">
              <Workflow />
              层级树
            </TabsTrigger>
            <TabsTrigger value="table">
              <List />
              列表
            </TabsTrigger>
          </TabsList>
          <span className="text-xs text-muted-foreground">
            {matched.length} / {allIssues.length} Issues
          </span>
        </div>

        <TabsContent value="tree">
          <Card className="overflow-hidden p-0">
            <IssueTree
              issues={treeIssues}
              relations={state?.relations ?? []}
              agents={state?.agents ?? []}
            />
          </Card>
        </TabsContent>

        <TabsContent value="table">
          <Card className="overflow-hidden p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Issue</TableHead>
                  <TableHead>层级 / 依赖</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>Agent</TableHead>
                  <TableHead>更新时间</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {matched.map((issue) => {
                  const blockers =
                    state?.relations.filter(
                      (relation) => relation.relatedIssueId === issue.id
                    ).length ?? 0
                  const children = allIssues.filter(
                    (candidate) => candidate.parentId === issue.id
                  ).length
                  const agent = state?.agents.find(
                    (candidate) => candidate.id === issue.assigneeAgentId
                  )
                  return (
                    <TableRow key={issue.id}>
                      <TableCell className="w-[28rem] max-w-md">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-xs text-muted-foreground">
                            {issue.identifier}
                          </span>
                          <Badge variant="outline">{issue.priority}</Badge>
                        </div>
                        <Link
                          to={`/issues/${issue.id}`}
                          className="mt-1.5 line-clamp-2 font-medium break-all hover:underline"
                          title={issue.title}
                        >
                          {issue.title}
                        </Link>
                        <p className="mt-1 line-clamp-1 text-xs text-muted-foreground">
                          {issue.acceptanceCriteria || issue.description}
                        </p>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {issue.parentId
                          ? `第 ${issue.requestDepth} 层`
                          : "顶层任务"}
                        <br />
                        {children
                          ? `${children} 个直属子项`
                          : blockers
                            ? `${blockers} 个 blocker`
                            : "无依赖"}
                      </TableCell>
                      <TableCell>
                        <Select
                          value={issue.status}
                          onValueChange={(value) =>
                            void change(issue.id, value as IssueStatus)
                          }
                        >
                          <SelectTrigger
                            size="sm"
                            className="w-32 border-0 bg-transparent px-0 shadow-none"
                          >
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {statuses.map((status) => (
                              <SelectItem key={status} value={status}>
                                <StatusBadge status={status} />
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </TableCell>
                      <TableCell>
                        <p className="text-sm">{agent?.name ?? "未分配"}</p>
                        <p className="font-mono text-xs text-muted-foreground">
                          {issue.assigneeAgentId ?? "—"}
                        </p>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {formatTime(issue.updatedAt)}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          render={
                            <Link
                              to={`/issues/${issue.id}`}
                              aria-label="查看 Issue"
                            />
                          }
                        >
                          <ExternalLink />
                        </Button>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function includeAncestors(matched: Issue[], allIssues: Issue[]) {
  const issueMap = new Map(allIssues.map((issue) => [issue.id, issue]))
  const visible = new Map<string, Issue>()
  for (const issue of matched) {
    visible.set(issue.id, issue)
    let parentID = issue.parentId
    while (parentID) {
      const parent = issueMap.get(parentID)
      if (!parent || visible.has(parent.id)) break
      visible.set(parent.id, parent)
      parentID = parent.parentId
    }
  }
  return [...visible.values()]
}
