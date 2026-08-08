/* eslint-disable react-hooks/set-state-in-effect */
import * as React from "react"
import {
  AlertTriangle,
  FileText,
  RefreshCw,
  Shield,
  ShieldAlert,
  Target,
  TriangleAlert,
} from "lucide-react"
import { Link } from "react-router-dom"

import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { fetchFindings } from "@/lib/api"
import { cn } from "@/lib/utils"

import type { Finding } from "@/types"

/* ── severity helpers ── */

const severityOrder = ["critical", "high", "medium", "low", "info"] as const
type Severity = (typeof severityOrder)[number]

const severityLabel: Record<Severity, string> = {
  critical: "严重",
  high: "高危",
  medium: "中危",
  low: "低危",
  info: "信息",
}

const severityColor: Record<Severity, string> = {
  critical: "bg-destructive/15 text-destructive",
  high: "bg-destructive/10 text-destructive",
  medium: "bg-warning/10 text-warning",
  low: "bg-info/10 text-info",
  info: "bg-muted text-muted-foreground",
}

const severityChartColor: Record<Severity, string> = {
  critical: "var(--color-chart-5)",
  high: "var(--color-destructive)",
  medium: "var(--color-warning)",
  low: "var(--color-info)",
  info: "var(--color-muted-foreground)",
}

const severityDot: Record<Severity, string> = {
  critical: "bg-destructive",
  high: "bg-destructive/70",
  medium: "bg-warning",
  low: "bg-info",
  info: "bg-muted-foreground/40",
}

// Weights for score calculation
const severityWeight: Record<Severity, number> = {
  critical: 25,
  high: 10,
  medium: 5,
  low: 2,
  info: 0,
}

function isSeverity(s: string): s is Severity {
  return severityOrder.includes(s as Severity)
}

function countBySeverity(findings: Finding[]): Record<Severity, number> {
  const counts: Record<string, number> = {}
  for (const f of findings) {
    const sev = isSeverity(f.severity) ? f.severity : "info"
    counts[sev] = (counts[sev] ?? 0) + 1
  }
  // Ensure all keys exist
  return Object.fromEntries(
    severityOrder.map((s) => [s, counts[s] ?? 0])
  ) as Record<Severity, number>
}

function calcScore(findings: Finding[]): number {
  if (findings.length === 0) return 100
  const totalDeduction = findings.reduce((sum, f) => {
    const sev = isSeverity(f.severity) ? f.severity : "info"
    return sum + severityWeight[sev]
  }, 0)
  return Math.max(0, Math.min(100, 100 - totalDeduction))
}

function scoreColor(score: number): string {
  if (score >= 80) return "text-success"
  if (score >= 50) return "text-warning"
  return "text-destructive"
}

/* ── category helpers ── */

type CategoryGroup = { category: string; count: number; severity: string }

function groupByCategory(findings: Finding[]): CategoryGroup[] {
  const map = new Map<string, { count: number; severity: string }>()
  for (const f of findings) {
    const existing = map.get(f.category) ?? { count: 0, severity: "info" }
    existing.count++
    // Keep the highest severity for display
    const sev = isSeverity(f.severity) ? f.severity : "info"
    if (
      severityOrder.indexOf(sev) <
      severityOrder.indexOf(existing.severity as Severity)
    ) {
      existing.severity = sev
    }
    map.set(f.category, existing)
  }
  return Array.from(map.entries())
    .map(([category, v]) => ({
      category,
      count: v.count,
      severity: v.severity,
    }))
    .sort((a, b) => b.count - a.count)
}

const categoryLabels: Record<string, string> = {
  recon: "侦察",
  dns: "DNS",
  headers: "HTTP 安全头",
  tls: "TLS / SSL",
  cors: "CORS",
  cookie: "Cookie 安全",
  subdomain: "子域名",
  vuln: "漏洞",
}

const threatModelLinks = [
  { label: "威胁建模指南", href: "#" },
  { label: "STRIDE 方法论", href: "#" },
  { label: "攻击树分析", href: "#" },
  { label: "风险评级矩阵", href: "#" },
]

function ThreatModelLink({ label, href }: { label: string; href: string }) {
  if (href.startsWith("http") || href.startsWith("#")) {
    return (
      <a
        href={href}
        className="inline-flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm text-muted-foreground hover:bg-muted/50 hover:text-foreground"
      >
        <Target className="size-3.5" />
        {label}
      </a>
    )
  }
  return (
    <Link
      to={href}
      className="inline-flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm text-muted-foreground hover:bg-muted/50 hover:text-foreground"
    >
      <Target className="size-3.5" />
      {label}
    </Link>
  )
}

/* ── Donut chart component ── */

