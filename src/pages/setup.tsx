import * as React from "react"
import {
  Bot,
  Check,
  ChevronLeft,
  ChevronRight,
  FolderCode,
  KeyRound,
  Radar,
  ShieldCheck,
  Sparkles,
} from "lucide-react"
import { useNavigate } from "react-router-dom"
import { toast } from "sonner"

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
import { Checkbox } from "@/components/ui/checkbox"
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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { completeSetup, testConnection } from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { SaveConfigInput } from "@/types"

const steps = [
  { title: "欢迎", description: "了解 Aegis", icon: Sparkles },
  { title: "模型", description: "认证与推理", icon: KeyRound },
  { title: "工作区", description: "权限与执行", icon: FolderCode },
]

const modelDefaults: Record<string, string> = {
  anthropic: "claude-sonnet-5",
  openai: "gpt-5.4",
  google: "gemini-3.5-flash",
  openrouter: "anthropic/claude-sonnet-4.5",
  deepseek: "deepseek-chat",
  "opencode-go": "deepseek-v4-flash",
}

export function SetupPage() {
  const { setState } = useAppState()
  const navigate = useNavigate()
  const [step, setStep] = React.useState(0)
  const [busy, setBusy] = React.useState<"test" | "save" | null>(null)
  const [accepted, setAccepted] = React.useState(false)
  const [form, setForm] = React.useState<SaveConfigInput>(() => ({
    language: "zh",
    provider: "anthropic",
    model: modelDefaults.anthropic,
    pricing: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    baseUrl: "",
    thinking: "medium",
    authMode: "api_key",
    apiKey: "",
    workspace: "",
    concurrency: 3,
    approvalMode: "none",
    reworkApprovalMode: "none",
    validationMode: "fixed",
    maxValidationAttempts: 3,
    maxIssueDepth: 4,
    maxChildrenPerRequest: 100,
    maxDirectChildren: 100,
    issueBudget: {
      maxTurns: 100,
      activeTimeMinutes: 20,
      summaryTurns: 10,
      summaryTimeMinutes: 3,
    },
    issueHeartbeat: {
      intervalSeconds: 60,
    },
    webSearch: {
      engine: "tavily",
      baseUrl: "https://api.tavily.com/search",
      apiKey: "",
      enabled: false,
    },
  }))

  const update = <K extends keyof SaveConfigInput>(
    key: K,
    value: SaveConfigInput[K]
  ) => {
    setForm((current) => ({ ...current, [key]: value }))
  }

  const handleTest = async () => {
    setBusy("test")
    try {
      const result = await testConnection(form)
      if (result.ok) {
        toast.success("模型连接成功", {
          description: `${result.reply} · ${result.durationMs} ms`,
        })
      } else {
        toast.error("模型连接失败", { description: result.error })
      }
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "连接测试失败")
    } finally {
      setBusy(null)
    }
  }

  const handleComplete = async () => {
    setBusy("save")
    try {
      const next = await completeSetup(form)
      setState(next)
      toast.success("Aegis 初始化完成")
      navigate("/", { replace: true })
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存配置失败")
    } finally {
      setBusy(null)
    }
  }

  const nextDisabled =
    (step === 1 &&
      (!form.provider ||
        !form.model ||
        (form.authMode === "api_key" && !form.apiKey))) ||
    (step === 2 && (!form.workspace || !accepted))

  return (
    <div className="relative min-h-svh overflow-hidden bg-muted/30 px-4 py-8 sm:px-6 lg:py-12">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-72 bg-[radial-gradient(circle_at_top,oklch(0.8_0.09_202_/_0.22),transparent_65%)]" />
      <div className="relative mx-auto mb-8 flex max-w-5xl items-center gap-3">
        <span className="flex size-10 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-lg shadow-primary/15">
          <ShieldCheck className="size-5" />
        </span>
        <div>
          <p className="font-semibold tracking-tight">Aegis</p>
          <p className="text-xs text-muted-foreground">
            AgentCore Control Plane
          </p>
        </div>
      </div>
      <Card className="relative mx-auto grid max-w-5xl overflow-hidden p-0 shadow-xl shadow-foreground/5 lg:grid-cols-[260px_1fr]">
        <aside className="border-b bg-muted/45 p-6 lg:border-r lg:border-b-0 lg:p-8">
          <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            首次设置
          </p>
          <ol className="mt-5 grid grid-cols-4 gap-2 lg:grid-cols-1 lg:gap-1">
            {steps.map((item, index) => (
              <li key={item.title}>
                <button
                  type="button"
                  disabled={index > step}
                  onClick={() => index <= step && setStep(index)}
                  className={`flex w-full items-center gap-3 rounded-xl p-2.5 text-left transition-colors ${
                    step === index
                      ? "bg-background shadow-sm ring-1 ring-border"
                      : "text-muted-foreground"
                  }`}
                >
                  <span
                    className={`flex size-8 shrink-0 items-center justify-center rounded-lg ${
                      index < step
                        ? "bg-primary text-primary-foreground"
                        : step === index
                          ? "bg-primary/10 text-primary"
                          : "bg-muted text-muted-foreground"
                    }`}
                  >
                    {index < step ? (
                      <Check className="size-4" />
                    ) : (
                      <item.icon className="size-4" />
                    )}
                  </span>
                  <span className="hidden min-w-0 lg:block">
                    <span className="block text-sm font-medium text-foreground">
                      {item.title}
                    </span>
                    <span className="block truncate text-xs">
                      {item.description}
                    </span>
                  </span>
                </button>
              </li>
            ))}
          </ol>
        </aside>

        <div className="flex min-h-[590px] flex-col">
          <CardHeader className="border-b px-6 py-5 sm:px-8">
            <p className="text-xs text-muted-foreground">
              步骤 {step + 1} / {steps.length}
            </p>
            <CardTitle className="text-xl">{steps[step].title}</CardTitle>
          </CardHeader>
          <CardContent className="flex-1 px-6 py-7 sm:px-8">
            {step === 0 ? <WelcomeStep /> : null}
            {step === 1 ? (
              <ModelStep
                form={form}
                update={update}
                onTest={handleTest}
                busy={busy === "test"}
              />
            ) : null}
            {step === 2 ? (
              <WorkspaceStep
                form={form}
                update={update}
                accepted={accepted}
                setAccepted={setAccepted}
              />
            ) : null}
          </CardContent>
          <CardFooter className="justify-between border-t bg-muted/20 px-6 py-4 sm:px-8">
            <Button
              variant="ghost"
              disabled={step === 0 || busy !== null}
              onClick={() => setStep((value) => value - 1)}
            >
              <ChevronLeft />
              上一步
            </Button>
            {step < steps.length - 1 ? (
              <Button
                disabled={nextDisabled || busy !== null}
                onClick={() => setStep((value) => value + 1)}
              >
                继续
                <ChevronRight />
              </Button>
            ) : (
              <Button
                disabled={nextDisabled || busy !== null}
                onClick={handleComplete}
              >
                {busy === "save" ? <Spinner /> : <ShieldCheck />}
                完成初始化
              </Button>
            )}
          </CardFooter>
        </div>
      </Card>
    </div>
  )
}

