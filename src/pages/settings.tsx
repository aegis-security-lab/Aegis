import * as React from "react"
import { Bot, Database, KeyRound, Radar, Save, ShieldCheck } from "lucide-react"
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
      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="flex flex-col gap-5">
          <Card>
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
                  <FieldLabel htmlFor="settings-node">Node.js 路径</FieldLabel>
                  <Input
                    id="settings-node"
                    value={form.nodePath}
                    onChange={(event) => update("nodePath", event.target.value)}
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
          <Card>
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
                      onChange={(event) => update("model", event.target.value)}
                    />
                  </Field>
                </div>
                <Field>
                  <FieldLabel htmlFor="settings-base">Base URL</FieldLabel>
                  <Input
                    id="settings-base"
                    value={form.baseUrl}
                    onChange={(event) => update("baseUrl", event.target.value)}
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
                        update("authMode", value as SaveConfigInput["authMode"])
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
                      onChange={(event) => update("apiKey", event.target.value)}
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
          <Card>
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
                  每个 Agent 的工具、Skills 与权限边界请在 Agents 页面独立配置。
                </FieldDescription>
              </FieldGroup>
            </CardContent>
          </Card>
        </div>
        <div className="flex flex-col gap-5 xl:sticky xl:top-20">
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
                  <FieldLabel>自治审批</FieldLabel>
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
              </FieldGroup>
            </CardContent>
          </Card>
          <Alert>
            <Database />
            <AlertTitle>SQLite + GORM</AlertTitle>
            <AlertDescription>
              配置、任务、Issues、消息与事件保存在 data/aegis.db，数据库启用 WAL
              模式。
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
    approvalMode: config.approvalMode || "risky",
  }
}
