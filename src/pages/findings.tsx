/* eslint-disable react-hooks/set-state-in-effect */
import * as React from "react"
import {
  AlertTriangle,
  Bug,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  FileSearch,
  RefreshCw,
  ShieldAlert,
  XCircle,
} from "lucide-react"
import { useSearchParams } from "react-router-dom"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
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
import { fetchFindings, updateFinding } from "@/lib/api"
import { cn } from "@/lib/utils"

import type { Finding } from "@/types"

/* ── constants ── */

const SEVERITY_ORDER = ["critical", "high", "medium", "low", "info"] as const
type Severity = (typeof SEVERITY_ORDER)[number]

const AUDIT_LEVEL_BY_SEVERITY: Record<Severity, string> = {
  critical: "A",
  high: "B",
  medium: "C",
  low: "E",
  info: "I",
}

const SEVERITY_LABEL: Record<Severity, string> = {
  critical: "严重",
  high: "高危",
  medium: "中危",
  low: "低危",
  info: "信息",
}

const SEVERITY_COLOR: Record<Severity, string> = {
  critical: "border-destructive/30 bg-destructive/15 text-destructive",
  high: "border-destructive/25 bg-destructive/10 text-destructive",
  medium: "border-warning/25 bg-warning/10 text-warning",
  low: "border-info/25 bg-info/10 text-info",
  info: "border-border bg-muted text-muted-foreground",
}

const SEVERITY_DOT: Record<Severity, string> = {
  critical: "bg-destructive",
  high: "bg-destructive/70",
  medium: "bg-warning",
  low: "bg-info",
  info: "bg-muted-foreground/40",
}

const CATEGORY_LABELS: Record<string, string> = {
  recon: "侦察",
  headers: "HTTP 安全头",
  tls: "TLS / SSL",
  cors: "CORS",
  cookie: "Cookie 安全",
  subdomain: "子域名",
  vuln: "漏洞",
}

const STATUS_TRANSITIONS: Record<
  string,
  {
    label: string
    target: string
    icon: typeof CheckCircle2
    variant: "default" | "secondary" | "destructive" | "outline" | "ghost"
  }[]
> = {
  open: [
    {
      label: "验证确认",
      target: "verified",
      icon: CheckCircle2,
      variant: "default",
    },
    {
      label: "误报",
      target: "false_positive",
      icon: XCircle,
      variant: "secondary",
    },
    {
      label: "关闭",
      target: "closed",
      icon: AlertTriangle,
      variant: "outline",
    },
  ],
  verified: [
    { label: "重新打开", target: "open", icon: Bug, variant: "secondary" },
    {
      label: "关闭",
      target: "closed",
      icon: AlertTriangle,
      variant: "outline",
    },
  ],
  false_positive: [
    { label: "重新打开", target: "open", icon: Bug, variant: "secondary" },
  ],
  closed: [
    { label: "重新打开", target: "open", icon: Bug, variant: "secondary" },
  ],
}

const STATUS_LABELS: Record<string, string> = {
  open: "待处理",
  verified: "已验证",
  false_positive: "误报",
  closed: "已关闭",
}

const STATUS_STYLES: Record<string, string> = {
  open: "border-warning/25 bg-warning/10 text-warning",
  verified: "border-success/25 bg-success/10 text-success",
  false_positive: "border-info/25 bg-info/10 text-info",
  closed: "border-border bg-muted text-muted-foreground",
}

const CATEGORIES = Object.keys(CATEGORY_LABELS)
const SEVERITIES = [...SEVERITY_ORDER]

function isSeverity(s: string): s is Severity {
  return SEVERITY_ORDER.includes(s as Severity)
}

/* ── helpers ── */

function formatEvidence(evidence: unknown): string {
  if (evidence === null || evidence === undefined) return ""
  try {
    return JSON.stringify(evidence, null, 2)
  } catch {
    return String(evidence)
  }
}

function evidenceLabel(evidence: unknown): string {
  if (evidence === null || evidence === undefined) return ""
  if (typeof evidence === "object") {
    if (Array.isArray(evidence)) return `数组 (${evidence.length} 项)`
    const keys = Object.keys(evidence as Record<string, unknown>)
    if (keys.length > 0) return `对象 (${keys.length} 字段)`
    return "空对象"
  }
  return String(evidence).substring(0, 60)
}