function WelcomeStep() {
  return (
    <div className="flex h-full flex-col justify-center py-4">
      <div className="mb-7 flex size-14 items-center justify-center rounded-2xl bg-primary/10 text-primary">
        <Bot className="size-7" />
      </div>
      <h2 className="max-w-lg text-3xl font-semibold tracking-tight">
        让原生 AgentCore 在可观察、可干预的控制平面里工作。
      </h2>
      <p className="mt-4 max-w-xl text-sm leading-7 text-muted-foreground">
        Aegis 直接在服务进程中运行 Go AgentCore，把任务拆成 Issues，分配并发
        Worker，并通过任务专属 Docker 容器执行工具，实时呈现模型输出、调用、费用与审批。
      </p>
      <div className="mt-8 grid gap-3 sm:grid-cols-3">
        {[
          [Radar, "过程可见", "每个事件与工具调用实时回传"],
          [ShieldCheck, "操作可控", "高风险调用进入审批中心"],
          [FolderCode, "本地执行", "代码和会话留在你的机器"],
        ].map(([Icon, title, detail]) => {
          const Component = Icon as typeof Radar
          return (
            <div
              key={String(title)}
              className="rounded-xl border bg-muted/20 p-4"
            >
              <Component className="mb-3 size-4 text-primary" />
              <p className="text-sm font-medium">{String(title)}</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {String(detail)}
              </p>
            </div>
          )
        })}
      </div>
    </div>
  )
}

interface StepProps {
  form: SaveConfigInput
  update: <K extends keyof SaveConfigInput>(
    key: K,
    value: SaveConfigInput[K]
  ) => void
}

