import { ChatComposer } from "@/components/chat-composer"
import { IssueMentionTextarea } from "@/components/issue-mention-input"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { useInputAttachments } from "@/hooks/use-input-attachments"
import {
  ISSUE_TITLE_MAX_LENGTH,
  issueTitleLength,
  limitIssueTitle,
} from "@/lib/issue-title"
import type { AgentDefinition, Issue } from "@/types"

type Attachments = ReturnType<typeof useInputAttachments>

const categoryLabels: Record<string, string> = {
  orchestrator: "调度",
  backend: "后端",
  frontend: "前端",
  design: "设计",
  security: "安全",
  general: "通用",
}

export function IssueBriefComposer({
  idPrefix,
  title,
  description,
  onTitleChange,
  onDescriptionChange,
  agents,
  selectedAgentId,
  onAgentChange,
  attachments,
  allowUnassigned = false,
  autoFocus = false,
  mentionIssues = [],
  objective,
  objectiveEnabled = false,
  onObjectiveChange,
  footerControls,
}: {
  idPrefix: string
  title: string
  description: string
  onTitleChange: (value: string) => void
  onDescriptionChange: (value: string) => void
  agents: AgentDefinition[]
  selectedAgentId: string
  onAgentChange: (value: string) => void
  attachments: Attachments
  allowUnassigned?: boolean
  autoFocus?: boolean
  mentionIssues?: Issue[]
  objective?: string
  objectiveEnabled?: boolean
  onObjectiveChange?: (value: string) => void
  footerControls?: React.ReactNode
}) {
  const selectItems = [
    ...(allowUnassigned ? [{ label: "未委派", value: "unassigned" }] : []),
    ...agents.map((agent) => ({
      label: `${agent.name} · ${categoryLabels[agent.category] ?? agent.category}`,
      value: agent.id,
    })),
  ]
  const selectValue = selectedAgentId || (allowUnassigned ? "unassigned" : "")

  return (
    <div className="min-w-0 bg-card">
      <div className="relative">
        <Input
          id={`${idPrefix}-title`}
          autoFocus={autoFocus}
          value={title}
          onChange={(event) =>
            onTitleChange(limitIssueTitle(event.target.value))
          }
          placeholder="标题（可选）"
          aria-label="标题"
          className="h-auto rounded-none border-0 bg-transparent px-1 pt-1 pr-16 pb-1 text-xl font-semibold tracking-tight shadow-none placeholder:font-medium focus-visible:ring-0 dark:bg-transparent"
        />
        <span className="pointer-events-none absolute top-2 right-1 text-[11px] text-muted-foreground tabular-nums">
          {issueTitleLength(title) || "—"} / {ISSUE_TITLE_MAX_LENGTH}
        </span>
      </div>
      <ChatComposer
        value={description}
        onValueChange={onDescriptionChange}
        onSend={() => {}}
        sending={false}
        showSend={false}
        embedded
        hint=""
        placeholder="描述目标、交付范围、业务规则和其他执行说明…"
        ariaLabel="任务说明"
        attachments={attachments}
        mentionIssues={mentionIssues}
        className="min-w-0"
        inputGroupClassName="min-h-40 rounded-none border-0 bg-transparent ring-0 focus-within:border-transparent focus-within:ring-0 has-[[data-slot=input-group-control]:focus-visible]:border-transparent has-[[data-slot=input-group-control]:focus-visible]:ring-0 dark:bg-transparent"
        textareaClassName="min-h-24 px-1 pt-2 text-sm leading-6"
        addonClassName="gap-2 px-0 pb-1"
        beforeAddon={
          objectiveEnabled && onObjectiveChange ? (
            <IssueMentionTextarea
              id={`${idPrefix}-objective`}
              value={objective ?? ""}
              onValueChange={onObjectiveChange}
              issues={mentionIssues}
              placeholder="描述可验证的完成标准、结果和质量要求…"
              aria-label="验收目标"
              className="min-h-16 resize-none rounded-none border-0 bg-transparent px-1 py-2 text-[13px] leading-5.5 font-medium tracking-[0.01em] text-foreground/80 shadow-none placeholder:font-normal placeholder:tracking-normal placeholder:text-muted-foreground focus-visible:ring-0 dark:bg-transparent"
            />
          ) : null
        }
        extra={
          <>
            <Select
              items={selectItems}
              value={selectValue}
              onValueChange={(value) =>
                onAgentChange(!value || value === "unassigned" ? "" : value)
              }
            >
              <SelectTrigger
                id={`${idPrefix}-agent`}
                size="sm"
                className="max-w-56 min-w-32 border-input bg-background shadow-xs *:data-[slot=select-value]:line-clamp-none"
                aria-label="选择 Agent 类型"
                aria-invalid={agents.length === 0 || undefined}
              >
                <SelectValue
                  placeholder="选择 Agent 类型"
                  className="max-w-[min(28rem,60vw)] min-w-0 [scrollbar-width:none] overflow-x-auto whitespace-nowrap [&::-webkit-scrollbar]:hidden"
                />
              </SelectTrigger>
              <SelectContent
                align="start"
                alignItemWithTrigger={false}
                className="w-max max-w-[min(36rem,calc(100vw-2rem))] min-w-(--anchor-width) overflow-x-auto"
              >
                <SelectGroup>
                  {allowUnassigned ? (
                    <SelectItem value="unassigned">未委派</SelectItem>
                  ) : null}
                  {agents.map((agent) => (
                    <SelectItem key={agent.id} value={agent.id}>
                      {agent.name} ·{" "}
                      {categoryLabels[agent.category] ?? agent.category}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            {footerControls}
          </>
        }
      />
    </div>
  )
}