/* ── FindingsPage ── */

export function FindingsPage() {
  const [searchParams, setSearchParams] = useSearchParams()

  const categoryParam = searchParams.get("category") || ""
  const severityParam = searchParams.get("severity") || ""
  const pageParam = parseInt(searchParams.get("page") || "1", 10)

  const [data, setData] = React.useState<Finding[] | null>(null)
  const [totalPages, setTotalPages] = React.useState(0)
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<string | null>(null)
  const [expandedId, setExpandedId] = React.useState<string | null>(null)
  const [busyId, setBusyId] = React.useState<string | null>(null)

  const load = React.useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await fetchFindings({
        category: categoryParam || undefined,
        severity: severityParam || undefined,
        page: pageParam,
        pageSize: 50,
      })
      setData(result.findings)
      setTotalPages(Math.ceil(result.total / result.pageSize))
    } catch (e) {
      setError(e instanceof Error ? e.message : "获取安全发现失败")
      setData(null)
    } finally {
      setLoading(false)
    }
  }, [categoryParam, severityParam, pageParam])

  React.useEffect(() => {
    void load()
  }, [load])

  /* ── filter handlers ── */

  const setFilter = (key: string, value: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) {
        next.set(key, value)
      } else {
        next.delete(key)
      }
      // Reset to page 1 when filters change
      if (key !== "page") next.set("page", "1")
      return next
    })
  }

  /* ── status change ── */

  const changeStatus = async (finding: Finding, newStatus: string) => {
    setBusyId(finding.id)
    try {
      const updated = await updateFinding(finding.id, { status: newStatus })
      setData((prev) =>
        prev ? prev.map((f) => (f.id === updated.id ? updated : f)) : null
      )
      toast.success(
        `「${finding.title}」状态已更新为 ${STATUS_LABELS[newStatus] ?? newStatus}`
      )
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "状态更新失败")
    } finally {
      setBusyId(null)
    }
  }

  /* ── derived ── */

  const findings = data ?? []

  /* ── loading state ── */
  if (loading) {
    return (
      <div className="flex flex-col gap-7">
        <PageHeader eyebrow="Security" title="安全发现" />
        <div className="flex flex-col gap-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-16 w-full rounded-xl" />
          ))}
        </div>
      </div>
    )
  }

  /* ── error state ── */
  if (error) {
    return (
      <div className="flex flex-col gap-7">
        <PageHeader eyebrow="Security" title="安全发现" />
        <Card>
          <CardContent className="py-16">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <ShieldAlert className="text-destructive" />
                </EmptyMedia>
                <EmptyTitle>加载失败</EmptyTitle>
                <EmptyDescription>{error}</EmptyDescription>
              </EmptyHeader>
              <Button onClick={() => void load()}>
                <RefreshCw data-icon="inline-start" />
                重试
              </Button>
            </Empty>
          </CardContent>
        </Card>
      </div>
    )
  }

  /* ── empty state ── */
  if (findings.length === 0) {
    return (
      <div className="flex flex-col gap-7">
        <PageHeader eyebrow="Security" title="安全发现" />
        <div className="flex flex-wrap items-center gap-3">
          <FilterSelect
            label="类别"
            value={categoryParam}
            options={[
              { value: "", label: "全部类别" },
              ...CATEGORIES.map((c) => ({
                value: c,
                label: CATEGORY_LABELS[c] ?? c,
              })),
            ]}
            onChange={(v) => setFilter("category", v)}
          />
          <FilterSelect
            label="严重级别"
            value={severityParam}
            options={[
              { value: "", label: "全部级别" },
              ...SEVERITIES.map((s) => ({
                value: s,
                label: SEVERITY_LABEL[s],
              })),
            ]}
            onChange={(v) => setFilter("severity", v)}
          />
        </div>
        <Card>
          <CardContent className="py-16">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <FileSearch />
                </EmptyMedia>
                <EmptyTitle>未找到安全发现</EmptyTitle>
                <EmptyDescription>
                  {categoryParam || severityParam
                    ? "当前筛选条件下没有匹配的发现，请调整筛选条件。"
                    : "暂无安全发现，攻击面干净。"}
                </EmptyDescription>
              </EmptyHeader>
              {(categoryParam || severityParam) && (
                <Button variant="outline" onClick={() => setSearchParams({})}>
                  清除筛选
                </Button>
              )}
            </Empty>
          </CardContent>
        </Card>
      </div>
    )
  }

  /* ── success state ── */
  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Security"
        title="安全发现"
        actions={
          <Button variant="outline" size="sm" onClick={() => void load()}>
            <RefreshCw data-icon="inline-start" />
            刷新
          </Button>
        }
      />

      {/* ── Filter bar ── */}
      <div className="flex flex-wrap items-center gap-3">
        <FilterSelect
          label="类别"
          value={categoryParam}
          options={[
            { value: "", label: "全部类别" },
            ...CATEGORIES.map((c) => ({
              value: c,
              label: CATEGORY_LABELS[c] ?? c,
            })),
          ]}
          onChange={(v) => setFilter("category", v)}
        />
        <FilterSelect
          label="严重级别"
          value={severityParam}
          options={[
            { value: "", label: "全部级别" },
            ...SEVERITIES.map((s) => ({
              value: s,
              label: SEVERITY_LABEL[s],
            })),
          ]}
          onChange={(v) => setFilter("severity", v)}
        />
        {(categoryParam || severityParam) && (
          <Button variant="ghost" size="sm" onClick={() => setSearchParams({})}>
            清除筛选
          </Button>
        )}
        <div className="ml-auto text-xs text-muted-foreground">
          第 {pageParam} / {totalPages || 1} 页
        </div>
      </div>

      {/* ── Findings list ── */}
      <div className="flex flex-col gap-2">
        {findings.map((finding) => {
          const isExpanded = expandedId === finding.id
          const isBusy = busyId === finding.id
          return (
            <Card
              key={finding.id}
              size="sm"
              className={cn(
                "transition-shadow",
                isExpanded && "ring-1 ring-ring"
              )}
            >
              {/* ── Summary row (always visible) ── */}
              <button
                type="button"
                onClick={() => setExpandedId(isExpanded ? null : finding.id)}
                className="flex w-full items-start gap-3 px-(--card-spacing) py-3 text-left focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
                aria-expanded={isExpanded}
              >
                <span
                  className={cn(
                    "mt-1.5 size-2 shrink-0 rounded-full",
                    SEVERITY_DOT[
                      isSeverity(finding.severity) ? finding.severity : "info"
                    ]
                  )}
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">
                      {finding.title}
                    </span>
                    <AuditLevelBadge
                      auditLevel={finding.auditLevel}
                      severity={finding.severity}
                    />
                    <SeverityBadge severity={finding.severity} />
                  </div>
                  <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                    {finding.domain}
                    {finding.domain && finding.category ? " · " : ""}
                    {CATEGORY_LABELS[finding.category] ?? finding.category}
                    {evidenceLabel(finding.evidence)
                      ? ` · ${evidenceLabel(finding.evidence)}`
                      : ""}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <StatusBadge
                    status={finding.status}
                    className={STATUS_STYLES[finding.status]}
                  />
                  {isExpanded ? (
                    <ChevronDown className="size-4 text-muted-foreground" />
                  ) : (
                    <ChevronRight className="size-4 text-muted-foreground" />
                  )}
                </div>
              </button>

              {/* ── Expanded detail ── */}
              {isExpanded && (
                <div className="border-t px-(--card-spacing) pt-3 pb-4">
                  <div className="grid gap-5 lg:grid-cols-[1fr_1fr]">
                    {/* Left column */}
                    <div className="flex flex-col gap-5">
                      {/* Description */}
                      {finding.description && (
                        <DetailBlock
                          title="描述"
                          content={finding.description}
                        />
                      )}

                      {/* Evidence */}
                      {finding.evidence !== null &&
                        finding.evidence !== undefined && (
                          <DetailBlock title="证据">
                            <pre className="overflow-x-auto rounded-lg bg-muted p-4 font-mono text-xs leading-5">
                              <SyntaxHighlightJSON
                                json={formatEvidence(finding.evidence)}
                              />
                            </pre>
                          </DetailBlock>
                        )}

                      {/* Steps to reproduce */}
                      {finding.stepsToReproduce && (
                        <DetailBlock
                          title="复现步骤"
                          content={finding.stepsToReproduce}
                        />
                      )}
                    </div>

                    {/* Right column */}
                    <div className="flex flex-col gap-5">
                      {/* Remediation */}
                      {finding.remediation && (
                        <DetailBlock
                          title="修复建议"
                          content={finding.remediation}
                        />
                      )}

                      {/* Status transition buttons */}
                      <DetailBlock title="处置操作">
                        <div className="flex flex-wrap gap-2">
                          {(
                            STATUS_TRANSITIONS[finding.status] ??
                            STATUS_TRANSITIONS["open"]
                          ).map((action) => (
                            <Button
                              key={action.target}
                              variant={action.variant}
                              size="sm"
                              disabled={isBusy}
                              onClick={() =>
                                void changeStatus(finding, action.target)
                              }
                            >
                              {isBusy ? (
                                <Spinner className="size-3.5" />
                              ) : (
                                <action.icon className="size-3.5" />
                              )}
                              {action.label}
                            </Button>
                          ))}
                        </div>
                      </DetailBlock>

                      {/* Metadata */}
                      <div className="rounded-lg border p-3 text-xs text-muted-foreground">
                        <div className="flex items-center justify-between">
                          <span>发现时间</span>
                          <span className="tabular-nums">
                            {formatTime(finding.createdAt)}
                          </span>
                        </div>
                        <div className="mt-1 flex items-center justify-between">
                          <span>更新时间</span>
                          <span className="tabular-nums">
                            {formatTime(finding.updatedAt)}
                          </span>
                        </div>
                        <div className="mt-1 flex items-center justify-between">
                          <span>ID</span>
                          <span className="font-mono">{finding.id}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </Card>
          )
        })}
      </div>

      {/* ── Pagination ── */}
      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={pageParam <= 1}
            onClick={() => setFilter("page", String(pageParam - 1))}
          >
            上一页
          </Button>
          <span className="px-2 text-xs text-muted-foreground">
            {pageParam} / {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={pageParam >= totalPages}
            onClick={() => setFilter("page", String(pageParam + 1))}
          >
            下一页
          </Button>
        </div>
      )}
    </div>
  )
}

