import { useCallback, useEffect, useState } from 'react';
import {
  ArrowUp, Bot, Check, CircleAlert, Code2, ExternalLink, FileCode2,
  Globe2, LoaderCircle, MoreHorizontal, Play, Plus, RefreshCw, Settings2,
  Sparkles, Square, TerminalSquare, X,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Bubble, BubbleContent } from '@/components/ui/bubble';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupTextarea } from '@/components/ui/input-group';
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker';
import { Message, MessageContent, MessageHeader } from '@/components/ui/message';
import { MessageScroller, MessageScrollerButton, MessageScrollerContent, MessageScrollerItem, MessageScrollerProvider, MessageScrollerViewport } from '@/components/ui/message-scroller';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Spinner } from '@/components/ui/spinner';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { TooltipProvider } from '@/components/ui/tooltip';
import type { ApiResult } from '../shared/contracts/api';
import type {
  CreateWebsiteProjectInput, RuntimeSettings, RuntimeStatus, WebsiteBuilderSnapshot,
  WebsitePurpose,
} from '../shared/contracts/website-builder';

const purposeOptions: { value: WebsitePurpose; label: string }[] = [
  { value: 'brand', label: '品牌展示' }, { value: 'product', label: '产品介绍' },
  { value: 'conversion', label: '获客转化' }, { value: 'content', label: '内容发布' },
];

const initialBrief: CreateWebsiteProjectInput = {
  name: '', industry: '', offering: '', audience: '', purposes: ['brand'], notes: '',
};

