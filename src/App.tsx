import * as React from "react"
import { Navigate, Route, Routes, useLocation } from "react-router-dom"

import { AppShell } from "@/components/app-shell"
import { ErrorBoundary } from "@/components/error-boundary"
import { PageSkeleton } from "@/components/page-skeleton"
import { Button } from "@/components/ui/button"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { useAppState } from "@/lib/state"
const AgentsPage = React.lazy(() =>
  import("@/pages/agents").then((module) => ({ default: module.AgentsPage }))
)
const ApprovalsPage = React.lazy(() =>
  import("@/pages/approvals").then((module) => ({
    default: module.ApprovalsPage,
  }))
)
const ContainersPage = React.lazy(() =>
  import("@/pages/containers").then((module) => ({
    default: module.ContainersPage,
  }))
)
const DashboardPage = React.lazy(() =>
  import("@/pages/dashboard").then((module) => ({
    default: module.DashboardPage,
  }))
)
const IssuesPage = React.lazy(() =>
  import("@/pages/issues").then((module) => ({ default: module.IssuesPage }))
)
const IssueDetailPage = React.lazy(() =>
  import("@/pages/board-issue-detail").then((module) => ({
    default: module.BoardIssueDetailPage,
  }))
)
const SettingsPage = React.lazy(() =>
  import("@/pages/settings").then((module) => ({
    default: module.SettingsPage,
  }))
)
const SessionsPage = React.lazy(() =>
  import("@/pages/sessions").then((module) => ({
    default: module.SessionsPage,
  }))
)
const SessionDetailPage = React.lazy(() =>
  import("@/pages/session-detail").then((module) => ({
    default: module.SessionDetailPage,
  }))
)
const FindingsPage = React.lazy(() =>
  import("@/pages/findings").then((module) => ({
    default: module.FindingsPage,
  }))
)
const SecurityPage = React.lazy(() =>
  import("@/pages/security").then((module) => ({
    default: module.SecurityPage,
  }))
)
const UncoverPage = React.lazy(() =>
  import("@/pages/uncover").then((module) => ({
    default: module.UncoverPage,
  }))
)
const SkillsPage = React.lazy(() =>
  import("@/pages/skills").then((module) => ({ default: module.SkillsPage }))
)
const CapabilitiesPage = React.lazy(() =>
  import("@/pages/capabilities").then((module) => ({
    default: module.CapabilitiesPage,
  }))
)
const KnowledgeBasesPage = React.lazy(() =>
  import("@/pages/knowledge-bases").then((module) => ({
    default: module.KnowledgeBasesPage,
  }))
)
const KnowledgeBaseDetailPage = React.lazy(() =>
  import("@/pages/knowledge-base-detail").then((module) => ({
    default: module.KnowledgeBaseDetailPage,
  }))
)
const SetupPage = React.lazy(() =>
  import("@/pages/setup").then((module) => ({ default: module.SetupPage }))
)
const TaskNewPage = React.lazy(() =>
  import("@/pages/task-new").then((module) => ({ default: module.TaskNewPage }))
)
const TaskDetailPage = React.lazy(() =>
  import("@/pages/task-detail").then((module) => ({
    default: module.TaskDetailPage,
  }))
)
const TaskBoardPage = React.lazy(() =>
  import("@/pages/task-board").then((module) => ({
    default: module.TaskBoardPage,
  }))
)
const TasksPage = React.lazy(() =>
  import("@/pages/tasks").then((module) => ({ default: module.TasksPage }))
)
const TimelinePage = React.lazy(() =>
  import("@/pages/timeline").then((module) => ({
    default: module.TimelinePage,
  }))
)
const WorkspaceChatPage = React.lazy(() =>
  import("@/pages/workspace-chat").then((module) => ({
    default: module.WorkspaceChatPage,
  }))
)

