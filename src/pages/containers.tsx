import * as React from "react"
import {
  Boxes,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Square,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import {
  createContainerProfile,
  buildWorkerContainerImage,
  deleteContainerProfile,
  probeDocker,
  startContainerProfile,
  stopContainerProfile,
  updateContainerProfile,
  type SaveContainerProfileInput,
} from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { ContainerProfile } from "@/types"

const defaults: SaveContainerProfileInput = {
  name: "",
  description: "",
  image: "aegis-pi-worker:latest",
  nodePath: "node",
  piPath: "/usr/local/bin/pi",
  workspacePath: "/workspace",
  hostWorkspace: "",
  networkMode: "bridge",
  memoryMb: 2048,
  cpus: 2,
  enabled: true,
}

export function ContainersPage() {
  const { state, refresh } = useAppState()
  const profiles = state?.containerProfiles ?? []
  const [editing, setEditing] = React.useState<ContainerProfile | "new" | null>(
    null
  )
  const [busy, setBusy] = React.useState(false)

  const checkDocker = async () => {
    try {
      await probeDocker()
      toast.success("Docker daemon 可用")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Docker 不可用")
    }
  }

  const buildImage = async () => {
    setBusy(true)
    try {
      const result = await buildWorkerContainerImage()
      toast.success(`镜像 ${result.image} 构建完成`)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "镜像构建失败")
    } finally {
      setBusy(false)
    }
  }

  const remove = async (profile: ContainerProfile) => {
    if (!window.confirm(`删除容器环境“${profile.name}”？`)) return
    try {
      await deleteContainerProfile(profile.id)
      await refresh()
      toast.success("容器环境已删除")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "删除失败")
    }
  }

  const changeRuntime = async (profile: ContainerProfile) => {
    setBusy(true)
    try {
      if (profile.runtimeStatus === "running")
        await stopContainerProfile(profile.id)
      else await startContainerProfile(profile.id)
      await refresh()
      toast.success(
        profile.runtimeStatus === "running" ? "容器已停止" : "容器已启动"
      )
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "容器操作失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Execution runtime"
        title="容器管理"
        description="配置任务可选择的 Docker 执行环境。所有环境统一使用项目 Dockerfile 构建的 aegis-pi-worker:latest 镜像。"
        actions={
          <>
            <Button variant="outline" onClick={() => void checkDocker()}>
              <RefreshCw />
              检测 Docker
            </Button>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void buildImage()}
            >
              <Boxes />
              {busy ? "构建中…" : "构建 Worker 镜像"}
            </Button>
            <Button onClick={() => setEditing("new")}>
              <Plus />
              新增环境
            </Button>
          </>
        }
      />
      {profiles.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Boxes />
            </EmptyMedia>
            <EmptyTitle>还没有容器环境</EmptyTitle>
            <EmptyDescription>
              创建一个已包含 Node.js 和 Pi CLI 的镜像配置。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {profiles.map((profile) => (
            <Card key={profile.id}>
              <CardHeader>
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <CardTitle>{profile.name}</CardTitle>
                    <CardDescription className="mt-1 font-mono">
                      固定镜像 · {profile.image}
                    </CardDescription>
                  </div>
                  <Badge
                    variant={
                      profile.runtimeStatus === "running"
                        ? "default"
                        : "secondary"
                    }
                  >
                    {profile.runtimeStatus === "running" ? "运行中" : "已停止"}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent className="flex flex-col gap-4">
                {profile.description ? (
                  <p className="text-sm text-muted-foreground">
                    {profile.description}
                  </p>
                ) : null}
                <div className="grid grid-cols-2 gap-3 text-sm">
                  <div>
                    <p className="text-muted-foreground">工作目录</p>
                    <p className="font-mono">{profile.workspacePath}</p>
                  </div>
                  <div>
                    <p className="text-muted-foreground">资源限制</p>
                    <p>
                      {profile.cpus || "不限"} CPU ·{" "}
                      {profile.memoryMb ? `${profile.memoryMb} MB` : "内存不限"}
                    </p>
                  </div>
                </div>
                <div className="flex justify-end gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy || !profile.enabled}
                    onClick={() => void changeRuntime(profile)}
                  >
                    {profile.runtimeStatus === "running" ? (
                      <Square />
                    ) : (
                      <Play />
                    )}
                    {profile.runtimeStatus === "running" ? "停止" : "启动"}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setEditing(profile)}
                  >
                    <Pencil />
                    编辑
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void remove(profile)}
                  >
                    <Trash2 />
                    删除
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
      <ProfileDialog
        key={editing === "new" ? "new" : (editing?.id ?? "closed")}
        value={editing}
        busy={busy}
        onClose={() => setEditing(null)}
        onSave={async (input) => {
          setBusy(true)
          try {
            if (editing === "new") await createContainerProfile(input)
            else if (editing) await updateContainerProfile(editing.id, input)
            await refresh()
            setEditing(null)
            toast.success("容器环境已保存")
          } catch (reason) {
            toast.error(reason instanceof Error ? reason.message : "保存失败")
          } finally {
            setBusy(false)
          }
        }}
      />
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
          nodePath: value.nodePath,
          piPath: value.piPath,
          workspacePath: value.workspacePath,
          hostWorkspace: value.hostWorkspace,
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
        if (!open) onClose()
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {value === "new" ? "新增容器环境" : "编辑容器环境"}
          </DialogTitle>
          <DialogDescription>
            固定使用项目 Dockerfile 构建的
            aegis-pi-worker:latest；宿主工作区会挂载到容器工作目录。
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel>名称</FieldLabel>
              <Input
                required
                value={form.name}
                onChange={(event) => set("name", event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>Docker Image</FieldLabel>
              <Input disabled value="aegis-pi-worker:latest" />
              <FieldDescription>
                由项目根目录 Dockerfile 统一构建，不允许环境单独覆盖。
              </FieldDescription>
            </Field>
          </div>
          <Field>
            <FieldLabel>说明</FieldLabel>
            <Textarea
              rows={2}
              value={form.description}
              onChange={(event) => set("description", event.target.value)}
            />
          </Field>
          <div className="grid gap-4 sm:grid-cols-1">
            <Field>
              <FieldLabel>宿主机工作目录</FieldLabel>
              <Input
                required
                placeholder="/absolute/path/to/workspace"
                value={form.hostWorkspace}
                onChange={(event) => set("hostWorkspace", event.target.value)}
              />
              <FieldDescription>
                启动容器时挂载到下方容器工作目录；选择该容器的任务将自动使用此目录。
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel>容器工作目录</FieldLabel>
              <Input
                value={form.workspacePath}
                onChange={(event) => set("workspacePath", event.target.value)}
              />
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <Field>
              <FieldLabel>网络</FieldLabel>
              <Select
                value={form.networkMode}
                onValueChange={(next) =>
                  set("networkMode", next as "bridge" | "none")
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="bridge">bridge</SelectItem>
                  <SelectItem value="none">none</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel>内存 (MB)</FieldLabel>
              <Input
                type="number"
                min={0}
                value={form.memoryMb}
                onChange={(event) =>
                  set("memoryMb", Number(event.target.value))
                }
              />
            </Field>
            <Field>
              <FieldLabel>CPU</FieldLabel>
              <Input
                type="number"
                min={0}
                step="0.1"
                value={form.cpus}
                onChange={(event) => set("cpus", Number(event.target.value))}
              />
            </Field>
          </div>
          <Field orientation="horizontal">
            <Switch
              checked={form.enabled}
              onCheckedChange={(checked) => set("enabled", checked)}
            />
            <div>
              <FieldLabel>允许任务选择</FieldLabel>
              <FieldDescription>
                停用不会影响历史 Execution，但新任务不能再选择。
              </FieldDescription>
            </div>
          </Field>
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button
            disabled={busy || !form.name.trim()}
            onClick={() => void onSave(form)}
          >
            {busy ? "保存中…" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
