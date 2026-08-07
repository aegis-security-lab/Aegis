import * as React from "react"
import { useNavigate, useParams } from "react-router-dom"
import { toast } from "sonner"

import { ConciergeChatPanel } from "@/components/concierge-chat-panel"
import { ConciergeSessionDrawer } from "@/components/concierge-session-drawer"
import { useInputAttachments } from "@/hooks/use-input-attachments"
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
import {
  createConciergeConversation,
  deleteConciergeConversation,
  fetchConciergeConversation,
  fetchConciergeConversations,
  fetchSessionDelta,
  fetchSessionMessages,
  sendConciergeMessage,
  uploadConciergeAttachment,
} from "@/lib/api"
import { chronological, mergeById } from "@/lib/collections"
import { useAppState } from "@/lib/state"
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
  const attachmentOwnerRef = React.useRef(conversationId ?? "")

  const ensureConversation = React.useCallback(async () => {
    const existing = conversationId ?? attachmentOwnerRef.current
    if (existing) return existing
    const conversation = await createConciergeConversation()
    attachmentOwnerRef.current = conversation.id
    setConversations((current) => [conversation, ...current])
    navigate(`/workspace/${conversation.id}`)
    return conversation.id
  }, [conversationId, navigate])

  const uploadAttachment = React.useCallback(
    async (file: File, onProgress?: (progress: number) => void) => {
      const id = await ensureConversation()
      return uploadConciergeAttachment(id, file, onProgress)
    },
    [ensureConversation]
  )
  const attachments = useInputAttachments({
    scopeKey: "concierge-composer",
    upload: uploadAttachment,
  })
  const discardAttachments = attachments.discard

  React.useEffect(() => {
    const next = conversationId ?? ""
    if (attachmentOwnerRef.current !== next) {
      discardAttachments()
      attachmentOwnerRef.current = next
    }
  }, [conversationId, discardAttachments])

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
      attachments.discard()
      attachmentOwnerRef.current = conversation.id
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
    if ((!message && attachments.attachmentIds.length === 0) || sending) return
    setSending(true)
    setDraft("")
    try {
      const id = await ensureConversation()
      await sendConciergeMessage(id, message, attachments.attachmentIds)
      attachments.clearBound()
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

  return (
    <div className="relative flex size-full max-h-full min-h-0 overflow-hidden">
      <ConciergeSessionDrawer
        conversations={conversations}
        selectedId={conversationId}
        loading={loading}
        creating={creating}
        onSelect={(id) => navigate(`/workspace/${id}`)}
        onDelete={setDeleteTarget}
        onCreate={() => void newConversation()}
      />
      <ConciergeChatPanel
        detail={detail}
        draft={draft}
        onDraftChange={setDraft}
        sending={sending}
        loadingMore={loadingMore}
        onLoadMore={() => void loadMore()}
        onSend={() => void send()}
        attachments={attachments}
      />
      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除这个管家会话？</AlertDialogTitle>
            <AlertDialogDescription>
              对话消息和对应的 AgentCore Session
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