function DonutChart({
  segments,
  size = 120,
}: {
  segments: { value: number; color: string }[]
  size?: number
}) {
  const total = segments.reduce((s, seg) => s + seg.value, 0)
  if (total === 0) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 36 36"
        className="-rotate-90"
      >
        <circle
          cx="18"
          cy="18"
          r="15.9"
          fill="none"
          stroke="var(--color-muted)"
          strokeWidth="3"
        />
      </svg>
    )
  }

  const strokeWidth = 3
  const radius = 15.9
  const circumference = 2 * Math.PI * radius

  const { arcs } = segments.reduce(
    (acc, seg) => {
      const proportion = seg.value / total
      const length = proportion * circumference
      const dasharray = `${length} ${circumference - length}`
      const dashoffset = -acc.cursor
      acc.arcs.push({ ...seg, dasharray, dashoffset })
      acc.cursor += length
      return acc
    },
    {
      arcs: [] as {
        value: number
        color: string
        dasharray: string
        dashoffset: number
      }[],
      cursor: 0,
    }
  )

  return (
    <svg width={size} height={size} viewBox="0 0 36 36" className="-rotate-90">
      <circle
        cx="18"
        cy="18"
        r={radius}
        fill="none"
        stroke="var(--color-muted)"
        strokeWidth={strokeWidth}
      />
      {arcs.map((arc, i) => (
        <circle
          key={i}
          cx="18"
          cy="18"
          r={radius}
          fill="none"
          stroke={arc.color}
          strokeWidth={strokeWidth}
          strokeDasharray={arc.dasharray}
          strokeDashoffset={arc.dashoffset}
          strokeLinecap="round"
          className="transition-[stroke-dasharray,stroke-dashoffset] duration-500"
        />
      ))}
    </svg>
  )
}

/* ── Main page ── */

