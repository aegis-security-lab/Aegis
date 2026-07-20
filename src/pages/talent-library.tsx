import * as React from "react"
import { Eye, EyeOff, Search, UserPlus, Users } from "lucide-react"
import { Link } from "react-router-dom"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { fetchAgentTemplates, setAgentTemplateHidden } from "@/lib/api"
import type { AgentTemplate } from "@/types"

export function TalentLibraryPage() {
  const [templates, setTemplates] = React.useState<AgentTemplate[]>([])
  const [query, setQuery] = React.useState("")
  const [provider, setProvider] = React.useState("all")
  const [position, setPosition] = React.useState("all")
  const [showHidden, setShowHidden] = React.useState(false)
  const load = React.useCallback(
    async () => setTemplates(await fetchAgentTemplates()),
    []
  )
  React.useEffect(() => {
    let active = true
    void fetchAgentTemplates().then((items) => {
      if (active) setTemplates(items)
    })
    return () => {
      active = false
    }
  }, [])
  const providers = [...new Set(templates.map((item) => item.provider))].sort()
  const positions = [
    ...new Set(templates.flatMap((item) => item.metadata.positions)),
  ].sort()
  const normalized = query.trim().toLowerCase()
  const visible = templates.filter(
    (item) =>
      (showHidden || !item.hidden) &&
      (provider === "all" || item.provider === provider) &&
      (position === "all" || item.metadata.positions.includes(position)) &&
      (!normalized ||
        [
          item.id,
          item.provider,
          item.model,
          item.metadata.englishName,
          item.metadata.chineseName,
          item.metadata.introduction,
          ...item.metadata.positions,
        ].some((value) => value.toLowerCase().includes(normalized)))
  )
  const toggleHidden = async (item: AgentTemplate) => {
    try {
      await setAgentTemplateHidden(item.id, !item.hidden)
      await load()
      toast.success(item.hidden ? "已恢复显示" : "已标记为不看")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "更新失败")
    }
  }
  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Agent marketplace"
        title="人才库"
        description="浏览可复用的人才模板；厂商、模型和系统提示词共同确定模板身份。"
      />
      <div className="grid gap-3 rounded-xl border bg-card p-4 md:grid-cols-[minmax(0,1fr)_180px_180px_auto]">
        <div className="relative">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-9"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索名称、介绍、职位、模型或 ID"
          />
        </div>
        <Select value={provider} onValueChange={(v) => setProvider(String(v))}>
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部厂商</SelectItem>
            {providers.map((v) => (
              <SelectItem key={v} value={v}>
                {v}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={position} onValueChange={(v) => setPosition(String(v))}>
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部职位</SelectItem>
            {positions.map((v) => (
              <SelectItem key={v} value={v}>
                {v}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          type="button"
          variant={showHidden ? "secondary" : "outline"}
          onClick={() => setShowHidden((v) => !v)}
        >
          {showHidden ? <Eye /> : <EyeOff />}
          {showHidden ? "隐藏已显示" : "查看不看的"}
        </Button>
      </div>
      {visible.length ? (
        <div className="grid gap-5 lg:grid-cols-2 xl:grid-cols-3">
          {visible.map((item) => (
            <Card
              key={item.id}
              className={item.hidden ? "opacity-60" : undefined}
            >
              <CardHeader>
                <div className="flex items-start gap-3">
                  <span className="flex size-10 items-center justify-center rounded-xl bg-muted">
                    <Users className="size-5" />
                  </span>
                  <div className="min-w-0">
                    <CardTitle>{item.metadata.chineseName}</CardTitle>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {item.metadata.englishName}
                    </p>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                <p className="line-clamp-3 text-sm text-muted-foreground">
                  {item.metadata.introduction}
                </p>
                <div className="flex flex-wrap gap-2">
                  {item.metadata.positions.map((v) => (
                    <Badge key={v} variant="secondary">
                      {v}
                    </Badge>
                  ))}
                </div>
                <div className="rounded-lg bg-muted/50 p-3 text-xs">
                  <div>
                    {item.provider} / {item.model}
                  </div>
                  <div className="mt-1 truncate font-mono text-muted-foreground">
                    {item.id}
                  </div>
                </div>
              </CardContent>
              <CardFooter className="justify-between">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => void toggleHidden(item)}
                >
                  {item.hidden ? <Eye /> : <EyeOff />}
                  {item.hidden ? "恢复显示" : "不看"}
                </Button>
                <Button
                  size="sm"
                  render={
                    <Link
                      to={`/agents?template=${encodeURIComponent(item.id)}`}
                    />
                  }
                >
                  <UserPlus />
                  聘用
                </Button>
              </CardFooter>
            </Card>
          ))}
        </div>
      ) : (
        <Card>
          <CardContent className="py-16 text-center text-sm text-muted-foreground">
            没有符合条件的人才模板。
          </CardContent>
        </Card>
      )}
    </div>
  )
}
