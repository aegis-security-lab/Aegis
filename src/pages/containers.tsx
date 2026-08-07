import * as React from "react"
import {
  Boxes,
  Container as ContainerIcon,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Settings2,
  Square,
  Trash2,
  TriangleAlert,
} from "lucide-react"
import { Link } from "react-router-dom"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
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
  FieldContent,
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
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import {
  buildWorkerContainerImage,
  createContainerProfile,
  deleteContainer,
  deleteContainers,
  deleteContainerProfile,
  fetchContainerBatchDeleteImpact,
  fetchContainerDeleteImpact,
  fetchContainerProfileDeleteImpact,
  probeDocker,
  startContainer,
  stopContainer,
  stopContainers,
  updateContainerProfile,
  type SaveContainerProfileInput,
} from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import type {
  ContainerBatchDeleteImpact,
  ContainerDeleteImpact,
  ContainerInstance,
  ContainerProfile,
  ContainerProfileDeleteImpact,
} from "@/types"

const defaults: SaveContainerProfileInput = {
	name: "",
	description: "",
	image: "aegis-worker:latest",
	workspacePath: "/workspace",
  networkMode: "bridge",
  memoryMb: 2048,
  cpus: 2,
  enabled: true,
}

const runtimeLabels: Record<ContainerInstance["runtimeStatus"], string> = {
  created: "已创建",
  running: "运行中",
  paused: "已暂停",
  restarting: "重启中",
  removing: "删除中",
  exited: "已停止",
  dead: "异常",
  missing: "待重建",
  unavailable: "Docker 不可用",
}

function runtimeBadgeVariant(
  status: ContainerInstance["runtimeStatus"]
): "default" | "secondary" | "outline" | "destructive" {
  if (status === "running") return "default"
  if (status === "dead" || status === "unavailable") return "destructive"
  if (status === "missing") return "outline"
  return "secondary"
}