function ModelStep({
  form,
  update,
  onTest,
  busy,
}: StepProps & { onTest: () => void; busy: boolean }) {
  return (
    <div>
      <p className="mb-6 max-w-xl text-sm leading-6 text-muted-foreground">
        模型由 AgentCore Provider 适配器直接连接。连接测试会发出一次真实请求并可能产生少量费用。
      </p>
      <FieldGroup>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field>
            <FieldLabel>Provider</FieldLabel>
            <Select
              value={form.provider}
              onValueChange={(value) => {
                update("provider", value as string)
                update("model", modelDefaults[String(value)] ?? "")
              }}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.keys(modelDefaults).map((provider) => (
                  <SelectItem key={provider} value={provider}>
                    {provider}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="model">模型 ID</FieldLabel>
            <Input
              id="model"
              value={form.model}
              onChange={(event) => update("model", event.target.value)}
            />
          </Field>
        </div>
        <Field>
          <FieldLabel htmlFor="base-url">Base URL（可选）</FieldLabel>
          <Input
            id="base-url"
            value={form.baseUrl}
            onChange={(event) => update("baseUrl", event.target.value)}
            placeholder="https://api.example.com/v1"
          />
          <FieldDescription>
            留空使用 Provider 默认地址；填写后 AgentCore 会连接该 endpoint。
          </FieldDescription>
        </Field>
        <ModelPricingFields
          idPrefix="setup-price"
          value={form.pricing}
          onChange={(pricing) => update("pricing", pricing)}
        />
        <div className="grid gap-5 sm:grid-cols-2">
          <Field>
            <FieldLabel>思考强度</FieldLabel>
            <Select
              value={form.thinking}
              onValueChange={(value) => update("thinking", String(value))}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {["off", "low", "medium", "high", "xhigh"].map((level) => (
                  <SelectItem key={level} value={level}>
                    {level}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel>认证方式</FieldLabel>
            <Select
              value={form.authMode}
              onValueChange={(value) =>
                update("authMode", value as SaveConfigInput["authMode"])
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
            <FieldLabel htmlFor="api-key">API Key</FieldLabel>
            <Input
              id="api-key"
              type="password"
              autoComplete="off"
              value={form.apiKey}
              onChange={(event) => update("apiKey", event.target.value)}
              placeholder="密钥仅写入本机 SQLite"
            />
            <FieldDescription>
              密钥不会通过状态 API 返回，由 AgentCore Provider 在服务进程内读取。
            </FieldDescription>
          </Field>
        ) : null}
      </FieldGroup>
      <Button
        className="mt-6"
        variant="outline"
        onClick={onTest}
        disabled={
          busy || !form.model || (form.authMode === "api_key" && !form.apiKey)
        }
      >
        {busy ? <Spinner /> : <Bot />}测试真实模型连接
      </Button>
    </div>
  )
}

function WorkspaceStep({
  form,
  update,
  accepted,
  setAccepted,
}: StepProps & { accepted: boolean; setAccepted: (value: boolean) => void }) {
  return (
    <div>
      <p className="mb-6 max-w-xl text-sm leading-6 text-muted-foreground">
        AgentCore 控制面运行在 Aegis 服务中，文件与命令工具在任务专属 Docker
        容器中执行。请限制可信工作区并选择工具审批级别。
      </p>
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="workspace">默认工作目录</FieldLabel>
          <Input
            id="workspace"
            value={form.workspace}
            onChange={(event) => update("workspace", event.target.value)}
            placeholder="/Users/you/Code/project"
          />
        </Field>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="setup-worker-concurrency">默认并发 Worker</FieldLabel>
            <Input
              id="setup-worker-concurrency"
              type="number"
              min={1}
              max={100}
              value={form.concurrency}
              onChange={(event) => update("concurrency", Number(event.target.value))}
            />
            <FieldDescription>可配置 1–100 个并发 Worker。</FieldDescription>
          </Field>
          <Field>
            <FieldLabel>工具调用审批策略</FieldLabel>
            <Select
              value={form.approvalMode}
              onValueChange={(value) =>
                update("approvalMode", value as SaveConfigInput["approvalMode"])
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
            <FieldLabel>Issue 返工审批策略</FieldLabel>
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
              aria-label="选择目标验收策略"
            >
              <ToggleGroupItem value="fixed">固定次数</ToggleGroupItem>
              <ToggleGroupItem value="automatic">自动判断</ToggleGroupItem>
            </ToggleGroup>
            <FieldDescription>
              每个 Issue 始终使用一个固定验收会话。
            </FieldDescription>
          </Field>
          <Field data-disabled={form.validationMode === "automatic"}>
            <FieldLabel htmlFor="setup-validation-attempts">
              最大常规验收次数
            </FieldLabel>
            <Input
              id="setup-validation-attempts"
              type="number"
              min={1}
              max={20}
              disabled={form.validationMode === "automatic"}
              value={form.maxValidationAttempts}
              onChange={(event) =>
                update("maxValidationAttempts", Number(event.target.value))
              }
            />
            <FieldDescription>
              固定模式耗尽后再进行一次终局验收，决定通过、证明后放弃或转人工。
            </FieldDescription>
          </Field>
        </div>
        <Alert>
          <ShieldCheck />
          <AlertTitle>安全说明</AlertTitle>
          <AlertDescription>
            “引导模式”任务始终审批所有写入和
            shell；“自治模式”使用上面的默认策略。Docker 容器提供任务级执行隔离。
          </AlertDescription>
        </Alert>
        <Field orientation="horizontal">
          <Checkbox
            id="accept-risk"
            checked={accepted}
            onCheckedChange={setAccepted}
          />
          <FieldLabel htmlFor="accept-risk" className="font-normal">
            我了解 AgentCore 会通过任务容器执行工具，并确认只把可信工作目录交给 Aegis。
          </FieldLabel>
        </Field>
      </FieldGroup>
    </div>
  )
}
