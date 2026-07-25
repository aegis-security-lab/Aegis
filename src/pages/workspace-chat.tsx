import * as React from "react"
import {
  ArrowLeft,
  ExternalLink,
  MessageSquare,
  Plus,
  Send,
  Sparkles,
  Trash2,
} from "lucide-react"
import { Link, useNavigate, useParams } from "react-router-dom"
import { toast } from "sonner"

import { ConciergeConversationView } from "@/components/concierge-conversation"
import { Badge } from "@/components/ui/badge"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import {
  createConciergeConversation,
  deleteConciergeConversation,
  fetchConciergeConversation,
  fetchConciergeConversations,
  fetchSessionDelta,
  fetchSessionMessages,
  sendConciergeMessage,
} from "@/lib/api"
import { chronological, mergeById } from "@/lib/collections"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type {
  ConciergeConversation,
  ConciergeConversationDetail,
} from "@/types"

export function WorkspaceChatPage() {
  const { conversationId } = useParams()
  const navigate = useNavigate()
  const { state } = useAppState()
  const [conversations, setConversations] = React.useState<
    ConciergeConversation[]
  >([])
  const [detail, setDetail] =
    React.useState<ConciergeConversationDetail | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [creating, setCreating] = React.useState(false)
  const [sending, setSending] = React.useState(false)
  const [loadingMore, setLoadingMore] = React.useState(false)
  const [draft, setDraft] = React.useState("")
  const [deleteTarget, setDeleteTarget] =
    React.useState<ConciergeConversation | null>(null)
  const watermarkRef = React.useRef("")

  const loadConversations = React.useCallback(async () => {
    try {
      const response = await fetchConciergeConversations()
      setConversations(response.conversations)
      setDetail((current) => {
        if (!current) return current
        const conversation = response.conversations.find(
          (item) => item.id === current.conversation.id
        )
        return conversation ? { ...current, conversation } : current
      })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取管家会话失败")
    } finally {
      setLoading(false)
    }
  }, [])

  React.useEffect(() => {
    const timer = window.setTimeout(() => void loadConversations(), 0)
    return () => window.clearTimeout(timer)
  }, [loadConversations, state?.updatedAt])

  React.useEffect(() => {
    if (loading || conversationId || conversations.length === 0) return
    navigate(`/workspace/${conversations[0].id}`, { replace: true })
  }, [conversationId, conversations, loading, navigate])

  React.useEffect(() => {
    if (!conversationId) {
      const timer = window.setTimeout(() => {
        setDetail(null)
        watermarkRef.current = ""
      }, 0)
      return () => window.clearTimeout(timer)
    }
    let cancelled = false
    void fetchConciergeConversation(conversationId)
      .then((next) => {
        if (cancelled) return
        watermarkRef.current = next.watermark
        setDetail(next)
      })
      .catch((error) => {
        if (!cancelled) {
          toast.error(error instanceof Error ? error.message : "读取对话失败")
          navigate("/workspace", { replace: true })
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [conversationId, navigate])

  React.useEffect(() => {
    if (!detail?.execution.id || !watermarkRef.current || !state?.updatedAt)
      return
    let cancelled = false
    const timer = window.setTimeout(() => {
      void fetchSessionDelta(detail.execution.id, watermarkRef.current)
        .then((delta) => {
          if (cancelled) return
          watermarkRef.current = delta.watermark
          setDetail((current) =>
            current
              ? {
                  ...current,
                  execution: delta.execution,
                  messages: mergeById(
                    current.messages,
                    delta.messages,
                    chronological
                  ),
                  messagesPage: {
                    ...current.messagesPage,
                    total:
                      current.messagesPage.total +
                      delta.messages.filter(
                        (message) =>
                          !current.messages.some(
                            (existing) => existing.id === message.id
                          )
                      ).length,
                  },
                  watermark: delta.watermark,
                }
              : current
          )
        })
        .catch(() => {
          if (!cancelled && conversationId) {
            void fetchConciergeConversation(conversationId).then((next) => {
              watermarkRef.current = next.watermark
              setDetail(next)
            })
          }
        })
    }, 200)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [conversationId, detail?.execution.id, state?.updatedAt])

  const newConversation = async () => {
    setCreating(true)
    try {
      const conversation = await createConciergeConversation()
      setConversations((current) => [conversation, ...current])
      navigate(`/workspace/${conversation.id}`)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "创建会话失败")
    } finally {
      setCreating(false)
    }
  }

  const send = async () => {
    const message = draft.trim()
    if (!message || sending) return
    let id = conversationId
    setSending(true)
    setDraft("")
    try {
      if (!id) {
        const conversation = await createConciergeConversation()
        id = conversation.id
        setConversations((current) => [conversation, ...current])
        navigate(`/workspace/${conversation.id}`)
      }
      await sendConciergeMessage(id, message)
      const next = await fetchConciergeConversation(id)
      watermarkRef.current = next.watermark
      setDetail(next)
      await loadConversations()
    } catch (error) {
      setDraft(message)
      toast.error(error instanceof Error ? error.message : "发送失败")
    } finally {
      setSending(false)
    }
  }

  const loadMore = async () => {
    if (!detail?.messagesPage.nextCursor || loadingMore) return
    setLoadingMore(true)
    try {
      const page = await fetchSessionMessages(
        detail.execution.id,
        detail.messagesPage.nextCursor
      )
      setDetail((current) =>
        current
          ? {
              ...current,
              messages: mergeById(page.items, current.messages, chronological),
              messagesPage: page.page,
            }
          : current
      )
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "加载消息失败")
    } finally {
      setLoadingMore(false)
    }
  }

  const removeConversation = async () => {
    if (!deleteTarget) return
    try {
      await deleteConciergeConversation(deleteTarget.id)
      setConversations((current) =>
        current.filter((item) => item.id !== deleteTarget.id)
      )
      if (conversationId === deleteTarget.id) navigate("/workspace")
      setDeleteTarget(null)
      toast.success("会话已删除")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除会话失败")
    }
  }

  const busy = ["queued", "starting", "running", "waiting_approval"].includes(
    detail?.execution.status ?? ""
  )

  return (
    <div className="flex size-full min-h-0 max-h-full overflow-hidden bg-background">
      <aside
        aria-hidden="true"
        className={cn(
          "hidden min-h-0 w-full shrink-0 flex-col border-r bg-muted/20 md:w-72"
        )}
      >
        <div className="flex h-14 shrink-0 items-center justify-between gap-3 px-4">
          <span className="font-medium">管家会话</span>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="新建会话"
            disabled={creating}
            onClick={() => void newConversation()}
          >
            {creating ? <Spinner /> : <Plus />}
          </Button>
        </div>
        <Separator />
        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col gap-1 p-2">
            {loading && conversations.length === 0 ? (
              <div className="flex justify-center py-10">
                <Spinner />
              </div>
            ) : conversations.length === 0 ? (
              <Empty className="border-0 py-12">
                <EmptyHeader>
                  <EmptyMedia variant="icon">
                    <MessageSquare />
                  </EmptyMedia>
                  <EmptyTitle>还没有会话</EmptyTitle>
                  <EmptyDescription>
                    新建会话，直接告诉管家你的需求。
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              conversations.map((conversation) => (
                <div
                  key={conversation.id}
                  className={cn(
                    "group/conversation flex items-center rounded-lg",
                    conversation.id === conversationId && "bg-secondary"
                  )}
                >
                  <Button
                    type="button"
                    variant="ghost"
                    className="h-auto min-w-0 flex-1 justify-start px-3 py-2.5 text-left hover:bg-transparent"
                    onClick={() => navigate(`/workspace/${conversation.id}`)}
                  >
                    <span className="flex min-w-0 flex-1 flex-col items-start gap-1">
                      <span className="w-full truncate text-sm font-medium">
                        {conversation.title}
                      </span>
                      <span className="w-full truncate text-xs text-muted-foreground">
                        {conversation.lastMessage || "开始一段新对话"}
                      </span>
                      <span className="text-[11px] text-muted-foreground">
                        {formatTime(conversation.updatedAt)}
                      </span>
                    </span>
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    className="mr-2 shrink-0 opacity-0 group-hover/conversation:opacity-100 focus-visible:opacity-100"
                    aria-label={`删除会话 ${conversation.title}`}
                    onClick={() => setDeleteTarget(conversation)}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))
            )}
          </div>
        </ScrollArea>
      </aside>

      <section
        className={cn(
          "grid h-full min-h-0 min-w-0 flex-1 grid-rows-[minmax(0,1fr)_auto]"
        )}
      >
        <div className="hidden">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="md:hidden"
            aria-label="返回会话列表"
            onClick={() => navigate("/workspace")}
          >
            <ArrowLeft />
          </Button>
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Sparkles className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">Aegis 管家</p>
            <p className="truncate text-xs text-muted-foreground">
              {busy ? "正在处理你的消息" : "可以直接回答，也可以创建并调度任务"}
            </p>
          </div>
          {busy ? (
            <Badge variant="secondary">
              <Spinner />
              思考中
            </Badge>
          ) : null}
          {detail?.conversation.createdTaskId ? (
            <Button
              variant="outline"
              size="sm"
              render={
                <Link to={`/tasks/${detail.conversation.createdTaskId}`} />
              }
              nativeButton={false}
            >
              查看任务
              <ExternalLink data-icon="inline-end" />
            </Button>
          ) : null}
        </div>

        {detail?.messages.length ? (
          <ConciergeConversationView
            messages={detail.messages}
            hasMore={detail.messagesPage.hasMore}
            loadingMore={loadingMore}
            onLoadMore={() => void loadMore()}
          />
        ) : (
          <div className="flex min-h-0 flex-1 items-center justify-center p-6">
            <Empty className="max-w-lg border-0">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Sparkles />
                </EmptyMedia>
                <EmptyTitle>有什么需要我处理？</EmptyTitle>
                <EmptyDescription>
                  你可以询问产品和任务情况，也可以直接描述要执行的工作。需求明确时，管家会创建真实任务并交给调度器。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </div>
        )}

        <div className="shrink-0 border-t bg-background p-3">
          <form
            className="mx-auto max-w-4xl"
            onSubmit={(event) => {
              event.preventDefault()
              void send()
            }}
          >
            <InputGroup className="min-h-24 items-stretch rounded-2xl bg-background">
              <InputGroupTextarea
                value={draft}
                maxLength={50000}
                aria-label="给管家发送消息"
                placeholder="告诉管家你的需求…"
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={(event) => {
                  if (
                    event.key === "Enter" &&
                    !event.shiftKey &&
                    !event.nativeEvent.isComposing
                  ) {
                    event.preventDefault()
                    void send()
                  }
                }}
                className="min-h-14 resize-none text-[13px]"
              />
              <InputGroupAddon align="block-end" className="justify-between">
                <span className="px-1 text-xs font-normal text-muted-foreground">
                  Enter 发送 · Shift+Enter 换行
                </span>
                <InputGroupButton
                  type="submit"
                  variant="default"
                  size="icon-sm"
                  aria-label="发送消息"
                  disabled={!draft.trim() || sending}
                >
                  {sending ? <Spinner /> : <Send />}
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
          </form>
        </div>
      </section>
      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除这个管家会话？</AlertDialogTitle>
            <AlertDialogDescription>
              对话消息和对应的 Pi Session
              记录将被删除；已经创建的任务不会受到影响。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>保留</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => void removeConversation()}
            >
              删除会话
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