export function ContainersPage() {
  const { state, refresh } = useAppState()
  const profiles = state?.containerProfiles ?? []
  const containers = state?.containers ?? []
  const tasks = state?.tasks ?? []
  const issues = state?.issues ?? []
  const [activeTab, setActiveTab] = React.useState("profiles")
  const [editing, setEditing] = React.useState<ContainerProfile | "new" | null>(
    null
  )
  const [busyAction, setBusyAction] = React.useState<string | null>(null)
  const [profileDeleteTarget, setProfileDeleteTarget] =
    React.useState<ContainerProfile | null>(null)
  const [profileDeleteImpact, setProfileDeleteImpact] =
    React.useState<ContainerProfileDeleteImpact | null>(null)
  const [containerDeleteTarget, setContainerDeleteTarget] =
    React.useState<ContainerInstance | null>(null)
  const [containerDeleteImpact, setContainerDeleteImpact] =
    React.useState<ContainerDeleteImpact | null>(null)
  const [selectedContainerIds, setSelectedContainerIds] = React.useState(
    () => new Set<string>()
  )
  const [batchDeleteImpact, setBatchDeleteImpact] =
    React.useState<ContainerBatchDeleteImpact | null>(null)

  const activeSelectedContainerIds = new Set(
    containers
      .filter((container) => selectedContainerIds.has(container.id))
      .map((container) => container.id)
  )
  const selectedContainers = containers.filter((container) =>
    activeSelectedContainerIds.has(container.id)
  )
  const stoppableContainers = selectedContainers.filter((container) =>
    ["running", "paused", "restarting"].includes(container.runtimeStatus)
  )
  const allContainersSelected =
    containers.length > 0 && activeSelectedContainerIds.size === containers.length
  const someContainersSelected =
    activeSelectedContainerIds.size > 0 && !allContainersSelected

  const checkDocker = async () => {
    setBusyAction("probe")
    try {
      await probeDocker()
      toast.success("Docker daemon 可用")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Docker 不可用")
    } finally {
      setBusyAction(null)
    }
  }

  const buildImage = async () => {
    setBusyAction("build")
    try {
      const result = await buildWorkerContainerImage()
      toast.success(`镜像 ${result.image} 构建完成`)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "镜像构建失败")
    } finally {
      setBusyAction(null)
    }
  }

  const prepareProfileDelete = async (profile: ContainerProfile) => {
    setBusyAction(`profile-impact:${profile.id}`)
    try {
      setProfileDeleteImpact(
        await fetchContainerProfileDeleteImpact(profile.id)
      )
      setProfileDeleteTarget(profile)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "无法检查删除影响")
    } finally {
      setBusyAction(null)
    }
  }

  const removeProfile = async () => {
    if (!profileDeleteTarget || !profileDeleteImpact) return
    setBusyAction(`profile-delete:${profileDeleteTarget.id}`)
    try {
      await deleteContainerProfile(profileDeleteTarget.id, false)
      await refresh()
      toast.success("环境配置已删除")
      setProfileDeleteTarget(null)
      setProfileDeleteImpact(null)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "删除失败")
    } finally {
      setBusyAction(null)
    }
  }

  const changeRuntime = async (container: ContainerInstance) => {
    setBusyAction(`runtime:${container.id}`)
    try {
      if (container.runtimeStatus === "running") await stopContainer(container.id)
      else await startContainer(container.id)
      await refresh()
      toast.success(
        container.runtimeStatus === "running" ? "容器已停止" : "容器已启动"
      )
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "容器操作失败")
    } finally {
      setBusyAction(null)
    }
  }

  const prepareContainerDelete = async (container: ContainerInstance) => {
    setBusyAction(`container-impact:${container.id}`)
    try {
      setContainerDeleteImpact(await fetchContainerDeleteImpact(container.id))
      setContainerDeleteTarget(container)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "无法检查删除影响")
    } finally {
      setBusyAction(null)
    }
  }

  const removeContainer = async () => {
    if (!containerDeleteTarget || !containerDeleteImpact) return
    setBusyAction(`container-delete:${containerDeleteTarget.id}`)
    try {
      const result = await deleteContainer(containerDeleteTarget.id, true)
      await refresh()
      toast.success("容器及关联任务已删除", {
        description: `同时删除 ${result.deletedTasks} 个任务、${result.deletedIssues} 个 Issues 和 ${result.deletedExecutions} 条执行记录。`,
      })
      setContainerDeleteTarget(null)
      setContainerDeleteImpact(null)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "删除失败")
    } finally {
      setBusyAction(null)
    }
  }

  const stopSelectedContainers = async () => {
    if (stoppableContainers.length === 0) return
    setBusyAction("container-batch-stop")
    try {
      const result = await stopContainers(
        stoppableContainers.map((container) => container.id)
      )
      await refresh()
      setSelectedContainerIds(
        new Set(result.failed.map((failure) => failure.containerId))
      )
      if (result.failed.length > 0) {
        toast.warning(
          `已停止 ${result.stopped.length} 个容器，${result.failed.length} 个失败`,
          { description: result.failed[0]?.error }
        )
      } else {
        toast.success(`已停止 ${result.stopped.length} 个容器`, {
          description: "任务工作区和数据卷均已保留。",
        })
      }
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "批量停止失败")
    } finally {
      setBusyAction(null)
    }
  }

  const prepareBatchDelete = async () => {
    if (activeSelectedContainerIds.size === 0) return
    setBusyAction("container-batch-impact")
    try {
      setBatchDeleteImpact(
        await fetchContainerBatchDeleteImpact([...activeSelectedContainerIds])
      )
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "无法检查批量删除影响")
    } finally {
      setBusyAction(null)
    }
  }

  const removeSelectedContainers = async () => {
    if (!batchDeleteImpact) return
    setBusyAction("container-batch-delete")
    try {
      const result = await deleteContainers(
        batchDeleteImpact.containerIds,
        true
      )
      await refresh()
      setBatchDeleteImpact(null)
      setSelectedContainerIds(
        new Set(result.failed.map((failure) => failure.containerId))
      )
      const description = `同时删除 ${result.deletedTasks} 个任务、${result.deletedIssues} 个 Issues 和 ${result.deletedExecutions} 条执行记录。`
      if (result.failed.length > 0) {
        toast.warning(
          `已删除 ${result.deleted.length} 个容器，${result.failed.length} 个失败`,
          { description: `${description} ${result.failed[0]?.error ?? ""}` }
        )
      } else {
        toast.success(`已删除 ${result.deleted.length} 个容器`, {
          description,
        })
      }
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "批量删除失败")
    } finally {
      setBusyAction(null)
    }
  }

  const profileHasReferences = Boolean(
    profileDeleteImpact &&
      (profileDeleteImpact.containerCount > 0 ||
        profileDeleteImpact.taskCount > 0 ||
        profileDeleteImpact.issueCount > 0)
  )

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Execution runtime"
        title="容器管理"
        actions={
          <>
            <Button
              variant="outline"
              disabled={busyAction !== null}
              onClick={() => void checkDocker()}
            >
              {busyAction === "probe" ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <RefreshCw data-icon="inline-start" />
              )}
              检测 Docker
            </Button>
            <Button
              variant="outline"
              disabled={busyAction !== null}
              onClick={() => void buildImage()}
            >
              {busyAction === "build" ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Boxes data-icon="inline-start" />
              )}
              {busyAction === "build" ? "构建中…" : "构建 Worker 镜像"}
            </Button>
            <Button
              onClick={() => {
                setActiveTab("profiles")
                setEditing("new")
              }}
            >
              <Plus data-icon="inline-start" />
              新增环境配置
            </Button>
          </>
        }
      />

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="profiles">
            <Settings2 data-icon="inline-start" />
            环境配置
            <Badge variant="secondary">{profiles.length}</Badge>
          </TabsTrigger>
          <TabsTrigger value="containers">
            <ContainerIcon data-icon="inline-start" />
            容器管理
            <Badge variant="secondary">{containers.length}</Badge>
          </TabsTrigger>
        </TabsList>

        <TabsContent value="profiles" className="pt-4">
          {profiles.length === 0 ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Settings2 />
                </EmptyMedia>
                <EmptyTitle>还没有环境配置</EmptyTitle>
                <EmptyDescription>
                  创建配置只会保存镜像、资源和网络设置，不会立即创建容器。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className="grid gap-4 lg:grid-cols-2">
              {profiles.map((profile) => {
                const configuredTasks = tasks.filter(
                  (task) => task.containerProfileId === profile.id
                )
                const createdContainers = containers.filter(
                  (container) => container.containerProfileId === profile.id
                )
                const checking =
                  busyAction === `profile-impact:${profile.id}`
                return (
                  <Card key={profile.id}>
                    <CardHeader>
                      <CardTitle>{profile.name}</CardTitle>
                      <CardDescription>
                        {profile.description || "任务容器创建模板"}
                      </CardDescription>
                      <CardAction>
                        <Badge variant={profile.enabled ? "outline" : "secondary"}>
                          {profile.enabled ? "可供任务选择" : "已停用"}
                        </Badge>
                      </CardAction>
                    </CardHeader>
                    <CardContent className="flex flex-col gap-4">
                      <dl className="grid grid-cols-2 gap-3 text-sm">
                        <div className="col-span-2 min-w-0">
                          <dt className="text-muted-foreground">镜像</dt>
                          <dd className="truncate font-mono" title={profile.image}>
                            {profile.image}
                          </dd>
                        </div>
                        <div>
                          <dt className="text-muted-foreground">容器工作目录</dt>
                          <dd className="font-mono">{profile.workspacePath}</dd>
                        </div>
                        <div>
                          <dt className="text-muted-foreground">网络</dt>
                          <dd className="font-mono">{profile.networkMode}</dd>
                        </div>
                        <div>
                          <dt className="text-muted-foreground">资源限制</dt>
                          <dd>
                            {profile.cpus || "不限"} CPU · {profile.memoryMb ? `${profile.memoryMb} MB` : "内存不限"}
                          </dd>
                        </div>
						<div>
							<dt className="text-muted-foreground">执行方式</dt>
							<dd>AgentCore 通过 Docker exec 调用工具</dd>
						</div>
                      </dl>
                      <Separator />
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="secondary">
                          {configuredTasks.length} 个关联任务
                        </Badge>
                        <Badge variant="secondary">
                          {createdContainers.length} 个已创建容器
                        </Badge>
                      </div>
                      <p className="text-sm text-muted-foreground">
                        修改此配置只影响以后创建的容器，现有任务容器继续使用创建时的配置快照。
                      </p>
                    </CardContent>
                    <CardFooter className="justify-end gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setEditing(profile)}
                      >
                        <Pencil data-icon="inline-start" />
                        编辑
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={checking}
                        onClick={() => void prepareProfileDelete(profile)}
                      >
                        {checking ? (
                          <Spinner data-icon="inline-start" />
                        ) : (
                          <Trash2 data-icon="inline-start" />
                        )}
                        删除
                      </Button>
                    </CardFooter>
                  </Card>
                )
              })}
            </div>
          )}
        </TabsContent>

        <TabsContent value="containers" className="pt-4">
          {containers.length === 0 ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <ContainerIcon />
                </EmptyMedia>
                <EmptyTitle>还没有任务容器</EmptyTitle>
                <EmptyDescription>
                  发布任务时，系统会立即创建并绑定唯一容器；真正执行时再启动 Docker Runtime。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <Card className="overflow-visible">
              <CardHeader>
                <CardTitle>全部任务容器</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="sticky top-2 z-20 mb-3 flex min-h-12 flex-wrap items-center justify-between gap-3 rounded-lg border bg-card/95 px-3 py-2 backdrop-blur supports-[backdrop-filter]:bg-card/85">
                  <div className="flex items-center gap-2 text-sm">
                    <Badge variant={activeSelectedContainerIds.size > 0 ? "default" : "secondary"}>
                      已选 {activeSelectedContainerIds.size}
                    </Badge>
                    <span className="text-muted-foreground">
                      {activeSelectedContainerIds.size > 0
                        ? `其中 ${stoppableContainers.length} 个正在运行，可安全停止并保留工作区。`
                        : "勾选容器后可以批量停止或删除。"}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    {activeSelectedContainerIds.size > 0 ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={busyAction !== null}
                        onClick={() => setSelectedContainerIds(new Set())}
                      >
                        清除选择
                      </Button>
                    ) : null}
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={stoppableContainers.length === 0 || busyAction !== null}
                      onClick={() => void stopSelectedContainers()}
                    >
                      {busyAction === "container-batch-stop" ? (
                        <Spinner data-icon="inline-start" />
                      ) : (
                        <Square data-icon="inline-start" />
                      )}
                      停止运行项
                      {stoppableContainers.length > 0 ? ` (${stoppableContainers.length})` : ""}
                    </Button>
                    <Button
                      variant="destructive"
                      size="sm"
                      disabled={activeSelectedContainerIds.size === 0 || busyAction !== null}
                      onClick={() => void prepareBatchDelete()}
                    >
                      {busyAction === "container-batch-impact" ? (
                        <Spinner data-icon="inline-start" />
                      ) : (
                        <Trash2 data-icon="inline-start" />
                      )}
                      批量删除
                    </Button>
                  </div>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10">
                        <Checkbox
                          aria-label="选择全部容器"
                          checked={allContainersSelected}
                          indeterminate={someContainersSelected}
                          onCheckedChange={(checked) =>
                            setSelectedContainerIds(
                              checked === true
                                ? new Set(containers.map((container) => container.id))
                                : new Set()
                            )
                          }
                        />
                      </TableHead>
                      <TableHead>容器</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead>环境配置</TableHead>
                      <TableHead>关联任务</TableHead>
                      <TableHead>工作目录</TableHead>
                      <TableHead>创建时间</TableHead>
                      <TableHead className="text-right">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {containers.map((container) => {
                      const profile = profiles.find(
                        (candidate) => candidate.id === container.containerProfileId
                      )
                      const task = tasks.find(
                        (candidate) => candidate.id === container.taskId
                      )
                      const latestRun = issues
                        .filter(
                          (issue) =>
                            !issue.parentId && issue.taskSourceId === container.taskId
                        )
                        .sort((left, right) =>
                          right.createdAt.localeCompare(left.createdAt)
                        )[0]
                      const changing = busyAction === `runtime:${container.id}`
                      const checkingDelete =
                        busyAction === `container-impact:${container.id}`
                      const selected = activeSelectedContainerIds.has(container.id)
                      return (
                        <TableRow
                          key={container.id}
                          data-state={selected ? "selected" : undefined}
                        >
                          <TableCell>
                            <Checkbox
                              aria-label={`选择容器 ${container.name}`}
                              checked={selected}
                              onCheckedChange={(checked) =>
                                setSelectedContainerIds((current) => {
                                  const next = new Set(current)
                                  if (checked === true) next.add(container.id)
                                  else next.delete(container.id)
                                  return next
                                })
                              }
                            />
                          </TableCell>
                          <TableCell>
                            <div className="flex max-w-64 flex-col gap-1">
                              <span className="truncate font-medium" title={container.name}>
                                {container.name}
                              </span>
                              <span className="truncate font-mono text-xs text-muted-foreground" title={container.image}>
                                {container.image}
                              </span>
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge variant={runtimeBadgeVariant(container.runtimeStatus)}>
                              {runtimeLabels[container.runtimeStatus]}
                            </Badge>
                          </TableCell>
                          <TableCell>{profile?.name ?? "配置已删除"}</TableCell>
                          <TableCell>
                            {latestRun ? (
                              <Link
                                to={`/tasks/${latestRun.id}`}
                                className="font-medium hover:underline"
                              >
                                {task?.title ?? latestRun.title}
                                <span className="ml-2 text-xs text-muted-foreground">
                                  {latestRun.identifier}
                                </span>
                              </Link>
                            ) : (
                              task?.title ?? "任务已删除"
                            )}
                          </TableCell>
                          <TableCell className="font-mono">
                            {container.workspacePath}
                          </TableCell>
                          <TableCell>{formatTime(container.createdAt)}</TableCell>
                          <TableCell>
                            <div className="flex justify-end gap-2">
                              <Button
                                variant="outline"
                                size="sm"
                                disabled={changing || busyAction !== null && !changing}
                                onClick={() => void changeRuntime(container)}
                              >
                                {changing ? (
                                  <Spinner data-icon="inline-start" />
                                ) : container.runtimeStatus === "running" ? (
                                  <Square data-icon="inline-start" />
                                ) : (
                                  <Play data-icon="inline-start" />
                                )}
                                {container.runtimeStatus === "running" ? "停止" : "启动"}
                              </Button>
                              <Button
                                variant="outline"
                                size="sm"
                                disabled={checkingDelete}
                                onClick={() => void prepareContainerDelete(container)}
                              >
                                {checkingDelete ? (
                                  <Spinner data-icon="inline-start" />
                                ) : (
                                  <Trash2 data-icon="inline-start" />
                                )}
                                删除
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          )}
        </TabsContent>
      </Tabs>

      <ProfileDialog
        key={editing === "new" ? "new" : (editing?.id ?? "closed")}
        value={editing}
        busy={busyAction === "profile-save"}
        onClose={() => setEditing(null)}
        onSave={async (input) => {
          setBusyAction("profile-save")
          try {
            if (editing === "new") await createContainerProfile(input)
            else if (editing) await updateContainerProfile(editing.id, input)
            await refresh()
            setEditing(null)
            toast.success("环境配置已保存", {
              description:
                editing === "new"
                  ? "尚未创建容器；任务选择此配置并开始执行时才会创建。"
                  : "现有容器不变，新配置将用于以后创建的容器。",
            })
          } catch (reason) {
            toast.error(reason instanceof Error ? reason.message : "保存失败")
          } finally {
            setBusyAction(null)
          }
        }}
      />

      <AlertDialog
        open={profileDeleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && busyAction?.startsWith("profile-delete:") !== true) {
            setProfileDeleteTarget(null)
            setProfileDeleteImpact(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <TriangleAlert />
            </AlertDialogMedia>
            <AlertDialogTitle>
              {profileHasReferences ? "环境配置仍被使用" : "删除环境配置？"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {profileHasReferences
                ? `“${profileDeleteTarget?.name ?? "该配置"}”仍关联 ${profileDeleteImpact?.containerCount ?? 0} 个容器、${profileDeleteImpact?.taskCount ?? 0} 个任务和 ${profileDeleteImpact?.issueCount ?? 0} 个 Issues。请先处理关联容器或任务，配置不会级联删除业务数据。`
                : `“${profileDeleteTarget?.name ?? "该配置"}”只包含创建模板，删除后无法恢复。`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>
              {profileHasReferences ? "关闭" : "取消"}
            </AlertDialogCancel>
            {!profileHasReferences ? (
              <AlertDialogAction
                variant="destructive"
                disabled={busyAction?.startsWith("profile-delete:")}
                onClick={() => void removeProfile()}
              >
                {busyAction?.startsWith("profile-delete:") ? (
                  <Spinner data-icon="inline-start" />
                ) : (
                  <Trash2 data-icon="inline-start" />
                )}
                确认删除
              </AlertDialogAction>
            ) : null}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={containerDeleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && busyAction?.startsWith("container-delete:") !== true) {
            setContainerDeleteTarget(null)
            setContainerDeleteImpact(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <TriangleAlert />
            </AlertDialogMedia>
            <AlertDialogTitle>删除容器及关联任务？</AlertDialogTitle>
            <AlertDialogDescription>
              {`“${containerDeleteTarget?.name ?? "该容器"}”绑定了 1 个任务、${containerDeleteImpact?.issueCount ?? 0} 个 Issues 和 ${containerDeleteImpact?.executionCount ?? 0} 条执行记录。确认后会停止并删除容器及数据卷，同时永久删除这些关联数据。`}
              {(containerDeleteImpact?.activeExecutionCount ?? 0) > 0
                ? ` 其中 ${containerDeleteImpact?.activeExecutionCount} 个执行仍在运行，将立即中止。`
                : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel
              disabled={busyAction?.startsWith("container-delete:")}
            >
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={busyAction?.startsWith("container-delete:")}
              onClick={() => void removeContainer()}
            >
              {busyAction?.startsWith("container-delete:") ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Trash2 data-icon="inline-start" />
              )}
              确认删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={batchDeleteImpact !== null}
        onOpenChange={(open) => {
          if (!open && busyAction !== "container-batch-delete") {
            setBatchDeleteImpact(null)
          }
        }}
      >
        <AlertDialogContent className="sm:max-w-lg">
          <AlertDialogHeader>
            <AlertDialogMedia className="bg-destructive/10 text-destructive">
              <TriangleAlert />
            </AlertDialogMedia>
            <AlertDialogTitle>
              永久删除 {batchDeleteImpact?.containerCount ?? 0} 个容器？
            </AlertDialogTitle>
            <AlertDialogDescription>
              确认后会逐个停止并删除所选容器及数据卷，同时永久删除绑定的任务和执行数据。成功项不会因为其他容器失败而回滚。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {[
              ["任务", batchDeleteImpact?.taskCount ?? 0],
              ["Issues", batchDeleteImpact?.issueCount ?? 0],
              ["执行记录", batchDeleteImpact?.executionCount ?? 0],
              ["活跃执行", batchDeleteImpact?.activeExecutionCount ?? 0],
            ].map(([label, value]) => (
              <div key={label} className="rounded-md bg-muted/60 px-3 py-2">
                <div className="text-xs text-muted-foreground">{label}</div>
                <div className="mt-1 font-mono text-lg font-semibold tabular-nums">
                  {value}
                </div>
              </div>
            ))}
          </div>
          {(batchDeleteImpact?.activeExecutionCount ?? 0) > 0 ? (
            <p className="text-sm font-medium text-destructive">
              活跃 Execution 将被立即中止，尚未保存的执行状态可能丢失。
            </p>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busyAction === "container-batch-delete"}>
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={busyAction === "container-batch-delete"}
              onClick={() => void removeSelectedContainers()}
            >
              {busyAction === "container-batch-delete" ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Trash2 data-icon="inline-start" />
              )}
              确认批量删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function ProfileDialog({
  value,
  busy,
  onClose,
  onSave,
}: {
  value: ContainerProfile | "new" | null
  busy: boolean
  onClose: () => void
  onSave: (input: SaveContainerProfileInput) => Promise<void>
}) {
  const [form, setForm] = React.useState<SaveContainerProfileInput>(() =>
    value && value !== "new"
      ? {
          name: value.name,
          description: value.description,
          image: value.image,
          workspacePath: value.workspacePath,
          networkMode: value.networkMode,
          memoryMb: value.memoryMb,
          cpus: value.cpus,
          enabled: value.enabled,
        }
      : defaults
  )
  const set = <K extends keyof SaveContainerProfileInput>(
    key: K,
    next: SaveContainerProfileInput[K]
  ) => setForm((current) => ({ ...current, [key]: next }))

  return (
    <Dialog
      open={value !== null}
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {value === "new" ? "新增环境配置" : "编辑环境配置"}
          </DialogTitle>
          <DialogDescription>
            此处只保存容器创建模板，不会创建或启动 Docker 容器。任务选择配置并开始执行时才会创建独立容器。
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <FieldGroup className="grid sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="container-name">名称</FieldLabel>
              <Input
                id="container-name"
                required
                value={form.name}
                onChange={(event) => set("name", event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="container-image">Docker Image</FieldLabel>
              <Input
                id="container-image"
                disabled
                value="aegis-worker:latest"
              />
              <FieldDescription>
                由项目 Dockerfile 统一构建，不允许配置单独覆盖。
              </FieldDescription>
            </Field>
          </FieldGroup>
          <Field>
            <FieldLabel htmlFor="container-description">说明</FieldLabel>
            <Textarea
              id="container-description"
              rows={2}
              value={form.description}
              onChange={(event) => set("description", event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="container-workspace">容器工作目录</FieldLabel>
            <Input
              id="container-workspace"
              disabled
              value="/workspace"
            />
            <FieldDescription>
              固定为 /workspace；每个任务使用自己的 Docker Volume，不映射宿主机目录。
            </FieldDescription>
          </Field>
          <FieldGroup className="grid sm:grid-cols-3">
            <Field>
              <FieldLabel htmlFor="container-network">网络</FieldLabel>
              <Select
                value={form.networkMode}
                onValueChange={(next) =>
                  set("networkMode", next as "bridge" | "none")
                }
              >
                <SelectTrigger id="container-network">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="bridge">bridge</SelectItem>
                    <SelectItem value="none">none</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel htmlFor="container-memory">内存 (MB)</FieldLabel>
              <Input
                id="container-memory"
                type="number"
                min={0}
                value={form.memoryMb}
                onChange={(event) => set("memoryMb", Number(event.target.value))}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="container-cpus">CPU</FieldLabel>
              <Input
                id="container-cpus"
                type="number"
                min={0}
                step="0.1"
                value={form.cpus}
                onChange={(event) => set("cpus", Number(event.target.value))}
              />
            </Field>
          </FieldGroup>
          <Field orientation="horizontal">
            <Switch
              id="container-enabled"
              checked={form.enabled}
              onCheckedChange={(checked) => set("enabled", checked)}
            />
            <FieldContent>
              <FieldLabel htmlFor="container-enabled">允许任务选择</FieldLabel>
              <FieldDescription>
                停用后，新任务不能选择此配置；已经创建的容器不受影响。
              </FieldDescription>
            </FieldContent>
          </Field>
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button
            disabled={busy || !form.name.trim()}
            onClick={() => void onSave(form)}
          >
            {busy ? <Spinner data-icon="inline-start" /> : null}
            {busy ? "保存中…" : "保存配置"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