export default function App() {
  const [snapshot, setSnapshot] = useState<WebsiteBuilderSnapshot>();
  const [runtime, setRuntime] = useState<RuntimeStatus>();
  const [composer, setComposer] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [newProjectOpen, setNewProjectOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const loadProject = useCallback(async (id: string) => {
    const result = await window.alvax.websiteBuilder.getProject(id);
    if (result.ok) setSnapshot(result.data); else setError(result.error.message);
  }, []);

  const load = useCallback(async () => {
    const [projectResult, runtimeResult] = await Promise.all([
      window.alvax.websiteBuilder.listProjects(), window.alvax.websiteBuilder.getRuntime(),
    ]);
    if (runtimeResult.ok) setRuntime(runtimeResult.data);
    if (projectResult.ok) {
      const selectedId = snapshot?.project.id ?? projectResult.data[0]?.id;
      if (selectedId) await loadProject(selectedId);
      else setNewProjectOpen(true);
    }
  }, [loadProject, snapshot?.project.id]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load();
    // The initial project selection is intentionally loaded once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useEffect(() => window.alvax.websiteBuilder.onEvent((event) => {
    if (event.projectId === snapshot?.project.id) void loadProject(event.projectId);
  }), [loadProject, snapshot?.project.id]);

  const perform = async <T,>(operation: () => Promise<ApiResult<T>>, onSuccess?: (value: T) => void) => {
    setBusy(true); setError('');
    try {
      const result = await operation();
      if (result.ok) onSuccess?.(result.data); else setError(result.error.message);
    } finally { setBusy(false); }
  };

  const send = async () => {
    if (!snapshot || !composer.trim()) return;
    const message = composer.trim(); setComposer('');
    await perform(() => window.alvax.websiteBuilder.sendMessage({ projectId: snapshot.project.id, message }), setSnapshot);
  };

  const status = snapshot?.project.status;
  const isGenerating = status === 'generating';

  return <TooltipProvider>
    <main className="flex h-full flex-col bg-background">
      <header className="app-titlebar window-drag-region flex h-16 shrink-0 items-center justify-between border-b bg-card/80 pr-5 backdrop-blur-xl">
        <div className="flex items-center gap-3">
          <div className="grid size-8 place-items-center rounded-lg bg-primary text-primary-foreground"><Sparkles className="size-4" /></div>
          <div><h1 className="text-sm font-semibold tracking-tight">{snapshot?.project.brief.name ?? 'Alvax AI Website Builder'}</h1><p className="text-xs text-muted-foreground">Pi Agent 驱动的网站工作室</p></div>
          {snapshot && <Badge variant="secondary" className="ml-2 font-normal">{statusLabel(status)}</Badge>}
        </div>
        <div className="window-no-drag flex items-center gap-2">
          <Button variant="ghost" size="icon-sm" onClick={() => setNewProjectOpen(true)}><Plus/><span className="sr-only">新建网站</span></Button>
          <Button variant="ghost" size="icon-sm" onClick={() => setSettingsOpen(true)}><Settings2/><span className="sr-only">Pi 设置</span></Button>
          <Button variant="ghost" size="icon-sm"><MoreHorizontal/><span className="sr-only">更多</span></Button>
        </div>
      </header>

      {error && <div className="flex items-center gap-2 border-b border-destructive/20 bg-destructive/8 px-5 py-2 text-xs text-destructive"><CircleAlert className="size-4"/><span className="flex-1">{error}</span><Button variant="ghost" size="icon-xs" onClick={() => setError('')}><X/></Button></div>}

      <ResizablePanelGroup orientation="horizontal" className="min-h-0 flex-1">
        <ResizablePanel defaultSize={62} minSize={42}>
          <section className="flex h-full min-w-0 flex-col">
            {snapshot ? <>
              <MessageScrollerProvider autoScroll defaultScrollPosition="end">
                <MessageScroller className="flex-1">
                  <MessageScrollerViewport>
                    <MessageScrollerContent className="mx-auto w-full max-w-3xl px-8 py-8">
                    <Marker variant="separator"><MarkerIcon><Sparkles/></MarkerIcon><MarkerContent>{snapshot.project.brief.industry} · {snapshot.project.brief.audience}</MarkerContent></Marker>
                    {snapshot.messages.map((message) => <MessageScrollerItem key={message.id} messageId={message.id} scrollAnchor={message.state === 'streaming'}>
                      <Message align={message.role === 'user' ? 'end' : 'start'}>
                        <MessageContent>
                          {message.role !== 'user' && <MessageHeader><Bot className="mr-1 size-3.5"/>Alvax Agent</MessageHeader>}
                          <Bubble variant={message.role === 'user' ? 'default' : message.state === 'error' ? 'destructive' : 'secondary'} align={message.role === 'user' ? 'end' : 'start'}>
                            <BubbleContent className="whitespace-pre-wrap">{message.content || <span className="flex items-center gap-2"><Spinner/>正在分析并修改网站…</span>}</BubbleContent>
                          </Bubble>
                        </MessageContent>
                      </Message>
                    </MessageScrollerItem>)}
                    </MessageScrollerContent>
                  </MessageScrollerViewport>
                  <MessageScrollerButton />
                </MessageScroller>
              </MessageScrollerProvider>

              <div className="shrink-0 bg-gradient-to-t from-background via-background to-transparent px-7 pb-6 pt-3">
                <InputGroup className="mx-auto max-w-3xl rounded-2xl bg-card shadow-lg shadow-foreground/5">
                  <InputGroupTextarea value={composer} onChange={(event) => setComposer(event.target.value)} placeholder="描述你想生成或修改的网站内容…" className="min-h-24 resize-none px-4 pt-4 text-sm" onKeyDown={(event) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void send(); } }} />
                  <InputGroupAddon align="block-end" className="justify-between px-3 pb-3">
                    <div className="flex items-center gap-2 text-xs text-muted-foreground"><Badge variant={runtime?.ready ? 'secondary' : 'outline'}>{runtime?.ready ? 'Pi 已连接' : 'Pi 未配置'}</Badge><span>Enter 发送 · Shift+Enter 换行</span></div>
                    {isGenerating ? <InputGroupButton variant="outline" size="icon-sm" onClick={() => void perform(() => window.alvax.websiteBuilder.cancel(snapshot.project.id), setSnapshot)}><Square/></InputGroupButton> : <InputGroupButton variant="default" size="icon-sm" disabled={busy || !composer.trim()} onClick={() => void send()}><ArrowUp/></InputGroupButton>}
                  </InputGroupAddon>
                </InputGroup>
              </div>
            </> : <EmptyWorkspace onCreate={() => setNewProjectOpen(true)} />}
          </section>
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel defaultSize={38} minSize={28}>
          <DeliveryPanel snapshot={snapshot} busy={busy} onRun={() => snapshot && void perform(() => window.alvax.websiteBuilder.runAcceptance(snapshot.project.id), setSnapshot)} onPreview={() => snapshot && void perform(() => window.alvax.websiteBuilder.startPreview(snapshot.project.id), setSnapshot)} />
        </ResizablePanel>
      </ResizablePanelGroup>
    </main>

    <NewProjectDialog open={newProjectOpen} onOpenChange={setNewProjectOpen} onCreate={(brief) => void perform(() => window.alvax.websiteBuilder.createProject(brief), (value) => { setSnapshot(value); setNewProjectOpen(false); void load(); })} busy={busy}/>
    {runtime && <RuntimeDialog open={settingsOpen} onOpenChange={setSettingsOpen} runtime={runtime.settings} onSave={(settings) => void perform(() => window.alvax.websiteBuilder.updateRuntime(settings), (value) => { setRuntime(value); setSettingsOpen(false); })} busy={busy}/>}
  </TooltipProvider>;
}

