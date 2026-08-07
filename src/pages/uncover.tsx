import * as React from "react"
import {
  CheckCircle2,
  Database,
  Download,
  Eye,
  EyeOff,
  ExternalLink,
  KeyRound,
  Radar,
  Save,
  Search,
  Settings2,
  ShieldCheck,
  Trash2,
  TriangleAlert,
} from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import {
  deleteUncoverProvider,
  fetchUncoverStatus,
  saveUncoverProvider,
  searchUncover,
} from "@/lib/api"
import { cn } from "@/lib/utils"
import type {
  UncoverEngine,
  UncoverSearchInput,
  UncoverSearchResult,
  UncoverStatus,
} from "@/types"

const exportFormats: Array<{
  value: UncoverSearchInput["format"]
  label: string
  description: string
}> = [
  { value: "jsonl", label: "JSONL", description: "逐行处理" },
  { value: "json", label: "JSON", description: "结构化归档" },
  { value: "csv", label: "CSV", description: "表格分析" },
  { value: "txt", label: "TXT", description: "端点列表" },
]

const initialForm: UncoverSearchInput = {
  engine: "",
  query: "",
  limit: 100,
  format: "jsonl",
  field: "ip:port",
  timeout: 30,
}

export function UncoverPage() {
  const [status, setStatus] = React.useState<UncoverStatus | null>(null)
  const [form, setForm] = React.useState(initialForm)
  const [result, setResult] = React.useState<UncoverSearchResult | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [searching, setSearching] = React.useState(false)
  const [error, setError] = React.useState("")

  React.useEffect(() => {
    let active = true
    void fetchUncoverStatus()
      .then((next) => {
        if (!active) return
        setStatus(next)
        const first =
          next.engines.find((engine) => engine.configured) ?? next.engines[0]
        if (first) {
          setForm((current) => ({
            ...current,
            engine: current.engine || first.id,
            query: current.query || first.example,
          }))
        }
      })
      .catch((reason) => {
        if (active) {
          setError(
            reason instanceof Error ? reason.message : "读取引擎配置失败"
          )
        }
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  const selectedEngine = status?.engines.find(
    (engine) => engine.id === form.engine
  )
  const configuredCount =
    status?.engines.filter((engine) => engine.configured).length ?? 0

  const update = <K extends keyof UncoverSearchInput>(
    key: K,
    value: UncoverSearchInput[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const selectEngine = (value: string | null) => {
    const engine = status?.engines.find((item) => item.id === value)
    if (!engine) return
    setForm((current) => ({
      ...current,
      engine: engine.id,
      query:
        current.query.trim() === "" ||
        status?.engines.some((item) => item.example === current.query)
          ? engine.example
          : current.query,
    }))
    setError("")
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setError("")
    setSearching(true)
    try {
      setResult(await searchUncover(form))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "网络空间检索失败")
    } finally {
      setSearching(false)
    }
  }

  if (loading) return <UncoverSkeleton />

  return (
    <div className="flex flex-col gap-6">
      {error ? (
        <Alert variant="destructive">
          <TriangleAlert />
          <AlertTitle>检索没有完成</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="grid min-w-0 gap-6 xl:grid-cols-[minmax(340px,0.78fr)_minmax(0,1.22fr)]">
        <div className="flex min-w-0 flex-col gap-6">
          <Card>
            <CardHeader className="border-b">
              <div className="flex size-10 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-sm">
                <Radar className="size-5" />
              </div>
              <CardTitle className="mt-3">网络空间检索</CardTitle>
              <CardAction>
                <Badge variant="outline">uncover v1.2.1</Badge>
              </CardAction>
            </CardHeader>
            <CardContent>
              <form onSubmit={submit} className="flex flex-col gap-6">
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="uncover-engine">搜索引擎</FieldLabel>
                    <Select
                      items={(status?.engines ?? []).map((engine) => ({
                        label: engine.name,
                        value: engine.id,
                      }))}
                      value={form.engine || null}
                      onValueChange={selectEngine}
                    >
                      <SelectTrigger id="uncover-engine" className="w-full">
                        <SelectValue placeholder="选择引擎" />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {(status?.engines ?? []).map((engine) => (
                            <SelectItem key={engine.id} value={engine.id}>
                              <span className="flex w-full items-center gap-2">
                                <span>{engine.name}</span>
                                {engine.anonymous ? (
                                  <Badge variant="secondary">免密钥</Badge>
                                ) : engine.configured ? (
                                  <CheckCircle2 className="ml-auto size-3.5 text-emerald-600" />
                                ) : (
                                  <KeyRound className="ml-auto size-3.5 text-muted-foreground" />
                                )}
                              </span>
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FieldDescription className="flex flex-wrap items-center gap-2">
                      {selectedEngine?.configured
                        ? "凭据可用，可以直接搜索。"
                        : "该引擎还没有可用凭据。"}
                      {selectedEngine?.docsUrl ? (
                        <a
                          href={selectedEngine.docsUrl}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex items-center gap-1 text-foreground underline-offset-4 hover:underline"
                        >
                          官方语法
                          <ExternalLink className="size-3" />
                        </a>
                      ) : null}
                    </FieldDescription>
                  </Field>

                  <Field>
                    <FieldLabel htmlFor="uncover-query">
                      原生查询语法
                    </FieldLabel>
                    <Textarea
                      id="uncover-query"
                      value={form.query}
                      onChange={(event) => update("query", event.target.value)}
                      placeholder={selectedEngine?.example || "输入查询语法"}
                      className="min-h-32 resize-y font-mono text-sm leading-6"
                      spellCheck={false}
                      autoComplete="off"
                    />
                    <FieldDescription>
                      示例：
                      <code className="ml-1 break-all text-foreground">
                        {selectedEngine?.example || "—"}
                      </code>
                    </FieldDescription>
                  </Field>

                  <Field>
                    <FieldLabel>导出格式</FieldLabel>
                    <ToggleGroup
                      value={[form.format]}
                      onValueChange={(values) => {
                        const next = values[0] as
                          UncoverSearchInput["format"] | undefined
                        if (next) update("format", next)
                      }}
                      variant="outline"
                      spacing={0}
                      className="grid w-full grid-cols-4"
                    >
                      {exportFormats.map((format) => (
                        <ToggleGroupItem
                          key={format.value}
                          value={format.value}
                          className="h-auto min-w-0 flex-col gap-0.5 py-2"
                          aria-label={`导出为 ${format.label}`}
                        >
                          <span>{format.label}</span>
                          <span className="hidden text-[10px] font-normal text-muted-foreground sm:block">
                            {format.description}
                          </span>
                        </ToggleGroupItem>
                      ))}
                    </ToggleGroup>
                  </Field>

                  {form.format === "txt" ? (
                    <Field>
                      <FieldLabel htmlFor="uncover-field">
                        TXT 每行字段
                      </FieldLabel>
                      <Select
                        items={(status?.textFields ?? []).map((field) => ({
                          label: field,
                          value: field,
                        }))}
                        value={form.field}
                        onValueChange={(value) =>
                          update("field", value as UncoverSearchInput["field"])
                        }
                      >
                        <SelectTrigger id="uncover-field" className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            {(status?.textFields ?? []).map((field) => (
                              <SelectItem key={field} value={field}>
                                {field}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </Field>
                  ) : null}

                  <div className="grid gap-4 sm:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor="uncover-limit">结果上限</FieldLabel>
                      <Input
                        id="uncover-limit"
                        type="number"
                        min={1}
                        max={1000}
                        value={form.limit}
                        onChange={(event) =>
                          update("limit", Number(event.target.value))
                        }
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="uncover-timeout">
                        超时（秒）
                      </FieldLabel>
                      <Input
                        id="uncover-timeout"
                        type="number"
                        min={5}
                        max={120}
                        value={form.timeout}
                        onChange={(event) =>
                          update("timeout", Number(event.target.value))
                        }
                      />
                    </Field>
                  </div>
                </FieldGroup>

                <Button
                  type="submit"
                  size="lg"
                  disabled={
                    searching ||
                    !selectedEngine?.configured ||
                    form.query.trim() === ""
                  }
                  className="w-full"
                >
                  {searching ? (
                    <Spinner data-icon="inline-start" />
                  ) : (
                    <Search />
                  )}
                  {searching ? "正在查询索引…" : "开始检索"}
                </Button>
              </form>
            </CardContent>
          </Card>

          <ProviderStatusCard
            status={status}
            selectedEngine={selectedEngine}
            configuredCount={configuredCount}
            onStatusChange={setStatus}
          />
        </div>

        <ResultPanel result={result} selectedEngine={selectedEngine} />
      </div>
    </div>
  )
}

function ProviderStatusCard({
  status,
  selectedEngine,
  configuredCount,
  onStatusChange,
}: {
  status: UncoverStatus | null
  selectedEngine?: UncoverEngine
  configuredCount: number
  onStatusChange: (status: UncoverStatus) => void
}) {
  const [configuring, setConfiguring] = React.useState<UncoverEngine | null>(
    null
  )
  const credentialEngines =
    status?.engines.filter((engine) => !engine.anonymous) ?? []
  const configuredCredentialCount = credentialEngines.filter(
    (engine) => engine.configured
  ).length

  const replaceEngine = (engine: UncoverEngine) => {
    if (!status) return
    onStatusChange({
      ...status,
      engines: status.engines.map((item) =>
        item.id === engine.id ? engine : item
      ),
    })
  }

  return (
    <>
      <Card size="sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Database className="size-4 text-muted-foreground" />
            引擎凭据
          </CardTitle>
          <CardDescription>
            {configuredCredentialCount}/{credentialEngines.length} 个付费引擎已配置，
            共 {configuredCount} 个引擎可用。
          </CardDescription>
          <CardAction>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!selectedEngine || selectedEngine.anonymous}
              onClick={() => selectedEngine && setConfiguring(selectedEngine)}
            >
              <Settings2 />
              {selectedEngine?.anonymous
                ? "无需配置"
                : selectedEngine?.configured
                  ? "更新凭据"
                  : "配置当前引擎"}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="flex items-start gap-3 rounded-lg border bg-muted/30 px-3 py-3">
            {selectedEngine?.anonymous || selectedEngine?.configured ? (
              <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-600" />
            ) : (
              <KeyRound className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            )}
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                {selectedEngine?.name ?? "尚未选择引擎"}
              </p>
              <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                {selectedEngine?.anonymous
                  ? "该引擎使用公开匿名接口，不需要保存凭据。"
                  : selectedEngine?.configured
                    ? "凭据已安全保存在 Aegis 本地数据库中；API 只返回配置状态。"
                    : "点击配置当前引擎，在页面内保存所需凭据。"}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <ProviderConfigDialog
        engine={configuring}
        onOpenChange={(open) => !open && setConfiguring(null)}
        onUpdated={replaceEngine}
      />
    </>
  )
}

function ProviderConfigDialog({
  engine,
  onOpenChange,
  onUpdated,
}: {
  engine: UncoverEngine | null
  onOpenChange: (open: boolean) => void
  onUpdated: (engine: UncoverEngine) => void
}) {
  const [values, setValues] = React.useState<Record<string, string>>({})
  const [visible, setVisible] = React.useState<Record<string, boolean>>({})
  const [saving, setSaving] = React.useState(false)
  const [clearing, setClearing] = React.useState(false)
  const [confirmClear, setConfirmClear] = React.useState(false)

  if (!engine) return null

  const missingRequiredField = engine.credentialFields.some(
    (field) => !field.configured && !values[field.key]?.trim()
  )

  const save = async (event: React.FormEvent) => {
    event.preventDefault()
    setSaving(true)
    try {
      const updated = await saveUncoverProvider(engine.id, { values })
      onUpdated(updated)
      toast.success(`${engine.name} 凭据已保存`, {
        description: "之后的页面检索和 Agent 工具调用会立即使用新配置。",
      })
      onOpenChange(false)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存凭据失败")
    } finally {
      setSaving(false)
    }
  }

  const clear = async () => {
    setClearing(true)
    try {
      await deleteUncoverProvider(engine.id)
      onUpdated({
        ...engine,
        configured: false,
        credentialFields: engine.credentialFields.map((field) => ({
          ...field,
          configured: false,
          maskedValue: undefined,
        })),
      })
      toast.success(`${engine.name} 凭据已清除`)
      setConfirmClear(false)
      onOpenChange(false)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "清除凭据失败")
    } finally {
      setClearing(false)
    }
  }

  return (
    <>
      <Dialog open onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>配置 {engine.name}</DialogTitle>
            <DialogDescription>
              凭据保存在当前 Aegis 实例的本地数据库中。已保存的值不会回显，
              留空会保留原值。
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={save} className="flex flex-col gap-5">
            <FieldGroup>
              {engine.credentialFields.map((field) => {
                const inputID = `uncover-credential-${engine.id}-${field.key}`
                return (
                  <Field key={field.key}>
                    <FieldLabel htmlFor={inputID}>{field.label}</FieldLabel>
                    {field.secret ? (
                      <InputGroup>
                        <InputGroupInput
                          id={inputID}
                          type={visible[field.key] ? "text" : "password"}
                          value={values[field.key] ?? ""}
                          onChange={(event) =>
                            setValues((current) => ({
                              ...current,
                              [field.key]: event.target.value,
                            }))
                          }
                          placeholder={
                            field.configured
                              ? `${field.maskedValue ?? "••••••••"} 已保存；留空保留`
                              : `输入 ${field.label}`
                          }
                          autoComplete="new-password"
                          spellCheck={false}
                        />
                        <InputGroupAddon align="inline-end">
                          <InputGroupButton
                            size="icon-xs"
                            aria-label={
                              visible[field.key] ? "隐藏输入内容" : "显示输入内容"
                            }
                            onClick={() =>
                              setVisible((current) => ({
                                ...current,
                                [field.key]: !current[field.key],
                              }))
                            }
                          >
                            {visible[field.key] ? <EyeOff /> : <Eye />}
                          </InputGroupButton>
                        </InputGroupAddon>
                      </InputGroup>
                    ) : (
                      <Input
                        id={inputID}
                        value={values[field.key] ?? ""}
                        onChange={(event) =>
                          setValues((current) => ({
                            ...current,
                            [field.key]: event.target.value,
                          }))
                        }
                        placeholder={
                          field.configured
                            ? `${field.maskedValue ?? "••••••••"} 已保存；留空保留`
                            : `输入 ${field.label}`
                        }
                        autoComplete="off"
                        spellCheck={false}
                      />
                    )}
                    <FieldDescription>{field.description}</FieldDescription>
                  </Field>
                )
              })}
            </FieldGroup>

            <DialogFooter className="sm:justify-between">
              {engine.configured ? (
                <Button
                  type="button"
                  variant="ghost"
                  disabled={saving || clearing}
                  onClick={() => setConfirmClear(true)}
                >
                  <Trash2 />
                  清除配置
                </Button>
              ) : (
                <span />
              )}
              <div className="flex flex-col-reverse gap-2 sm:flex-row">
                <Button
                  type="button"
                  variant="outline"
                  disabled={saving || clearing}
                  onClick={() => onOpenChange(false)}
                >
                  取消
                </Button>
                <Button
                  type="submit"
                  disabled={saving || clearing || missingRequiredField}
                >
                  {saving ? <Spinner data-icon="inline-start" /> : <Save />}
                  {saving ? "正在保存…" : "保存凭据"}
                </Button>
              </div>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog open={confirmClear} onOpenChange={setConfirmClear}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>清除 {engine.name} 凭据？</AlertDialogTitle>
            <AlertDialogDescription>
              清除后，该引擎的页面检索和 Agent 工具调用都会停止工作，
              直到重新保存凭据。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={clearing}>返回</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={clearing}
              onClick={() => void clear()}
            >
              {clearing ? <Spinner data-icon="inline-start" /> : <Trash2 />}
              {clearing ? "正在清除…" : "确认清除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function ResultPanel({
  result,
  selectedEngine,
}: {
  result: UncoverSearchResult | null
  selectedEngine?: UncoverEngine
}) {
  if (!result) {
    return (
      <Card className="min-h-[640px]">
        <CardContent className="flex min-h-[600px] items-center justify-center">
          <Empty>
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <ShieldCheck />
              </EmptyMedia>
              <EmptyTitle>等待检索</EmptyTitle>
              <EmptyDescription className="max-w-md">
                查询结果会先归一化为来源、IP、端口、主机和
                URL；完整数据保存在可下载的导出文件中。
              </EmptyDescription>
            </EmptyHeader>
            {selectedEngine?.docsUrl ? (
              <Button
                variant="outline"
                nativeButton={false}
                render={
                  <a
                    href={selectedEngine.docsUrl}
                    target="_blank"
                    rel="noreferrer"
                  />
                }
              >
                <ExternalLink />
                阅读 {selectedEngine.name} 官方语法
              </Button>
            ) : null}
          </Empty>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className="min-w-0 self-start">
      <CardHeader className="border-b">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary">{result.engine}</Badge>
          <Badge variant="outline">{result.count} 条</Badge>
          <Badge variant="outline">{result.durationMs} ms</Badge>
        </div>
        <CardTitle className="mt-2">检索结果</CardTitle>
        <CardDescription className="line-clamp-2 font-mono break-all">
          {result.query}
        </CardDescription>
        <CardAction>
          <Button
            nativeButton={false}
            render={<a href={result.export.downloadUrl} download />}
          >
            <Download />
            导出 {result.export.format.toUpperCase()}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="flex min-w-0 flex-col gap-4">
        {result.warnings.map((warning) => (
          <Alert key={warning}>
            <TriangleAlert />
            <AlertTitle>引擎提示</AlertTitle>
            <AlertDescription>{warning}</AlertDescription>
          </Alert>
        ))}

        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm text-muted-foreground">
            {result.previewLimited
              ? `显示前 ${result.preview.length} 条，完整结果请下载导出文件。`
              : `已展示全部 ${result.preview.length} 条结果。`}
          </p>
          <span className="text-xs text-muted-foreground">
            {formatBytes(result.export.size)} · {result.export.name}
          </span>
        </div>

        {result.preview.length === 0 ? (
          <Empty className="min-h-80 border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Search />
              </EmptyMedia>
              <EmptyTitle>没有匹配结果</EmptyTitle>
              <EmptyDescription>
                查询执行成功，但所选引擎没有返回资产。检查字段、范围或账户权限后重试。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <ScrollArea className="h-[520px] rounded-lg border">
            <Table className="min-w-[760px]">
              <TableHeader className="sticky top-0 z-10 bg-background">
                <TableRow>
                  <TableHead className="w-24">来源</TableHead>
                  <TableHead>主机 / URL</TableHead>
                  <TableHead>IP</TableHead>
                  <TableHead className="w-20 text-right">端口</TableHead>
                  <TableHead className="w-36">观测时间</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {result.preview.map((asset, index) => (
                  <TableRow
                    key={`${asset.source}-${asset.ip}-${asset.port}-${asset.host}-${index}`}
                  >
                    <TableCell>
                      <Badge variant="outline">{asset.source}</Badge>
                    </TableCell>
                    <TableCell className="max-w-72">
                      <div className="flex min-w-0 flex-col gap-0.5">
                        <span
                          className={cn(
                            "truncate font-medium",
                            !asset.host &&
                              !asset.url &&
                              "font-normal text-muted-foreground"
                          )}
                          title={asset.url || asset.host}
                        >
                          {asset.url || asset.host || "—"}
                        </span>
                        {asset.url && asset.host ? (
                          <span className="truncate text-xs text-muted-foreground">
                            {asset.host}
                          </span>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {asset.ip || "—"}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs tabular-nums">
                      {asset.port || "—"}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {formatAssetTime(asset.timestamp)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <ScrollBar orientation="horizontal" />
          </ScrollArea>
        )}
      </CardContent>
    </Card>
  )
}

function UncoverSkeleton() {
  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(340px,0.78fr)_minmax(0,1.22fr)]">
      <div className="flex flex-col gap-4">
        <Skeleton className="h-[590px] rounded-xl" />
        <Skeleton className="h-36 rounded-xl" />
      </div>
      <Skeleton className="h-[640px] rounded-xl" />
    </div>
  )
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / (1024 * 1024)).toFixed(1)} MB`
}

function formatAssetTime(timestamp: number) {
  if (!timestamp) return "—"
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(timestamp * 1000))
}