/* ── Sub-components ── */

function FilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string
  options: { value: string; label: string }[]
  onChange: (value: string) => void
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <Select value={value} onValueChange={(v) => onChange(v ?? "")}>
        <SelectTrigger className="h-7 w-36 text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {options.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

function AuditLevelBadge({
  auditLevel,
  severity,
}: {
  auditLevel?: string
  severity: string
}) {
  const sev = isSeverity(severity) ? severity : "info"
  const level = auditLevel || AUDIT_LEVEL_BY_SEVERITY[sev]

  return (
    <Badge variant="outline" className="font-semibold tabular-nums">
      {level}级
    </Badge>
  )
}

function SeverityBadge({ severity }: { severity: string }) {
  const sev = isSeverity(severity) ? severity : "info"
  return (
    <Badge variant="outline" className={cn("font-medium", SEVERITY_COLOR[sev])}>
      {SEVERITY_LABEL[sev]}
    </Badge>
  )
}

function DetailBlock({
  title,
  content,
  children,
}: {
  title: string
  content?: string
  children?: React.ReactNode
}) {
  return (
    <div>
      <h4 className="mb-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {title}
      </h4>
      {content ? (
        <p className="text-sm leading-6 whitespace-pre-wrap">{content}</p>
      ) : null}
      {children}
    </div>
  )
}

/* ── Simple JSON syntax highlighting ── */

function SyntaxHighlightJSON({ json }: { json: string }) {
  const highlighted = json
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/("(?:\\.|[^"\\])*")\s*:/g, '<span class="text-info">$1</span>:')
    .replace(
      /:\s*("(?:\\.|[^"\\])*")/g,
      ': <span class="text-success">$1</span>'
    )
    .replace(/:\s*(true|false)/g, ': <span class="text-warning">$1</span>')
    .replace(/:\s*(null)/g, ': <span class="text-muted-foreground">$1</span>')
    .replace(
      /:\s*(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
      ': <span class="text-primary-foreground">$1</span>'
    )

  return <span dangerouslySetInnerHTML={{ __html: highlighted }} />
}

/* ── time formatter (inline to avoid import cycle) ── */

function formatTime(value?: string) {
  if (!value) return "—"
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(value))
}