export function App() {
  const { state, loading, error, refresh } = useAppState()
  const location = useLocation()
  if (loading || !state) {
    return <AppBootState error={error} onRetry={() => void refresh()} />
  }
  if (!state.configured) {
    return (
      <ErrorBoundary resetKey={location.pathname}>
        <React.Suspense fallback={<PageSkeleton />}>
          <Routes>
            <Route path="/setup" element={<SetupPage />} />
            <Route path="*" element={<Navigate to="/setup" replace />} />
          </Routes>
        </React.Suspense>
      </ErrorBoundary>
    )
  }
  return (
    <ErrorBoundary resetKey={location.pathname}>
      <React.Suspense fallback={<PageSkeleton />}>
        <Routes>
          <Route path="/setup" element={<Navigate to="/" replace />} />
          <Route element={<AppShell />}>
            <Route index element={<DashboardPage />} />
            <Route path="workspace" element={<WorkspaceChatPage />} />
            <Route
              path="workspace/:conversationId"
              element={<WorkspaceChatPage />}
            />
            <Route path="tasks" element={<TasksPage />} />
            <Route path="tasks/new" element={<TaskNewPage />} />
            <Route path="tasks/:taskId/issues" element={<IssuesPage />} />
            <Route path="tasks/:taskId/board" element={<TaskBoardPage />} />
            <Route path="tasks/:taskId" element={<TaskDetailPage />} />
            <Route path="timeline" element={<TimelinePage />} />
            <Route path="issues" element={<IssuesPage />} />
            <Route path="issues/:issueId" element={<IssueDetailPage />} />
            <Route path="agents" element={<AgentsPage />} />
            <Route path="skills" element={<SkillsPage />} />
            <Route path="capabilities" element={<CapabilitiesPage />} />
            <Route path="knowledge-bases" element={<KnowledgeBasesPage />} />
            <Route
              path="knowledge-bases/:knowledgeBaseId"
              element={<KnowledgeBaseDetailPage />}
            />
            <Route path="sessions" element={<SessionsPage />} />
            <Route
              path="sessions/:executionId"
              element={<SessionDetailPage />}
            />
            <Route path="approvals" element={<ApprovalsPage />} />
            <Route path="containers" element={<ContainersPage />} />
            <Route path="security/findings" element={<FindingsPage />} />
            <Route path="security/uncover" element={<UncoverPage />} />
            <Route path="security" element={<SecurityPage />} />
            <Route path="settings" element={<SettingsPage />} />
            <Route path="*" element={<NotFound />} />
          </Route>
        </Routes>
      </React.Suspense>
    </ErrorBoundary>
  )
}

function AppBootState({
  error,
  onRetry,
}: {
  error: string | null
  onRetry: () => void
}) {
  return (
    <div className="grid min-h-svh grid-cols-[4.5rem_minmax(0,1fr)] bg-background sm:grid-cols-[15rem_minmax(0,1fr)]">
      <aside className="border-r border-border/70 p-3" aria-hidden="true">
        <Skeleton className="size-8" />
        <div className="mt-8 flex flex-col gap-3">
          <Skeleton className="h-7 w-full" />
          <Skeleton className="h-7 w-4/5" />
          <Skeleton className="h-7 w-11/12" />
        </div>
      </aside>
      <main className="min-w-0 p-2">
        <Skeleton className="h-9 w-56 rounded-full" />
        <div className="mt-2 min-h-[calc(100svh-3.5rem)] rounded-2xl bg-card p-5 ring-1 ring-foreground/5">
          <div className="mx-auto flex max-w-5xl flex-col gap-5">
            <div className="flex flex-col gap-2">
              <Skeleton className="h-2.5 w-28" />
              <Skeleton className="h-8 w-48" />
            </div>
            {error ? (
              <div className="flex min-h-48 flex-col items-center justify-center gap-3 rounded-xl border border-dashed p-6 text-center">
                <p className="text-sm font-medium">无法连接控制平面</p>
                <p className="max-w-md text-xs text-muted-foreground">
                  {error}
                </p>
                <Button variant="outline" size="sm" onClick={onRetry}>
                  重试
                </Button>
              </div>
            ) : (
              <div
                className="grid gap-4 lg:grid-cols-2"
                aria-label="正在连接 Aegis Control Plane"
              >
                <Skeleton className="h-52 w-full rounded-xl" />
                <Skeleton className="h-52 w-full rounded-xl" />
                <span className="sr-only">正在连接 Aegis Control Plane…</span>
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  )
}

function NotFound() {
  return (
    <div className="py-20">
      <Empty>
        <EmptyHeader>
          <EmptyTitle>页面不存在</EmptyTitle>
          <EmptyDescription>检查地址，或从左侧导航返回。</EmptyDescription>
        </EmptyHeader>
      </Empty>
    </div>
  )
}

export default App
