import * as React from "react"
import { Navigate, Route, Routes } from "react-router-dom"

import { AppShell } from "@/components/app-shell"
import { Button } from "@/components/ui/button"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { Spinner } from "@/components/ui/spinner"
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
  if (loading || !state) {
    return (
      <div className="flex min-h-svh items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-3 text-sm text-muted-foreground">
          <Spinner className="size-5" />
          <span>{error ?? "正在连接 Aegis Control Plane…"}</span>
          {error ? (
            <Button variant="outline" size="sm" onClick={() => void refresh()}>
              重试
            </Button>
          ) : null}
        </div>
      </div>
    )
  }
  if (!state.configured) {
    return (
      <React.Suspense fallback={<PageLoader />}>
        <Routes>
          <Route path="/setup" element={<SetupPage />} />
          <Route path="*" element={<Navigate to="/setup" replace />} />
        </Routes>
      </React.Suspense>
    )
  }
  return (
    <React.Suspense fallback={<PageLoader />}>
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
          <Route path="tasks/:issueId/issues" element={<IssuesPage />} />
          <Route path="tasks/:issueId/board" element={<TaskBoardPage />} />
          <Route path="tasks/:issueId" element={<TaskDetailPage />} />
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
          <Route path="sessions/:executionId" element={<SessionDetailPage />} />
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
  )
}

function PageLoader() {
  return (
    <div className="flex min-h-64 items-center justify-center">
      <Spinner className="size-5 text-muted-foreground" />
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
