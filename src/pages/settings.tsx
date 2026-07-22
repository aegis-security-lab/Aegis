import * as React from "react"
import { Bot, Database, Gauge, HeartPulse, KeyRound, Radar, Save, ShieldCheck } from "lucide-react"
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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { probeRuntime, saveSettings, testConnection } from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { SaveConfigInput } from "@/types"

const providers = [
  "opencode-go",
  "anthropic",
  "openai",
  "google",
  "openrouter",
  "deepseek",
]
export function SettingsPage() {
  const { state, setState } = useAppState()
  const config = state!.config
  const [busy, setBusy] = React.useState<"save" | "probe" | "test" | null>(null)
  const [form, setForm] = React.useState<SaveConfigInput>(() =>
    fromConfig(config)
  )
  const [section, setSection] = React.useState<
    "runtime" | "model" | "workspace" | "budget" | "policy"
  >("runtime")
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
        description: "新配置将用于之后启动的 Pi sessions。",
      })
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存失败")
    } finally {
      setBusy(null)
    }
  }
  const probe = async () => {
    setBusy("probe")
    try {
      const result = await probeRuntime(form.nodePath, form.piPath)
      if (result.ready) toast.success(`Pi ${result.piVersion} 已就绪`)
      else toast.error(result.error)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "检测失败")
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
  return (
    <form onSubmit={save} className="flex flex-col gap-7">
      <PageHeader
        eyebrow="System"
        title="设置"
        description="配置 Pi runtime、Provider、默认工作区与全局安全策略。"
        actions={
          <Button type="submit" disabled={busy !== null}>
            {busy === "save" ? <Spinner /> : <Save />}保存设置
          </Button>
        }
      />
      <div className="grid items-start gap-5 md:grid-cols-[180px_minmax(0,1fr)]">
        <nav
          className="flex gap-2 overflow-x-auto rounded-xl border bg-card p-2 md:sticky md:top-20 md:flex-col"
          aria-label="设置分类"
        >
          {(
            [
              ["runtime", "Pi Runtime"],
              ["model", "模型与认证"],
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
              onClick={() => setSection(value)}
            >
              {label}
            </Button>
          ))}
        </nav>
        <div className="min-w-0">
          <div className="flex flex-col gap-5">
            <Card className={section === "runtime" ? undefined : "hidden"}>
              <CardHeader>
                <div className="flex items-center gap-3">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
                    <Radar className="size-4" />
                  </span>
                  <div>
                    <CardTitle>Pi Runtime</CardTitle>
                    <p className="mt-1 text-xs text-muted-foreground">
                      本机 RPC 进程入口
                    </p>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="settings-node">
                      Node.js 路径
                    </FieldLabel>
                    <Input
                      id="settings-node"
                      value={form.nodePath}
                      onChange={(event) =>
                        update("nodePath", event.target.value)
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="settings-pi">Pi CLI 路径</FieldLabel>
                    <Input
                      id="settings-pi"
                      value={form.piPath}
                      onChange={(event) => update("piPath", event.target.value)}
                    />
                  </Field>
                </FieldGroup>
              </CardContent>
              <CardFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={probe}
                  disabled={busy !== null}
                >
                  {busy === "probe" ? <Spinner /> : <Radar />}重新检测
                </Button>
              </CardFooter>
            </Card>
            <Card className={section === "model" ? undefined : "hidden"}>
              <CardHeader>
                <div className="flex items-center gap-3">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
                    <Bot className="size-4" />
                  </span>
                  <div>
                    <CardTitle>模型与认证</CardTitle>
                    <p className="mt-1 text-xs text-muted-foreground">
                      直接传递给 Pi CLI
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
                      placeholder="留空使用 Pi 内置 endpoint"
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
                          <SelectItem value="pi_auth">Pi 已有登录</SelectItem>
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
                    <CardTitle>Issue 执行预算</CardTitle>
                    <p className="mt-1 text-xs text-muted-foreground">
                      仅对根 Issue 生效；任一预算达到后会总结现有成果并放弃根目标
                    </p>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="settings-budget-tokens">
                      Token 预算
                    </FieldLabel>
                    <Input
                      id="settings-budget-tokens"
                      type="number"
                      min={1}
                      placeholder="留空表示不限制"
                      value={form.issueBudget.tokenLimit ?? ""}
                      onChange={(event) =>
                        update("issueBudget", {
                          ...form.issueBudget,
                          tokenLimit: event.target.value === "" ? null : Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      只统计根 Issue 当前这一次 Execution 的 Token；评论唤醒、返工、继续执行或手动重启产生新 Execution 后重新计数，子 Issue 不会单独触发预算。
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="settings-budget-cost">
                      价格成本预算（美元）
                    </FieldLabel>
                    <Input
                      id="settings-budget-cost"
                      type="number"
                      min={0.000001}
                      step="0.000001"
                      placeholder="留空表示不限制"
                      value={form.issueBudget.costLimit ?? ""}
                      onChange={(event) =>
                        update("issueBudget", {
                          ...form.issueBudget,
                          costLimit: event.target.value === "" ? null : Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      只统计根 Issue 当前这一次 Execution 按模型价格快照计算的成本；新 Execution 会重新计数。
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="settings-budget-time">
                      时间预算（分钟）
                    </FieldLabel>
                    <Input
                      id="settings-budget-time"
                      type="number"
                      min={1}
                      placeholder="留空表示不限制"
                      value={form.issueBudget.timeLimitMinutes ?? ""}
                      onChange={(event) =>
                        update("issueBudget", {
                          ...form.issueBudget,
                          timeLimitMinutes: event.target.value === "" ? null : Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      默认 10 分钟；只计算根 Issue 当前这一次 Execution 的运行时间。等待期间不会消耗，新 Execution 会获得完整的新预算。
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="settings-budget-poll">
                      预算轮询间隔（秒）
                    </FieldLabel>
                    <Input
                      id="settings-budget-poll"
                      type="number"
                      min={1}
                      max={3600}
                      value={form.issueBudget.checkIntervalSeconds}
                      onChange={(event) =>
                        update("issueBudget", {
                          ...form.issueBudget,
                          checkIntervalSeconds: Number(event.target.value),
                        })
                      }
                    />
                    <FieldDescription>
                      默认每 60 秒检查一次。三个预算都留空时不执行预算检查。
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
                      定期唤醒等待子树或未完成依赖的 Issue 负责人
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
                    <FieldLabel>Worker 并发</FieldLabel>
                    <Select
                      value={String(form.concurrency)}
                      onValueChange={(value) =>
                        update("concurrency", Number(value))
                      }
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {[1, 2, 3, 4, 6, 8].map((value) => (
                          <SelectItem key={value} value={String(value)}>
                            {value}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
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
                        max={50}
                        value={form.maxChildrenPerRequest}
                        onChange={(event) =>
                          update(
                            "maxChildrenPerRequest",
                            Number(event.target.value)
                          )
                        }
                      />
                      <FieldDescription>
                        一次工具调用允许创建 2 到该数量的子 Issue。
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
                        max={200}
                        value={form.maxDirectChildren}
                        onChange={(event) =>
                          update(
                            "maxDirectChildren",
                            Number(event.target.value)
                          )
                        }
                      />
                      <FieldDescription>
                        不得小于单次拆分上限，保存时会自动校正。
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
              <AlertTitle>Pi 没有内置沙箱</AlertTitle>
              <AlertDescription>
                审批扩展是操作门禁，不是系统级隔离。请只配置可信目录。
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
    nodePath: config.nodePath,
    piPath: config.piPath,
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
    maxChildrenPerRequest: config.maxChildrenPerRequest || 8,
    maxDirectChildren: config.maxDirectChildren || 16,
    issueBudget: config.issueBudget ?? {
      tokenLimit: null,
      costLimit: null,
      timeLimitMinutes: 10,
      checkIntervalSeconds: 60,
    },
    issueHeartbeat: config.issueHeartbeat ?? {
      intervalSeconds: 60,
    },
  }
}