function DeliveryPanel({ snapshot, busy, onRun, onPreview }: { snapshot: WebsiteBuilderSnapshot | undefined; busy: boolean; onRun(): void; onPreview(): void }) {
  const passed = snapshot?.checks.filter((check) => check.status === 'passed').length ?? 0;
  return <aside className="flex h-full min-w-0 flex-col bg-card">
    <div className="flex h-14 shrink-0 items-center justify-between border-b px-4"><div><h2 className="text-sm font-semibold">交付</h2><p className="text-xs text-muted-foreground">预览、文件与验收结果</p></div>{snapshot?.preview.url && <Button variant="ghost" size="icon-sm" onClick={() => window.open(snapshot.preview.url)}><ExternalLink/></Button>}</div>
    <Tabs defaultValue="preview" className="flex min-h-0 flex-1 flex-col">
      <TabsList variant="line" className="w-full shrink-0 justify-start rounded-none border-b px-3"><TabsTrigger value="preview">预览</TabsTrigger><TabsTrigger value="files">文件 <Badge variant="secondary">{snapshot?.artifacts.length ?? 0}</Badge></TabsTrigger><TabsTrigger value="checks">验收 {snapshot?.checks.length ? `${passed}/${snapshot.checks.length}` : ''}</TabsTrigger></TabsList>
      <TabsContent value="preview" className="relative min-h-0 flex-1 p-0">
        {snapshot?.preview.url ? <iframe title="网站预览" src={snapshot.preview.url} sandbox="allow-scripts allow-forms allow-same-origin" className="size-full border-0 bg-background" /> : <div className="grid h-full place-items-center p-8 text-center"><div><div className="mx-auto grid size-12 place-items-center rounded-xl bg-muted text-muted-foreground"><Globe2/></div><h3 className="mt-4 text-sm font-medium">网站尚未启动</h3><p className="mt-2 max-w-64 text-xs leading-5 text-muted-foreground">运行验收后会自动构建并在本地启动可访问的预览服务。</p><Button className="mt-5" size="sm" disabled={!snapshot || busy} onClick={snapshot?.checks.length ? onPreview : onRun}>{busy ? <Spinner/> : <Play/>}{snapshot?.checks.length ? '启动预览' : '验收并启动'}</Button></div></div>}
      </TabsContent>
      <TabsContent value="files" className="min-h-0 flex-1 p-0"><ScrollArea className="h-full"><div className="grid gap-1 p-3">{snapshot?.artifacts.map((artifact) => <div key={artifact.path} className="flex items-center gap-2 rounded-md px-2 py-2 text-xs hover:bg-muted"><FileCode2 className="size-4 text-muted-foreground"/><code className="truncate">{artifact.path}</code><Badge variant="outline" className="ml-auto">{artifact.kind}</Badge></div>)}</div></ScrollArea></TabsContent>
      <TabsContent value="checks" className="min-h-0 flex-1 p-0"><ScrollArea className="h-full"><div className="grid gap-3 p-4">{snapshot?.checks.map((check) => <div key={check.id} className="rounded-lg border p-3"><div className="flex items-center gap-2">{check.status === 'passed' ? <Check className="size-4 text-success"/> : check.status === 'failed' ? <CircleAlert className="size-4 text-destructive"/> : check.status === 'running' ? <LoaderCircle className="size-4 animate-spin text-primary"/> : <TerminalSquare className="size-4 text-muted-foreground"/>}<span className="text-sm font-medium">{check.label}</span><Badge variant="outline" className="ml-auto">{check.status}</Badge></div>{check.summary && <pre className="mt-2 max-h-28 overflow-auto whitespace-pre-wrap rounded bg-muted p-2 text-[11px] text-muted-foreground">{check.summary}</pre>}</div>)}{snapshot && <Button variant="outline" size="sm" disabled={busy} onClick={onRun}><RefreshCw/>{snapshot.checks.length ? '重新验收' : '开始验收'}</Button>}</div></ScrollArea></TabsContent>
    </Tabs>
  </aside>;
}

function EmptyWorkspace({ onCreate }: { onCreate(): void }) { return <div className="grid h-full place-items-center"><div className="max-w-md text-center"><div className="mx-auto grid size-14 place-items-center rounded-2xl bg-primary/10 text-primary"><Code2/></div><h2 className="mt-5 text-xl font-semibold tracking-tight">从一个产品想法开始</h2><p className="mt-2 text-sm leading-6 text-muted-foreground">输入行业、服务和目标用户，Alvax AI 会创建网站结构、文案与视觉，并自动验证可运行性。</p><Button className="mt-6" onClick={onCreate}><Plus/>创建网站项目</Button></div></div>; }

