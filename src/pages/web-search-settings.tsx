import * as React from "react"
import { ExternalLink, KeyRound, Save, Search } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { saveWebSearch, testWebSearch } from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { WebSearchConfig, WebSearchInput, WebSearchResult } from "@/types"

export function WebSearchSettingsPage() {
  const { state, setState } = useAppState()
  const stored = state!.config.webSearch
  const [config, setConfig] = React.useState<WebSearchConfig>({
    engine: "tavily",
    baseUrl: stored?.baseUrl || "https://api.tavily.com/search",
    apiKey: "",
    enabled: true,
  })
  const [search, setSearch] = React.useState<WebSearchInput>({
    query: "latest AI agent news",
    topic: "general",
    searchDepth: "basic",
    includeAnswer: true,
    maxResults: 5,
  })
  const [result, setResult] = React.useState<WebSearchResult | null>(null)
  const [busy, setBusy] = React.useState<"save" | "search" | null>(null)

  const save = async () => {
    setBusy("save")
    try {
      const next = await saveWebSearch(config)
      setState(next)
      setConfig((current) => ({ ...current, apiKey: "" }))
      toast.success("Tavily 搜索服务已保存")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存失败")
    } finally {
      setBusy(null)
    }
  }

  const runSearch = async () => {
    if (!search.query.trim()) return
    setBusy("search")
    try {
      setResult(await testWebSearch(config, search))
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "检索失败")
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="flex min-h-full flex-col gap-5">
      <div>
        <h2 className="text-2xl font-semibold tracking-tight">搜索服务</h2>
        <p className="mt-1 text-sm text-muted-foreground">配置所有 Agent 共用的公开 Web 搜索能力，并在保存前测试返回结果。</p>
      </div>
      <div className="grid min-h-[620px] flex-1 gap-5 xl:grid-cols-[180px_minmax(320px,0.9fr)_minmax(360px,1.1fr)]">
        <Card>
          <CardHeader><CardTitle>搜索引擎</CardTitle></CardHeader>
          <CardContent>
            <Button type="button" variant="secondary" className="w-full justify-start"><Search data-icon="inline-start" />Tavily</Button>
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle>Tavily 配置</CardTitle></CardHeader>
          <CardContent>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="web-search-base-url">API 地址</FieldLabel>
                <Input id="web-search-base-url" value={config.baseUrl} onChange={(event) => setConfig((current) => ({ ...current, baseUrl: event.target.value }))} placeholder="https://example.com/api/tavily/search" />
              </Field>
              <Field>
                <FieldLabel htmlFor="web-search-api-key">API Key</FieldLabel>
                <Input id="web-search-api-key" type="password" autoComplete="off" value={config.apiKey} onChange={(event) => setConfig((current) => ({ ...current, apiKey: event.target.value }))} placeholder={stored?.hasApiKey ? "已保存；留空保持不变" : "th-..."} />
                <FieldDescription>配置完整后自动启用。密钥只保存在服务端，不会暴露给 Agent。</FieldDescription>
              </Field>
              <Button type="button" onClick={() => void save()} disabled={busy !== null || !config.baseUrl.trim()}>{busy === "save" ? <Spinner /> : <Save data-icon="inline-start" />}保存配置</Button>
              <Field>
                <FieldLabel htmlFor="web-search-query">测试关键词</FieldLabel>
                <Input id="web-search-query" value={search.query} onChange={(event) => setSearch((current) => ({ ...current, query: event.target.value }))} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); void runSearch() } }} />
              </Field>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field><FieldLabel>主题</FieldLabel><Select value={search.topic} onValueChange={(value) => setSearch((current) => ({ ...current, topic: value as WebSearchInput["topic"] }))}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectGroup><SelectItem value="general">通用</SelectItem><SelectItem value="news">新闻</SelectItem><SelectItem value="finance">金融</SelectItem></SelectGroup></SelectContent></Select></Field>
                <Field><FieldLabel>深度</FieldLabel><Select value={search.searchDepth} onValueChange={(value) => setSearch((current) => ({ ...current, searchDepth: value as WebSearchInput["searchDepth"] }))}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectGroup><SelectItem value="basic">Basic</SelectItem><SelectItem value="advanced">Advanced</SelectItem></SelectGroup></SelectContent></Select></Field>
              </div>
              <Button type="button" variant="outline" onClick={() => void runSearch()} disabled={busy !== null || !search.query.trim()}>{busy === "search" ? <Spinner /> : <Search data-icon="inline-start" />}测试检索</Button>
            </FieldGroup>
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardHeader><div className="flex items-center justify-between gap-3"><CardTitle>搜索结果</CardTitle>{result ? <Badge variant="secondary">{result.results.length} 条</Badge> : null}</div></CardHeader>
          <CardContent>
            <ScrollArea className="h-[530px] pr-4">
              {result ? <div className="flex flex-col gap-4">
                {result.answer ? <Alert><KeyRound /><AlertTitle>综合答案</AlertTitle><AlertDescription>{result.answer}</AlertDescription></Alert> : null}
                {result.results.map((item, index) => <Card key={`${item.url}-${index}`}><CardHeader><CardTitle className="text-sm leading-snug"><a href={item.url} target="_blank" rel="noreferrer" className="inline-flex items-start gap-1 hover:underline">{item.title || item.url}<ExternalLink /></a></CardTitle></CardHeader><CardContent><p className="text-sm leading-relaxed text-muted-foreground">{item.content}</p></CardContent></Card>)}
              </div> : <div className="flex h-[490px] flex-col items-center justify-center gap-2 text-center text-muted-foreground"><Search /><p>执行测试检索后在这里预览结果</p></div>}
            </ScrollArea>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
