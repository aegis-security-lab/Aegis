import * as React from "react"
import {
  Bot,
  FileText,
  LibraryBig,
  Pencil,
  Plus,
  Search,
  Trash2,
} from "lucide-react"
import { Link } from "react-router-dom"
import { toast } from "sonner"

import { KnowledgeBaseDialog } from "@/components/knowledge-base-dialog"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Input } from "@/components/ui/input"
import { deleteKnowledgeBase } from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { KnowledgeBase } from "@/types"

export function KnowledgeBasesPage() {
  const { state, refresh } = useAppState()
  const [query, setQuery] = React.useState("")
  const [editing, setEditing] = React.useState<KnowledgeBase | "new" | null>(
    null
  )
  const [removing, setRemoving] = React.useState<KnowledgeBase | null>(null)
  const [deleting, setDeleting] = React.useState(false)
  const knowledgeBases = state?.knowledgeBases ?? []
  const agents = state?.agents ?? []
  const filtered = knowledgeBases.filter((knowledgeBase) => {
    const needle = query.trim().toLocaleLowerCase()
    return (
      !needle ||
      knowledgeBase.name.toLocaleLowerCase().includes(needle) ||
      knowledgeBase.description.toLocaleLowerCase().includes(needle)
    )
  })
  const documentCount = knowledgeBases.reduce(
    (total, knowledgeBase) => total + knowledgeBase.documentCount,
    0
  )
  const associatedAgents = agents.filter(
    (agent) => agent.knowledgeBaseIds.length > 0
  ).length

  const remove = async () => {
    if (!removing || deleting) return
    setDeleting(true)
    try {
      await deleteKnowledgeBase(removing.id)
      toast.success("知识库已删除")
      setRemoving(null)
      await refresh()
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "删除知识库失败")
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-4 sm:grid-cols-3">
        <Summary icon={LibraryBig} label="知识库" value={knowledgeBases.length} />
        <Summary icon={FileText} label="Markdown 文档" value={documentCount} />
        <Summary icon={Bot} label="已关联 Agent" value={associatedAgents} />
      </div>

      <Card size="sm">
        <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              aria-label="搜索知识库"
              className="pl-9"
              placeholder="搜索名称或介绍"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          </div>
          <Button onClick={() => setEditing("new")}>
            <Plus data-icon="inline-start" />
            创建知识库
          </Button>
        </CardContent>
      </Card>

      {filtered.length === 0 ? (
        <Card>
          <CardContent className="py-16">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <LibraryBig />
                </EmptyMedia>
                <EmptyTitle>
                  {knowledgeBases.length === 0
                    ? "还没有知识库"
                    : "没有匹配的知识库"}
                </EmptyTitle>
                <EmptyDescription>
                  {knowledgeBases.length === 0
                    ? "创建知识库后添加 Markdown 文档，再把它关联到需要使用知识的 Agent。"
                    : "尝试缩短关键词或清空搜索条件。"}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-5 lg:grid-cols-2 xl:grid-cols-3">
          {filtered.map((knowledgeBase) => {
            const usedBy = agents.filter((agent) =>
              agent.knowledgeBaseIds.includes(knowledgeBase.id)
            )
            return (
              <Card key={knowledgeBase.id}>
                <CardHeader>
                  <div className="flex min-w-0 items-start gap-3">
                    <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-muted">
                      <LibraryBig className="size-5" />
                    </span>
                    <div className="min-w-0">
                      <CardTitle className="truncate">
                        {knowledgeBase.name}
                      </CardTitle>
                      <CardDescription className="mt-1 line-clamp-3">
                        {knowledgeBase.description}
                      </CardDescription>
                    </div>
                  </div>
                  <CardAction>
                    <Badge variant="secondary">关键词 + AI</Badge>
                  </CardAction>
                </CardHeader>
                <CardContent className="flex flex-col gap-4">
                  <div className="grid grid-cols-2 gap-3">
                    <Metric
                      label="文档"
                      value={`${knowledgeBase.documentCount} 篇`}
                    />
                    <Metric label="关联 Agent" value={`${usedBy.length} 个`} />
                  </div>
                  {usedBy.length > 0 ? (
                    <div className="flex flex-wrap gap-2">
                      {usedBy.slice(0, 4).map((agent) => (
                        <Badge key={agent.id} variant="outline">
                          {agent.name}
                        </Badge>
                      ))}
                      {usedBy.length > 4 ? (
                        <Badge variant="outline">+{usedBy.length - 4}</Badge>
                      ) : null}
                    </div>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      尚未关联 Agent
                    </p>
                  )}
                </CardContent>
                <CardFooter className="justify-between gap-2">
                  <div className="flex gap-1">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`编辑 ${knowledgeBase.name}`}
                      onClick={() => setEditing(knowledgeBase)}
                    >
                      <Pencil />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`删除 ${knowledgeBase.name}`}
                      onClick={() => setRemoving(knowledgeBase)}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    render={
                      <Link to={`/knowledge-bases/${knowledgeBase.id}`} />
                    }
                    nativeButton={false}
                  >
                    管理文档
                  </Button>
                </CardFooter>
              </Card>
            )
          })}
        </div>
      )}

      {editing ? (
        <KnowledgeBaseDialog
          key={editing === "new" ? "new" : editing.id}
          knowledgeBase={editing === "new" ? null : editing}
          onOpenChange={(open) => {
            if (!open) setEditing(null)
          }}
          onSaved={async () => {
            setEditing(null)
            await refresh()
          }}
        />
      ) : null}

      <AlertDialog
        open={removing !== null}
        onOpenChange={(open) => {
          if (!open && !deleting) setRemoving(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <Trash2 />
            </AlertDialogMedia>
            <AlertDialogTitle>删除知识库？</AlertDialogTitle>
            <AlertDialogDescription>
              将永久删除「{removing?.name}」及其中全部 Markdown
              文档。已关联 Agent 的知识库不能直接删除。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleting}
              onClick={(event) => {
                event.preventDefault()
                void remove()
              }}
            >
              {deleting ? "删除中…" : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function Summary({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof LibraryBig
  label: string
  value: number
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 p-5">
        <span className="flex size-10 items-center justify-center rounded-xl bg-muted">
          <Icon className="size-4" />
        </span>
        <div>
          <p className="text-xs text-muted-foreground">{label}</p>
          <p className="mt-1 text-xl font-semibold tabular-nums">{value}</p>
        </div>
      </CardContent>
    </Card>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-muted/60 p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 font-medium">{value}</p>
    </div>
  )
}
