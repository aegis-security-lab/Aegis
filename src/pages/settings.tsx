import * as React from "react"
import {
  Bot,
  Database,
  ExternalLink,
  Gauge,
  Languages,
  HeartPulse,
  KeyRound,
  Radar,
  Save,
  Search,
  ShieldCheck,
} from "lucide-react"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { ModelPricingFields } from "@/components/model-pricing-fields"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { saveSettings, testConnection, testWebSearch } from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { SaveConfigInput, WebSearchInput, WebSearchResult } from "@/types"
import { cn } from "@/lib/utils"

const providers = [
  "opencode-go",
  "anthropic",
  "openai",
  "google",
  "openrouter",
  "deepseek",
]

export type SettingsSection =
  "general" | "search" | "runtime" | "model" | "workspace" | "budget" | "policy"

export function SettingsPage({
  embedded = false,
  section: controlledSection,
  onSectionChange,
}: {
  embedded?: boolean
  section?: SettingsSection
  onSectionChange?: (section: SettingsSection) => void
} = {}) {
  const { state, setState } = useAppState()
  const config = state!.config
  const [busy, setBusy] = React.useState<"save" | "test" | null>(null)
  const [searchBusy, setSearchBusy] = React.useState(false)
  const [searchResult, setSearchResult] =
    React.useState<WebSearchResult | null>(null)
  const [searchInput, setSearchInput] = React.useState<WebSearchInput>({
    query: "latest AI agent news",
    topic: "general",
    searchDepth: "basic",
    includeAnswer: true,
    maxResults: 5,
  })
  const [form, setForm] = React.useState<SaveConfigInput>(() =>
    fromConfig(config)
  )
  const [localSection, setLocalSection] =
    React.useState<SettingsSection>("general")
  const section = controlledSection ?? localSection
  const changeSection = (next: SettingsSection) => {
    if (onSectionChange) onSectionChange(next)
    else setLocalSection(next)
  }
  const update = <K extends keyof SaveConfigInput>(
    key: K,
    value: SaveConfigInput[K]
  ) => setForm((current) => ({ ...current, [key]: value }))
  const save = async (event: React.FormEvent) => {
    event.preventDefault()
    setBusy("save")
    try {
      const next = await saveSettings(form)
      setState(next)
      setForm(fromConfig(next.config))
      toast.success("设置已保存", {
        description: "新配置将用于之后启动的 AgentCore executions。",
      })
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存失败")
    } finally {
      setBusy(null)
    }
  }
  const test = async () => {
    setBusy("test")
    try {
      const result = await testConnection(form)
      if (result.ok)
        toast.success("真实模型连接成功", {
          description: `${result.reply} · ${result.durationMs} ms`,
        })
      else toast.error("连接失败", { description: result.error })
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "连接失败")
    } finally {
      setBusy(null)
    }
  }
  const runSearch = async () => {
    if (!searchInput.query.trim()) return
    setSearchBusy(true)
    try {
      setSearchResult(await testWebSearch(form.webSearch, searchInput))
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "检索失败")
    } finally {
      setSearchBusy(false)
    }
  }
  return (
    <form onSubmit={save} className="flex flex-col gap-7">
      <PageHeader
        eyebrow="System"
        title="设置"
        showIdentity={!embedded}
        actions={
          <Button type="submit" disabled={busy !== null}>
            {busy === "save" ? <Spinner /> : <Save />}保存设置
          </Button>
        }
      />
      <div
        className={cn(
          "items-start gap-5",
          embedded ? "flex flex-col" : "grid md:grid-cols-[180px_minmax(0,1fr)]"
        )}
      >
        {embedded ? null : (
          <nav
            className="flex gap-1 overflow-x-auto rounded-lg border bg-card p-1.5 md:sticky md:top-20 md:flex-col"
            aria-label="设置分类"
          >
            {(
              [
                ["general", "全局配置"],
                ["runtime", "AgentCore"],
                ["model", "模型与认证"],
                ["search", "Web 搜索"],
                ["workspace", "工作区"],
                ["budget", "预算与心跳"],
                ["policy", "执行与拆分策略"],
              ] as const
            ).map(([value, label]) => (
              <Button
                key={value}
                type="button"
                variant={section === value ? "secondary" : "ghost"}
                className="shrink-0 justify-start"
                onClick={() => changeSection(value)}
              >
                {label}
              </Button>
            ))}
          </nav>
        )}
        <div className="min-w-0">
          <div className="flex flex-col gap-5">
            <Card className={section === "general" ? undefined : "hidden"}>
              <CardHeader>
                <div className="flex items-center gap-3">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
                    <Languages className="size-4" />
                  </span>
                  <CardTitle>语言</CardTitle>
                </div>
              </CardHeader>
              <CardContent>
                <Field>
                  <FieldLabel>AI 输出语言</FieldLabel>
                  <Select
                    value={form.language}
                    onValueChange={(value) =>
                      update("language", value as SaveConfigInput["language"])
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="zh">中文</SelectItem>
                        <SelectItem value="en">English</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FieldDescription>
                    用于所有新启动 Agent 的文本回复、分析结论和报告。
                  </FieldDescription>
                </Field>
              </CardContent>
            </Card>
            <Card className={section === "runtime" ? undefined : "hidden"}>
              <CardHeader>
                <div className="flex items-center gap-3">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
                    <Radar className="size-4" />
                  </span>
                  <div>
                    <CardTitle>AgentCore Runtime</CardTitle>
                    <p className="mt-1 text-xs text-muted-foreground">
                      内嵌 Go 控制面
                    </p>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <Alert>
                  <ShieldCheck />
                  <AlertTitle>AgentCore 已内嵌</AlertTitle>
                  <AlertDescription>
                    Agent 循环、模型调用、工具策略和调度都在 Aegis
                    服务进程中运行；无需 Node.js 或外部 Agent
                    CLI。文件和命令工具通过任务专属 Docker 容器执行。
                  </AlertDescription>
                </Alert>
              </CardContent>
            </Card>
            <div
              className={
                section === "search"
                  ? "grid min-h-[560px] gap-5 xl:grid-cols-[180px_minmax(320px,0.9fr)_minmax(360px,1.1fr)]"
                  : "hidden"
              }
            >
              <Card>
                <CardHeader>
                  <CardTitle>搜索引擎</CardTitle>
                </CardHeader>
                <CardContent>
                  <Button
                    type="button"
                    variant="secondary"
                    className="w-full justify-start"
                  >
                    <Search data-icon="inline-start" />
                    Tavily
                  </Button>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>Tavily 配置</CardTitle>
                </CardHeader>
                <CardContent>
                  <FieldGroup>
                    <Field>
                      <FieldLabel>启用搜索工具</FieldLabel>
                      <ToggleGroup
                        value={form.webSearch.enabled ? ["enabled"] : []}
                        onValueChange={(value) =>
                          update("webSearch", {
                            ...form.webSearch,
                            enabled: value.includes("enabled"),
                          })
                        }
                        variant="outline"
                      >
                        <ToggleGroupItem value="enabled">启用</ToggleGroupItem>
                      </ToggleGroup>
                      <FieldDescription>
                        启用后，所有 Agent 都可以调用 aegis_web_search。
                      </FieldDescription>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="search-base-url">
                        API 地址
                      </FieldLabel>
                      <Input
                        id="search-base-url"
                        value={form.webSearch.baseUrl}
                        onChange={(event) =>
                          update("webSearch", {
                            ...form.webSearch,
                            baseUrl: event.target.value,
                          })
                        }
                        placeholder="https://example.com/api/tavily/search"
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="search-api-key">API Key</FieldLabel>
                      <Input
                        id="search-api-key"
                        type="password"
                        autoComplete="off"
                        value={form.webSearch.apiKey}
                        onChange={(event) =>
                          update("webSearch", {
                            ...form.webSearch,
                            apiKey: event.target.value,
                          })
                        }
                        placeholder={
                          config.webSearch?.hasApiKey
                            ? "已保存；留空保持不变"
                            : "th-…"
                        }
                      />
                      <FieldDescription>
                        密钥只保存在服务端，不会返回前端或暴露给 Agent。
                      </FieldDescription>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="search-query">测试关键词</FieldLabel>
                      <Input
                        id="search-query"
                        value={searchInput.query}
                        onChange={(event) =>
                          setSearchInput((current) => ({
                            ...current,
                            query: event.target.value,
                          }))
                        }
                        onKeyDown={(event) => {
                          if (event.key === "Enter") {
                            event.preventDefault()
                            void runSearch()
                          }
                        }}
                      />
                    </Field>
                    <div className="grid gap-4 sm:grid-cols-2">
                      <Field>
                        <FieldLabel>主题</FieldLabel>
                        <Select
                          value={searchInput.topic}
                          onValueChange={(value) =>
                            setSearchInput((current) => ({
                              ...current,
                              topic: value as WebSearchInput["topic"],
                            }))
                          }
                        >
                          <SelectTrigger className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectGroup>
                              <SelectItem value="general">通用</SelectItem>
                              <SelectItem value="news">新闻</SelectItem>
                              <SelectItem value="finance">金融</SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      </Field>
                      <Field>
                        <FieldLabel>深度</FieldLabel>
                        <Select
                          value={searchInput.searchDepth}
                          onValueChange={(value) =>
                            setSearchInput((current) => ({
                              ...current,
                              searchDepth:
                                value as WebSearchInput["searchDepth"],
                            }))
                          }
                        >
                          <SelectTrigger className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectGroup>
                              <SelectItem value="basic">Basic</SelectItem>
                              <SelectItem value="advanced">Advanced</SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      </Field>
                    </div>
                    <Button
                      type="button"
                      onClick={() => void runSearch()}
                      disabled={searchBusy || !searchInput.query.trim()}
                    >
                      {searchBusy ? (
                        <Spinner />
                      ) : (
                        <Search data-icon="inline-start" />
                      )}
                      检索
                    </Button>
                  </FieldGroup>
                </CardContent>
              </Card>
              <Card className="min-w-0">
                <CardHeader>
                  <div className="flex items-center justify-between gap-3">
                    <CardTitle>搜索结果</CardTitle>
                    {searchResult ? (
                      <Badge variant="secondary">
                        {searchResult.results.length} 条
                      </Badge>
                    ) : null}
                  </div>
                </CardHeader>
                <CardContent>
                  <ScrollArea className="h-[470px] pr-4">
                    {searchResult ? (
                      <div className="flex flex-col gap-4">
                        {searchResult.answer ? (
                          <Alert>
                            <Search />
                            <AlertTitle>综合答案</AlertTitle>
                            <AlertDescription>
                              {searchResult.answer}
                            </AlertDescription>
                          </Alert>
                        ) : null}
                        {searchResult.results.map((item, index) => (
                          <Card key={`${item.url}-${index}`} size="sm">
                            <CardHeader>
                              <CardTitle className="text-sm leading-snug">
                                <a
                                  href={item.url}
                                  target="_blank"
                                  rel="noreferrer"
                                  className="inline-flex items-start gap-1 hover:underline"
                                >
                                  {item.title || item.url}
                                  <ExternalLink />
                                </a>
                              </CardTitle>
                            </CardHeader>
                            <CardContent>
                              <p className="text-sm leading-relaxed text-muted-foreground">
                                {item.content}
                              </p>
                            </CardContent>
                          </Card>
                        ))}
                      </div>
                    ) : (
                      <div className="flex h-[430px] flex-col items-center justify-center gap-2 text-center text-muted-foreground">
                        <Search />
                        <p>配置引擎并执行一次测试检索</p>
                      </div>
                    )}
                  </ScrollArea>
                </CardContent>
              </Card>
            </div>
            <Card className={section === "model" ? undefined : "hidden"}>
              <CardHeader>
                <div className="flex items-center gap-3">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
                    <Bot className="size-4" />
                  </span>
                  <div>
                    <CardTitle>模型与认证</CardTitle>
                    <p className="mt-1 text-xs text-muted-foreground">
                      由 AgentCore Provider 直接连接
                    </p>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <div className="grid gap-5 sm:grid-cols-2">
                    <Field>
                      <FieldLabel>Provider</FieldLabel>
                      <Select
                        value={form.provider}
                        onValueChange={(value) =>
                          update("provider", String(value))
                        }
                      >
                        <SelectTrigger className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {providers.map((provider) => (
                            <SelectItem key={provider} value={provider}>
                              {provider}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="settings-model">模型 ID</FieldLabel>
                      <Input
                        id="settings-model"
                        value={form.model}
                        onChange={(event) =>
                          update("model", event.target.value)
                        }
                      />
                    </Field>
                  </div>
                  <Field>
                    <FieldLabel htmlFor="settings-base">Base URL</FieldLabel>
                    <Input
                      id="settings-base"
                      value={form.baseUrl}
                      onChange={(event) =>
                        update("baseUrl", event.target.value)
                      }
                      placeholder="留空使用 Provider 默认 endpoint"
                    />
                  </Field>
                  <ModelPricingFields
                    idPrefix="settings-price"
                    value={form.pricing}
                    onChange={(pricing) => update("pricing", pricing)}
                  />
                  <div className="grid gap-5 sm:grid-cols-2">
                    <Field>
                      <FieldLabel>思考强度</FieldLabel>
                      <Select
                        value={form.thinking}
                        onValueChange={(value) =>
                          update("thinking", String(value))
                        }
                      >
                        <SelectTrigger className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {["off", "low", "medium", "high", "xhigh"].map(
                            (value) => (
                              <SelectItem key={value} value={value}>
                                {value}
                              </SelectItem>
                            )
                          )}
                        </SelectContent>
                      </Select>
                    </Field>
                    <Field>
                      <FieldLabel>认证方式</FieldLabel>
                      <Select
                        value={form.authMode}
                        onValueChange={(value) =>
                          update(
                            "authMode",
                            value as SaveConfigInput["authMode"]
                          )
                        }
                      >
                        <SelectTrigger className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="api_key">API Key</SelectItem>
                          <SelectItem value="environment">环境变量</SelectItem>
                        </SelectContent>
                      </Select>
                    </Field>
                  </div>
                  {form.authMode === "api_key" ? (
                    <Field>
                      <FieldLabel htmlFor="settings-key">API Key</FieldLabel>
                      <Input
                        id="settings-key"
                        type="password"
                        autoComplete="off"
                        value={form.apiKey}
                        onChange={(event) =>
                          update("apiKey", event.target.value)
                        }
                        placeholder={
                          config.hasApiKey
                            ? "已保存；留空保持不变"
                            : "输入 API Key"
                        }
                      />
                      <FieldDescription>
                        状态 API 只返回“已配置”标记，不会回传密钥。
                      </FieldDescription>
                    </Field>
                  ) : null}
                </FieldGroup>
              </CardContent>
              <CardFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={test}
                  disabled={busy !== null}
                >
                  {busy === "test" ? <Spinner /> : <KeyRound />}测试真实连接
                </Button>
              </CardFooter>
            </Card>
            <Card className={section === "workspace" ? undefined : "hidden"}>
              <CardHeader>
                <CardTitle>默认工作区</CardTitle>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="settings-workspace">
                      默认工作目录
                    </FieldLabel>
                    <Input
                      id="settings-workspace"
                      value={form.workspace}
                      onChange={(event) =>
                        update("workspace", event.target.value)
                      }
                    />
                  </Field>
                  <FieldDescription>
                    每个 Agent 的工具、Skills 与权限边界请在 Agents
                    页面独立配置。
                  </FieldDescription>
                </FieldGroup>
              </CardContent>
            </Card>
            <Card className={section === "budget" ? undefined : "hidden"}>
              <CardHeader>
                <div className="flex items-center gap-3">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
                    <Gauge className="size-4" />
                  </span>
                  <div>
                    <CardTitle>子 Issue 单次 Execution 预算</CardTitle>
                    <p className="mt-1 text-xs text-muted-foreground">
                      每次重新执行都会获得新预算；Task
                      总时钟墙预算独立计算且不会重置
                    </p>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="settings-budget-turns">
                      正常工作轮数
                    </FieldLabel>
                    <Input
                      id="settings-budget-turns"
                      type="number"
                      min={1}
                      max={1000}
                      value={form.issueBudget.maxTurns}
                      onChange={(event) =>
                        update("issueBudget", {
                          ...form.issueBudget,
                          maxTurns: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      默认 100
                      轮。达到上限后不再继续正常工作，立即切换到受限总结阶段。
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="settings-budget-active-time">
                      正常工作时间（分钟）
                    </FieldLabel>
                    <Input
                      id="settings-budget-active-time"
                      type="number"
                      min={1}
                      max={1440}
                      value={form.issueBudget.activeTimeMinutes}
                      onChange={(event) =>
                        update("issueBudget", {
                          ...form.issueBudget,
                          activeTimeMinutes: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      默认 20 分钟，只计算当前 Execution；新 Execution
                      从零开始。
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="settings-budget-summary-turns">
                      总结最多轮数
                    </FieldLabel>
                    <Input
                      id="settings-budget-summary-turns"
                      type="number"
                      min={1}
                      max={100}
                      value={form.issueBudget.summaryTurns}
                      onChange={(event) =>
                        update("issueBudget", {
                          ...form.issueBudget,
                          summaryTurns: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      默认最多 10
                      轮。总结阶段禁用委派、写入和正常交付等扩展型工具。
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="settings-budget-summary-time">
                      总结时间上限（分钟）
                    </FieldLabel>
                    <Input
                      id="settings-budget-summary-time"
                      type="number"
                      min={1}
                      max={60}
                      value={form.issueBudget.summaryTimeMinutes}
                      onChange={(event) =>
                        update("issueBudget", {
                          ...form.issueBudget,
                          summaryTimeMinutes: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      默认 3 分钟；与总结轮数任一先达到即结束，并把 Issue
                      标记为超出预算。
                    </FieldDescription>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
            <Card className={section === "budget" ? undefined : "hidden"}>
              <CardHeader>
                <div className="flex items-center gap-3">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
                    <HeartPulse />
                  </span>
                  <div>
                    <CardTitle>等待 Issue 心跳</CardTitle>
                    <p className="mt-1 text-xs text-muted-foreground">
                      定期唤醒正在等待子树完成的 Issue 负责人
                    </p>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="settings-heartbeat-interval">
                      心跳间隔（秒）
                    </FieldLabel>
                    <Input
                      id="settings-heartbeat-interval"
                      type="number"
                      min={1}
                      max={3600}
                      value={form.issueHeartbeat.intervalSeconds}
                      onChange={(event) =>
                        update("issueHeartbeat", {
                          intervalSeconds: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      默认 60 秒。已有运行中 Execution 的 Issue 不会被重复唤醒。
                    </FieldDescription>
                  </Field>
                </FieldGroup>
              </CardContent>
            </Card>
          </div>
          <div
            className={
              section === "policy"
                ? "flex flex-col gap-5 xl:sticky xl:top-20"
                : "hidden"
            }
          >
            <Card>
              <CardHeader>
                <div className="flex items-center gap-3">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
                    <ShieldCheck className="size-4" />
                  </span>
                  <CardTitle>默认策略</CardTitle>
                </div>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="settings-worker-concurrency">
                      Worker 并发
                    </FieldLabel>
                    <Input
                      id="settings-worker-concurrency"
                      type="number"
                      min={1}
                      max={100}
                      value={form.concurrency}
                      onChange={(event) =>
                        update("concurrency", Number(event.target.value))
                      }
                    />
                    <FieldDescription>
                      允许同时运行 1–100 个 Worker，保存后立即生效。
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel>工具调用审批</FieldLabel>
                    <Select
                      value={form.approvalMode}
                      onValueChange={(value) =>
                        update(
                          "approvalMode",
                          value as SaveConfigInput["approvalMode"]
                        )
                      }
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="risky">仅高风险 shell</SelectItem>
                        <SelectItem value="all">所有写入与 shell</SelectItem>
                        <SelectItem value="none">不审批</SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel>Issue 返工审批</FieldLabel>
                    <Select
                      value={form.reworkApprovalMode}
                      onValueChange={(value) =>
                        update(
                          "reworkApprovalMode",
                          value as SaveConfigInput["reworkApprovalMode"]
                        )
                      }
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">始终需要人工批准</SelectItem>
                        <SelectItem value="none">自动批准并重新执行</SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel>目标验收策略</FieldLabel>
                    <ToggleGroup
                      value={[form.validationMode]}
                      onValueChange={(value) => {
                        if (value[0])
                          update(
                            "validationMode",
                            value[0] as SaveConfigInput["validationMode"]
                          )
                      }}
                      variant="outline"
                      className="w-full"
                      aria-label="选择目标验收策略"
                    >
                      <ToggleGroupItem className="flex-1" value="fixed">
                        固定次数
                      </ToggleGroupItem>
                      <ToggleGroupItem className="flex-1" value="automatic">
                        自动判断
                      </ToggleGroupItem>
                    </ToggleGroup>
                    <FieldDescription>
                      每个 Issue
                      使用一个固定验收会话，因此后续轮次能看到此前的验收结论。
                    </FieldDescription>
                  </Field>
                  <Field data-disabled={form.validationMode === "automatic"}>
                    <FieldLabel htmlFor="settings-validation-attempts">
                      最大常规验收次数
                    </FieldLabel>
                    <Input
                      id="settings-validation-attempts"
                      type="number"
                      min={1}
                      max={20}
                      disabled={form.validationMode === "automatic"}
                      value={form.maxValidationAttempts}
                      onChange={(event) =>
                        update(
                          "maxValidationAttempts",
                          Number(event.target.value)
                        )
                      }
                    />
                    <FieldDescription>
                      固定模式耗尽后再验收一次：可以通过、基于证明放弃，或转为人工处理。
                    </FieldDescription>
                  </Field>
                  <div className="grid gap-4 sm:grid-cols-3 xl:grid-cols-1">
                    <Field>
                      <FieldLabel htmlFor="settings-max-depth">
                        最大 Issue 层级
                      </FieldLabel>
                      <Input
                        id="settings-max-depth"
                        type="number"
                        min={1}
                        max={20}
                        value={form.maxIssueDepth}
                        onChange={(event) =>
                          update("maxIssueDepth", Number(event.target.value))
                        }
                      />
                      <FieldDescription>
                        根 Issue 深度为 0，默认最多拆分到第 4 层。
                      </FieldDescription>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="settings-max-per-request">
                        单次拆分子 Issue 数
                      </FieldLabel>
                      <Input
                        id="settings-max-per-request"
                        type="number"
                        min={2}
                        max={100}
                        value={form.maxChildrenPerRequest}
                        onChange={(event) =>
                          update(
                            "maxChildrenPerRequest",
                            Number(event.target.value)
                          )
                        }
                      />
                      <FieldDescription>
                        一次工具调用允许创建 2 到该数量的子 Issue，默认 100。
                      </FieldDescription>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="settings-max-direct">
                        直属子 Issue 上限
                      </FieldLabel>
                      <Input
                        id="settings-max-direct"
                        type="number"
                        min={2}
                        max={100}
                        value={form.maxDirectChildren}
                        onChange={(event) =>
                          update(
                            "maxDirectChildren",
                            Number(event.target.value)
                          )
                        }
                      />
                      <FieldDescription>
                        不得小于单次拆分上限，默认 100，保存后立即生效。
                      </FieldDescription>
                    </Field>
                  </div>
                </FieldGroup>
              </CardContent>
            </Card>
            <Alert>
              <Database />
              <AlertTitle>SQLite + GORM</AlertTitle>
              <AlertDescription>
                配置、任务、Issues、消息与事件保存在 data/aegis.db，数据库启用
                WAL 模式。
              </AlertDescription>
            </Alert>
            <Alert variant="destructive">
              <ShieldCheck />
              <AlertTitle>工具在任务容器中隔离</AlertTitle>
              <AlertDescription>
                AgentCore 位于控制面；Shell、文件与浏览器工具进入任务专属 Docker
                容器。仍请只配置可信目录并限制容器网络和资源。
              </AlertDescription>
            </Alert>
          </div>
        </div>
      </div>
    </form>
  )
}

function fromConfig(
  config: NonNullable<ReturnType<typeof useAppState>["state"]>["config"]
): SaveConfigInput {
  return {
    language: config.language || "zh",
    provider: config.provider,
    model: config.model,
    pricing: config.pricing,
    baseUrl: config.baseUrl,
    thinking: config.thinking,
    authMode: config.authMode || "api_key",
    apiKey: "",
    workspace: config.workspace,
    concurrency: config.concurrency || 3,
    approvalMode: config.approvalMode || "none",
    reworkApprovalMode: config.reworkApprovalMode || "none",
    validationMode: config.validationMode || "fixed",
    maxValidationAttempts: config.maxValidationAttempts || 3,
    maxIssueDepth: config.maxIssueDepth || 4,
    maxChildrenPerRequest: config.maxChildrenPerRequest || 100,
    maxDirectChildren: config.maxDirectChildren || 100,
    issueBudget: config.issueBudget ?? {
      maxTurns: 100,
      activeTimeMinutes: 20,
      summaryTurns: 10,
      summaryTimeMinutes: 3,
    },
    issueHeartbeat: config.issueHeartbeat ?? {
      intervalSeconds: 60,
    },
    webSearch: {
      engine: "tavily",
      baseUrl: config.webSearch?.baseUrl || "https://api.tavily.com/search",
      apiKey: "",
      enabled: config.webSearch?.enabled ?? false,
    },
  }
}