function NewProjectDialog({ open, onOpenChange, onCreate, busy }: { open: boolean; onOpenChange(value: boolean): void; onCreate(brief: CreateWebsiteProjectInput): void; busy: boolean }) {
  const [brief, setBrief] = useState(initialBrief);
  const valid = brief.name && brief.industry && brief.offering && brief.audience && brief.purposes.length;
  const update = (key: keyof CreateWebsiteProjectInput, value: string) => setBrief((current) => ({ ...current, [key]: value }));
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="sm:max-w-xl"><DialogHeader><DialogTitle>创建网站项目</DialogTitle><DialogDescription>这些信息会成为 Pi Agent 生成网站的基础上下文。</DialogDescription></DialogHeader><FieldGroup className="grid gap-4 sm:grid-cols-2"><Field><FieldLabel>项目或品牌名称</FieldLabel><Input value={brief.name} onChange={(e) => update('name', e.target.value)} placeholder="例如：Nova Studio"/></Field><Field><FieldLabel>所属行业</FieldLabel><Input value={brief.industry} onChange={(e) => update('industry', e.target.value)} placeholder="例如：企业服务"/></Field><Field className="sm:col-span-2"><FieldLabel>产品或服务</FieldLabel><Input value={brief.offering} onChange={(e) => update('offering', e.target.value)} placeholder="描述你提供的核心产品或服务"/></Field><Field className="sm:col-span-2"><FieldLabel>目标用户</FieldLabel><Input value={brief.audience} onChange={(e) => update('audience', e.target.value)} placeholder="例如：正在数字化转型的中小企业管理者"/></Field><Field className="sm:col-span-2"><FieldLabel>网站用途</FieldLabel><div className="flex flex-wrap gap-2">{purposeOptions.map((option) => <Button key={option.value} type="button" variant={brief.purposes.includes(option.value) ? 'default' : 'outline'} size="sm" onClick={() => setBrief((current) => ({ ...current, purposes: current.purposes.includes(option.value) ? current.purposes.filter((item) => item !== option.value) : [...current.purposes, option.value] }))}>{brief.purposes.includes(option.value) && <Check/>}{option.label}</Button>)}</div></Field><Field className="sm:col-span-2"><FieldLabel>补充说明</FieldLabel><Input value={brief.notes} onChange={(e) => update('notes', e.target.value)} placeholder="品牌调性、差异化、希望包含的页面等（可选）"/></Field></FieldGroup><DialogFooter><Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button disabled={!valid || busy} onClick={() => onCreate(brief)}>{busy ? <Spinner/> : <Sparkles/>}创建并生成</Button></DialogFooter></DialogContent></Dialog>;
}

function RuntimeDialog({ open, onOpenChange, runtime, onSave, busy }: { open: boolean; onOpenChange(value: boolean): void; runtime: RuntimeSettings; onSave(value: RuntimeSettings): void; busy: boolean }) {
  const [value, setValue] = useState(runtime);
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent><DialogHeader><DialogTitle>Pi Agent 运行时</DialogTitle><DialogDescription>路径只保存在本机 Electron userData 中，不会传给渲染进程以外的服务。</DialogDescription></DialogHeader><FieldGroup><Field><FieldLabel>Node.js 可执行文件</FieldLabel><Input value={value.nodePath} onChange={(e) => setValue({ ...value, nodePath: e.target.value })}/><FieldDescription>建议使用 Node.js 24。</FieldDescription></Field><Field><FieldLabel>Pi CLI 文件</FieldLabel><Input value={value.piPath} onChange={(e) => setValue({ ...value, piPath: e.target.value })}/></Field><div className="grid grid-cols-2 gap-3"><Field><FieldLabel>Provider（可选）</FieldLabel><Input value={value.provider} onChange={(e) => setValue({ ...value, provider: e.target.value })} placeholder="anthropic"/></Field><Field><FieldLabel>Model（可选）</FieldLabel><Input value={value.model} onChange={(e) => setValue({ ...value, model: e.target.value })} placeholder="claude-sonnet…"/></Field></div></FieldGroup><DialogFooter><Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button disabled={busy} onClick={() => onSave(value)}>{busy ? <Spinner/> : <Check/>}保存并检测</Button></DialogFooter></DialogContent></Dialog>;
}

function statusLabel(status?: string): string { return ({ ready: '待命', generating: '生成中', checking: '验收中', previewing: '预览运行中', failed: '需要处理' } as Record<string, string>)[status ?? ''] ?? '待命'; }
