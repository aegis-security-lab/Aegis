import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import {
  FilePlus2,
  FileText,
  Pencil,
  Trash2,
  Upload,
} from "lucide-react"
import { useParams } from "react-router-dom"
import { toast } from "sonner"

import { KnowledgeBaseDialog } from "@/components/knowledge-base-dialog"
import { MarkdownContent } from "@/components/markdown-content"
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
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import {
  createKnowledgeDocument,
  deleteKnowledgeDocument,
  fetchKnowledgeBase,
  updateKnowledgeDocument,
  type SaveKnowledgeDocumentInput,
} from "@/lib/api"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type {
  KnowledgeBaseDetail,
  KnowledgeDocument,
} from "@/types"

export function KnowledgeBaseDetailPage() {
  const { knowledgeBaseId = "" } = useParams()
  const { refresh: refreshState } = useAppState()
  const uploadRef = React.useRef<HTMLInputElement>(null)
  const [detail, setDetail] = React.useState<KnowledgeBaseDetail | null>(null)
  const [error, setError] = React.useState("")
  const [selectedID, setSelectedID] = React.useState("")
  const [editingBase, setEditingBase] = React.useState(false)
  const [editingDocument, setEditingDocument] = React.useState<
    KnowledgeDocument | "new" | null
  >(null)
  const [removingDocument, setRemovingDocument] =
    React.useState<KnowledgeDocument | null>(null)
  const [deleting, setDeleting] = React.useState(false)
  const [uploading, setUploading] = React.useState(false)

  const load = React.useCallback(async () => {
    try {
      const next = await fetchKnowledgeBase(knowledgeBaseId)
      setDetail(next)
      setError("")
      setSelectedID((current) =>
        next.documents.some((document) => document.id === current)
          ? current
          : (next.documents[0]?.id ?? "")
      )
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "读取知识库详情失败"
      )
    }
  }, [knowledgeBaseId])

  React.useEffect(() => {
    void load()
  }, [load])

  const selectedDocument = detail?.documents.find(
    (document) => document.id === selectedID
  )

  const upload = async (files: FileList | null) => {
    if (!files?.length || uploading) return
    setUploading(true)
    let imported = 0
    try {
      for (const file of Array.from(files)) {
        if (!file.name.toLocaleLowerCase().endsWith(".md")) {
          throw new Error(`${file.name} 不是 Markdown 文件`)
        }
        if (file.size > 512 * 1024) {
          throw new Error(`${file.name} 超过 512 KiB`)
        }
        await createKnowledgeDocument(knowledgeBaseId, {
          name: file.name,
          content: await file.text(),
        })
        imported += 1
      }
      toast.success(`已导入 ${imported} 篇 Markdown 文档`)
      await Promise.all([load(), refreshState()])
    } catch (reason) {
      toast.error(
        reason instanceof Error ? reason.message : "导入 Markdown 文档失败"
      )
      if (imported > 0) await Promise.all([load(), refreshState()])
    } finally {
      setUploading(false)
      if (uploadRef.current) uploadRef.current.value = ""
    }
  }

  const removeDocument = async () => {
    if (!removingDocument || deleting) return
    setDeleting(true)
    try {
      await deleteKnowledgeDocument(removingDocument.id)
      toast.success("文档已删除")
      setRemovingDocument(null)
      await Promise.all([load(), refreshState()])
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "删除文档失败")
    } finally {
      setDeleting(false)
    }
  }

  if (!detail && !error) {
    return (
      <div className="flex min-h-80 items-center justify-center">
        <Spinner className="size-5 text-muted-foreground" />
      </div>
    )
  }

  if (!detail) {
    return (
      <Card>
        <CardContent className="py-16">
          <Empty>
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <FileText />
              </EmptyMedia>
              <EmptyTitle>无法读取知识库</EmptyTitle>
              <EmptyDescription>{error}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center justify-start gap-3">
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => setEditingBase(true)}>
            <Pencil data-icon="inline-start" />
            编辑介绍
          </Button>
          <input
            ref={uploadRef}
            className="sr-only"
            type="file"
            accept=".md,text/markdown,text/plain"
            multiple
            onChange={(event) => void upload(event.target.files)}
          />
          <Button
            variant="outline"
            disabled={uploading}
            onClick={() => uploadRef.current?.click()}
          >
            {uploading ? (
              <Spinner data-icon="inline-start" />
            ) : (
              <Upload data-icon="inline-start" />
            )}
            导入 Markdown
          </Button>
          <Button onClick={() => setEditingDocument("new")}>
            <FilePlus2 data-icon="inline-start" />
            新增文档
          </Button>
        </div>
      </div>

      <Card size="sm">
        <CardHeader>
          <div className="min-w-0">
            <CardTitle>{detail.knowledgeBase.name}</CardTitle>
            <CardDescription className="mt-1 max-w-4xl whitespace-pre-wrap">
              {detail.knowledgeBase.description}
            </CardDescription>
          </div>
          <CardAction className="flex items-center gap-2">
            <Badge variant="secondary">关键词 + AI</Badge>
            <Badge variant="outline">
              {detail.documents.length} 篇文档
            </Badge>
          </CardAction>
        </CardHeader>
      </Card>

      <Card className="min-h-0">
        {detail.documents.length === 0 ? (
          <CardContent className="py-20">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <FileText />
                </EmptyMedia>
                <EmptyTitle>知识库还是空的</EmptyTitle>
                <EmptyDescription>
                  导入一个或多个 .md 文件，也可以直接创建并编辑 Markdown。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </CardContent>
        ) : (
          <CardContent className="grid min-h-0 gap-0 p-0 lg:grid-cols-[20rem_minmax(0,1fr)]">
            <div className="border-b lg:border-r lg:border-b-0">
              <div className="flex h-12 items-center justify-between border-b px-4">
                <span className="text-sm font-medium">Markdown 文档</span>
                <span className="text-xs text-muted-foreground">
                  {detail.documents.length}
                </span>
              </div>
              <ScrollArea className="h-64 lg:h-[62svh]">
                <div className="p-2">
                  {detail.documents.map((document) => (
                    <button
                      key={document.id}
                      type="button"
                      className={cn(
                        "flex w-full items-start gap-3 rounded-lg px-3 py-3 text-left transition-colors hover:bg-muted/70",
                        selectedID === document.id && "bg-muted"
                      )}
                      onClick={() => setSelectedID(document.id)}
                    >
                      <FileText className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">
                          {document.name}
                        </span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {characterCount(document.content)} 字符
                        </span>
                      </span>
                    </button>
                  ))}
                </div>
              </ScrollArea>
            </div>
            {selectedDocument ? (
              <div className="min-w-0">
                <div className="flex h-12 items-center justify-between gap-3 border-b px-4">
                  <span className="truncate text-sm font-medium">
                    {selectedDocument.name}
                  </span>
                  <div className="flex shrink-0 gap-1">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`编辑 ${selectedDocument.name}`}
                      onClick={() => setEditingDocument(selectedDocument)}
                    >
                      <Pencil />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`删除 ${selectedDocument.name}`}
                      onClick={() => setRemovingDocument(selectedDocument)}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                </div>
                <ScrollArea className="h-[62svh]">
                  <MarkdownContent className="p-5 lg:p-7">
                    {selectedDocument.content}
                  </MarkdownContent>
                </ScrollArea>
              </div>
            ) : null}
          </CardContent>
        )}
      </Card>

      {editingBase ? (
        <KnowledgeBaseDialog
          knowledgeBase={detail.knowledgeBase}
          onOpenChange={setEditingBase}
          onSaved={async () => {
            setEditingBase(false)
            await Promise.all([load(), refreshState()])
          }}
        />
      ) : null}

      {editingDocument ? (
        <KnowledgeDocumentDialog
          key={editingDocument === "new" ? "new" : editingDocument.id}
          knowledgeBaseID={knowledgeBaseId}
          document={editingDocument === "new" ? null : editingDocument}
          onOpenChange={(open) => {
            if (!open) setEditingDocument(null)
          }}
          onSaved={async (document) => {
            setEditingDocument(null)
            setSelectedID(document.id)
            await Promise.all([load(), refreshState()])
          }}
        />
      ) : null}

      <AlertDialog
        open={removingDocument !== null}
        onOpenChange={(open) => {
          if (!open && !deleting) setRemovingDocument(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <Trash2 />
            </AlertDialogMedia>
            <AlertDialogTitle>删除 Markdown 文档？</AlertDialogTitle>
            <AlertDialogDescription>
              「{removingDocument?.name}」会从知识库永久移除，后续检索将不再返回它。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleting}
              onClick={(event) => {
                event.preventDefault()
                void removeDocument()
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

function KnowledgeDocumentDialog({
  knowledgeBaseID,
  document,
  onOpenChange,
  onSaved,
}: {
  knowledgeBaseID: string
  document: KnowledgeDocument | null
  onOpenChange: (open: boolean) => void
  onSaved: (document: KnowledgeDocument) => void | Promise<void>
}) {
  const [form, setForm] = React.useState<SaveKnowledgeDocumentInput>({
    name: document?.name ?? "",
    content: document?.content ?? "",
  })
  const [saving, setSaving] = React.useState(false)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!form.name.trim() || !form.content.trim() || saving) return
    setSaving(true)
    try {
      const saved = document
        ? await updateKnowledgeDocument(document.id, form)
        : await createKnowledgeDocument(knowledgeBaseID, form)
      toast.success(document ? "文档已更新" : "文档已创建")
      await onSaved(saved)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存文档失败")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92svh] overflow-y-auto sm:max-w-5xl">
        <form onSubmit={submit} className="contents">
          <DialogHeader>
            <DialogTitle>{document ? "编辑文档" : "新增文档"}</DialogTitle>
            <DialogDescription>
              文档使用 Markdown 保存；单篇上限 512 KiB。
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="knowledge-document-name">
                文件名
              </FieldLabel>
              <Input
                id="knowledge-document-name"
                autoFocus
                placeholder="architecture.md"
                value={form.name}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
              />
              <FieldDescription>必须使用 .md 扩展名。</FieldDescription>
            </Field>
            <div className="grid min-h-0 gap-4 lg:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="knowledge-document-content">
                  Markdown 内容
                </FieldLabel>
                <Textarea
                  id="knowledge-document-content"
                  className="min-h-96 resize-y font-mono text-xs leading-5"
                  placeholder="# 标题\n\n写下可供 Agent 检索的知识。"
                  value={form.content}
                  onChange={(event) =>
                    setForm((current) => ({
                      ...current,
                      content: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field>
                <FieldLabel>预览</FieldLabel>
                <ScrollArea className="h-96 rounded-lg border">
                  {form.content.trim() ? (
                    <MarkdownContent className="p-4">
                      {form.content}
                    </MarkdownContent>
                  ) : (
                    <p className="p-4 text-sm text-muted-foreground">
                      输入 Markdown 后在这里预览。
                    </p>
                  )}
                </ScrollArea>
              </Field>
            </div>
            <Separator />
            <p className="text-xs text-muted-foreground">
              当前 {characterCount(form.content)} 字符。检索 Agent
              对每篇候选文档只读取开头最多 1000 字符。
            </p>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button
              type="submit"
              disabled={
                saving || !form.name.trim() || !form.content.trim()
              }
            >
              {saving ? <Spinner data-icon="inline-start" /> : null}
              {saving ? "保存中…" : "保存文档"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function characterCount(value: string) {
  return Array.from(value).length.toLocaleString("zh-CN")
}