export function SecurityPage() {
  const [data, setData] = React.useState<Finding[] | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<string | null>(null)

  const load = React.useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await fetchFindings({ pageSize: 100 })
      setData(result.findings)
    } catch (e) {
      setError(e instanceof Error ? e.message : "获取安全发现失败")
    } finally {
      setLoading(false)
    }
  }, [])

  React.useEffect(() => {
    void load()
  }, [load])

  /* ── derived data ── */
  const findings = data ?? []
  const total = findings.length
  const sevCounts = countBySeverity(findings)
  const score = calcScore(findings)
  const categories = groupByCategory(findings)

  // Latest 6 findings sorted by createdAt desc
  const latest = [...findings]
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
    .slice(0, 6)

  /* ── loading state ── */
  if (loading) {
    return (
      <div className="flex flex-col gap-7">
        <PageHeader eyebrow="Security" title="安全态势" />
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Card key={i}>
              <CardContent className="flex items-center gap-4 p-5">
                <Skeleton className="size-10 rounded-xl" />
                <div className="flex flex-col gap-1.5">
                  <Skeleton className="h-4 w-16" />
                  <Skeleton className="h-7 w-12" />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
        <div className="grid gap-5 lg:grid-cols-[1fr_1fr]">
          <Card>
            <CardHeader>
              <Skeleton className="h-5 w-32" />
            </CardHeader>
            <CardContent className="flex items-center justify-center gap-6 py-8">
              <Skeleton className="size-28 rounded-full" />
              <div className="flex flex-col gap-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-4 w-24" />
                ))}
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <Skeleton className="h-5 w-32" />
            </CardHeader>
            <CardContent>
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="mb-2 h-6 w-full" />
              ))}
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  /* ── error state ── */
  if (error) {
    return (
      <div className="flex flex-col gap-7">
        <PageHeader eyebrow="Security" title="安全态势" />
        <Card>
          <CardContent className="py-16">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <TriangleAlert className="text-destructive" />
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
        <PageHeader eyebrow="Security" title="安全态势" />
        <Card>
          <CardContent className="py-16">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Shield />
                </EmptyMedia>
                <EmptyTitle>暂无安全发现</EmptyTitle>
                <EmptyDescription>
                  未扫描出任何安全项，当前攻击面干净。
                </EmptyDescription>
              </EmptyHeader>
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
        title="安全态势"
        actions={
          <Button variant="outline" size="sm" onClick={() => void load()}>
            <RefreshCw data-icon="inline-start" />
            刷新
          </Button>
        }
      />

      {/* ── Metric cards ── */}
      <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          icon={ShieldAlert}
          label="安全评分"
          value={score}
          render={
            <span
              className={cn(
                "text-lg font-semibold tabular-nums",
                scoreColor(score)
              )}
            >
              {score}/100
            </span>
          }
        />
        <MetricCard icon={FileText} label="发现总数" value={total} />
        <MetricCard
          icon={AlertTriangle}
          label="严重 + 高危"
          value={sevCounts.critical + sevCounts.high}
        />
        <MetricCard icon={Target} label="涉及类别" value={categories.length} />
      </section>

      {/* ── Severity distribution + Category table ── */}
      <div className="grid gap-5 lg:grid-cols-[1fr_1.2fr]">
        {/* Severity donut */}
        <Card>
          <CardHeader>
            <CardTitle>严重级别分布</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col items-center gap-6 sm:flex-row sm:items-start">
            <DonutChart
              size={140}
              segments={severityOrder
                .filter((s) => sevCounts[s] > 0)
                .map((s) => ({
                  value: sevCounts[s],
                  color: severityChartColor[s],
                }))}
            />
            <div className="flex flex-col gap-1.5">
              {severityOrder.map((s) => {
                const count = sevCounts[s]
                if (count === 0 && s !== "critical") return null
                return (
                  <div key={s} className="flex items-center gap-2 text-sm">
                    <span
                      className={cn("size-2.5 rounded-full", severityDot[s])}
                    />
                    <span className="w-10 text-muted-foreground">
                      {severityLabel[s]}
                    </span>
                    <span className="font-medium tabular-nums">{count}</span>
                    <span className="text-xs text-muted-foreground">
                      ({total > 0 ? Math.round((count / total) * 100) : 0}%)
                    </span>
                  </div>
                )
              })}
            </div>
          </CardContent>
        </Card>

        {/* Category table */}
        <Card>
          <CardHeader>
            <CardTitle>按类别统计</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>类别</TableHead>
                  <TableHead className="text-right">数量</TableHead>
                  <TableHead className="text-right">最高严重级别</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {categories.map((cat) => (
                  <TableRow key={cat.category}>
                    <TableCell className="font-medium">
                      {categoryLabels[cat.category] ?? cat.category}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {cat.count}
                    </TableCell>
                    <TableCell className="text-right">
                      <SeverityBadge severity={cat.severity} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>

      {/* ── Latest findings + Threat model links ── */}
      <div className="grid gap-5 lg:grid-cols-[1.5fr_1fr]">
        {/* Latest findings */}
        <Card>
          <CardHeader>
            <CardTitle>最新发现</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 p-0">
            {latest.map((f) => (
              <div
                key={f.id}
                className="flex items-start gap-3 border-b px-(--card-spacing) py-3 last:border-b-0"
              >
                <span
                  className={cn(
                    "mt-0.5 size-2 shrink-0 rounded-full",
                    severityDotCls(f.severity)
                  )}
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">
                      {f.title}
                    </span>
                    <SeverityBadge severity={f.severity} />
                  </div>
                  <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                    {f.domain} — {categoryLabels[f.category] ?? f.category}
                  </p>
                </div>
                <StatusBadge status={f.status} className="shrink-0" />
              </div>
            ))}
          </CardContent>
        </Card>

        {/* Threat model quick links & score summary */}
        <div className="flex flex-col gap-5">
          <Card>
            <CardHeader>
              <CardTitle>安全评分详情</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              <div className="flex items-center gap-3">
                <span
                  className={cn(
                    "text-3xl font-bold tabular-nums",
                    scoreColor(score)
                  )}
                >
                  {score}
                </span>
                <span className="text-sm text-muted-foreground">/ 100</span>
              </div>
              <p className="text-xs leading-5 text-muted-foreground">
                评分依据严重级别加权计算。每项严重发现扣 25 分，高危 10 分，中危
                5 分，低危 2 分。
              </p>
              <div className="mt-1 rounded-lg bg-muted/50 p-3 text-xs leading-5 text-muted-foreground">
                <p>
                  严重: {sevCounts.critical} × 25 = {sevCounts.critical * 25}
                </p>
                <p>
                  高危: {sevCounts.high} × 10 = {sevCounts.high * 10}
                </p>
                <p>
                  中危: {sevCounts.medium} × 5 = {sevCounts.medium * 5}
                </p>
                <p>
                  低危: {sevCounts.low} × 2 = {sevCounts.low * 2}
                </p>
                <p className="mt-1 font-medium">
                  总扣分:{" "}
                  {sevCounts.critical * 25 +
                    sevCounts.high * 10 +
                    sevCounts.medium * 5 +
                    sevCounts.low * 2}
                </p>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>威胁模型</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-wrap gap-2">
              {threatModelLinks.map((link) => (
                <ThreatModelLink
                  key={link.label}
                  label={link.label}
                  href={link.href}
                />
              ))}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

/* ── Sub-components ── */

function MetricCard({
  icon: Icon,
  label,
  value,
  render,
}: {
  icon: typeof ShieldAlert
  label: string
  value: number | string
  render?: React.ReactNode
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 p-5">
        <span className="flex size-10 items-center justify-center rounded-xl bg-muted">
          <Icon className="size-4" />
        </span>
        <div>
          <p className="text-xs text-muted-foreground">{label}</p>
          {render ?? (
            <p className="text-xl font-semibold tabular-nums">{value}</p>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function SeverityBadge({ severity }: { severity: string }) {
  const sev = isSeverity(severity) ? severity : "info"
  return (
    <Badge variant="outline" className={cn("font-medium", severityColor[sev])}>
      {severityLabel[sev]}
    </Badge>
  )
}

function severityDotCls(severity: string): string {
  const sev = isSeverity(severity) ? severity : "info"
  return severityDot[sev]
}
