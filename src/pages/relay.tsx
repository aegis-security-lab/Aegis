import * as React from "react"
import { Inbox, MessageCircleMore, Plus, Send } from "lucide-react"
import { Link } from "react-router-dom"
import { toast } from "sonner"

import { MarkdownContent } from "@/components/markdown-content"
import { Badge } from "@/components/ui/badge"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
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
import { Message, MessageContent, MessageHeader } from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  fetchRelayConversation,
  fetchRelayInbox,
  sendRelayMessage,
} from "@/lib/api"
import { isSendMessageKey } from "@/lib/keyboard"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import type { RelayConversation, RelayThreadSummary } from "@/types"

export function RelayPage() {
  const { state } = useAppState()
  const employees = React.useMemo(
    () =>
      (state?.agents ?? []).filter(
        (agent) =>
          agent.enabled && !agent.internal && agent.category !== "concierge"
      ),
    [state?.agents]
  )
  const [selectedAgentId, setSelectedAgentId] = React.useState("")
  const agentId = selectedAgentId || employees[0]?.id || ""
  const [threads, setThreads] = React.useState<RelayThreadSummary[]>([])
  const [threadId, setThreadId] = React.useState("")
  const [conversation, setConversation] =
    React.useState<RelayConversation | null>(null)
  const [recipientId, setRecipientId] = React.useState("")
  const [draft, setDraft] = React.useState("")

  const loadInbox = React.useCallback(
    () =>
      agentId ? fetchRelayInbox(agentId) : Promise.resolve({ threads: [] }),
    [agentId]
  )
  React.useEffect(() => {
    let active = true
    void loadInbox()
      .then((response) => {
        if (!active) return
        setThreads(response.threads)
        if (!threadId && response.threads[0])
          setThreadId(response.threads[0].thread.id)
      })
      .catch((error) =>
        toast.error(
          error instanceof Error ? error.message : "读取 Relay 收件箱失败"
        )
      )
    return () => {
      active = false
    }
  }, [loadInbox, state?.updatedAt, threadId])
  React.useEffect(() => {
    if (!agentId || !threadId) return
    let active = true
    void fetchRelayConversation(agentId, threadId)
      .then((next) => {
        if (active) {
          setConversation(next)
          setRecipientId(
            next.thread.participantIds.find(
              (id) => id !== agentId && id !== "board" && id !== "operator"
            ) ?? ""
          )
        }
      })
      .catch((error) =>
        toast.error(
          error instanceof Error ? error.message : "读取 Relay 会话失败"
        )
      )
    return () => {
      active = false
    }
  }, [agentId, threadId, state?.updatedAt])

  const send = async () => {
    const body = draft.trim()
    if (!agentId || !recipientId || !body) return
    setDraft("")
    try {
      const message = await sendRelayMessage(agentId, recipientId, body)
      setThreadId(message.threadId)
      const response = await loadInbox()
      setThreads(response.threads)
    } catch (error) {
      setDraft(body)
      toast.error(
        error instanceof Error ? error.message : "发送 Relay 消息失败"
      )
    }
  }

  return (
    <div className="grid size-full min-h-0 grid-cols-[280px_minmax(0,1fr)] overflow-hidden">
      <aside className="flex min-h-0 flex-col border-r">
        <div className="flex h-14 shrink-0 items-center gap-2 px-3">
          <Select
            value={agentId}
            onValueChange={(value) => {
              setSelectedAgentId(String(value))
              setThreads([])
              setThreadId("")
              setConversation(null)
              setRecipientId("")
            }}
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="选择员工账号">
                {(value) =>
                  employees.find((employee) => employee.id === value)?.name ??
                  "选择员工账号"
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {employees.map((employee) => (
                  <SelectItem key={employee.id} value={employee.id}>
                    {employee.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            size="icon-sm"
            aria-label="新消息"
            onClick={() => {
              setThreadId("")
              setConversation(null)
              setRecipientId("")
            }}
          >
            <Plus />
          </Button>
        </div>
        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col gap-1 p-2">
            {threads.map((item) => {
              const counterpart =
                item.thread.participantIds.find((id) => id !== agentId) ||
                item.lastMessage?.senderId ||
                "Board"
              return (
                <Button
                  key={item.thread.id}
                  variant={threadId === item.thread.id ? "secondary" : "ghost"}
                  className="h-auto w-full justify-start px-3 py-3 text-left"
                  onClick={() => setThreadId(item.thread.id)}
                >
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center justify-between gap-2">
                      <span className="truncate font-medium">
                        {counterpart}
                      </span>
                      {item.unreadCount ? (
                        <Badge>{item.unreadCount}</Badge>
                      ) : null}
                    </span>
                    <span className="mt-1 block truncate text-xs font-normal text-muted-foreground">
                      {item.lastMessage?.body || "暂无消息"}
                    </span>
                  </span>
                </Button>
              )
            })}
          </div>
        </ScrollArea>
      </aside>
      <section className="flex min-h-0 min-w-0 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
          <MessageCircleMore />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold">
              {conversation
                ? conversation.thread.participantIds.find(
                    (id) => id !== agentId
                  ) || "Board"
                : "新消息"}
            </p>
            <p className="text-xs text-muted-foreground">
              Relay · 异步办公聊天
            </p>
          </div>
          {agentId ? (
            <Button
              variant="ghost"
              size="sm"
              render={<Link to={`/employees/${agentId}`} />}
            >
              打开员工工作台
            </Button>
          ) : null}
        </header>
        <div className="min-h-0 flex-1">
          {conversation ? (
            <MessageScrollerProvider defaultScrollPosition="last-anchor">
              <MessageScroller>
                <MessageScrollerViewport>
                  <MessageScrollerContent className="mx-auto w-full max-w-3xl gap-3 px-6 py-6">
                    {conversation.messages.map((item) => {
                      const own = item.senderId === agentId
                      return (
                        <MessageScrollerItem key={item.id} scrollAnchor>
                          <Message align={own ? "end" : "start"}>
                            <MessageContent>
                              <MessageHeader>
                                {item.senderId} · {formatTime(item.createdAt)}
                              </MessageHeader>
                              <Bubble
                                variant={own ? "secondary" : "outline"}
                                align={own ? "end" : "start"}
                              >
                                <BubbleContent>
                                  <MarkdownContent className="text-[13px] !leading-5">
                                    {item.body}
                                  </MarkdownContent>
                                  {item.issueId ? (
                                    <Button
                                      variant="link"
                                      className="mt-2 px-0"
                                      render={
                                        <Link to={`/issues/${item.issueId}`} />
                                      }
                                    >
                                      查看关联 Issue
                                    </Button>
                                  ) : null}
                                </BubbleContent>
                              </Bubble>
                            </MessageContent>
                          </Message>
                        </MessageScrollerItem>
                      )
                    })}
                  </MessageScrollerContent>
                </MessageScrollerViewport>
                <MessageScrollerButton />
              </MessageScroller>
            </MessageScrollerProvider>
          ) : (
            <Empty className="h-full">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Inbox />
                </EmptyMedia>
                <EmptyTitle>选择会话或发起新消息</EmptyTitle>
                <EmptyDescription>
                  消息会异步通知收件员工；发送者可以继续处理其他工作。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
        </div>
        <div className="shrink-0 px-5 pt-2 pb-4">
          <div className="mx-auto flex max-w-3xl flex-col gap-2">
            {!conversation ? (
              <Select
                value={recipientId}
                onValueChange={(value) => setRecipientId(String(value))}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="选择收件员工">
                    {(value) =>
                      employees.find((employee) => employee.id === value)
                        ?.name ?? "选择收件员工"
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {employees
                      .filter((employee) => employee.id !== agentId)
                      .map((employee) => (
                        <SelectItem key={employee.id} value={employee.id}>
                          {employee.name}
                        </SelectItem>
                      ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            ) : null}
            <InputGroup>
              <InputGroupTextarea
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={(event) => {
                  if (isSendMessageKey(event)) {
                    event.preventDefault()
                    void send()
                  }
                }}
                placeholder={
                  recipientId ? `给 ${recipientId} 发消息…` : "先选择收件员工"
                }
                disabled={!recipientId}
                className="min-h-20 resize-none"
              />
              <InputGroupAddon align="block-end" className="justify-end">
                <InputGroupButton
                  variant="default"
                  size="icon-sm"
                  disabled={!recipientId || !draft.trim()}
                  onClick={() => void send()}
                  aria-label="发送"
                >
                  <Send />
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
          </div>
        </div>
      </section>
    </div>
  )
}
