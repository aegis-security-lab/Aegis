import { useCallback, useEffect, useRef, useState } from 'react';
import {
  ArrowUp, Bot, Check, ChevronDown, ChevronLeft, ChevronRight, CircleAlert,
  Globe2, LoaderCircle, MoreHorizontal, PanelsTopLeft, Play, Plus, Settings2,
  Sparkles, Square, TerminalSquare, X,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Bubble, BubbleContent } from '@/components/ui/bubble';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupTextarea } from '@/components/ui/input-group';
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker';
import { Message, MessageContent, MessageHeader } from '@/components/ui/message';
import { MessageScroller, MessageScrollerButton, MessageScrollerContent, MessageScrollerItem, MessageScrollerProvider, MessageScrollerViewport } from '@/components/ui/message-scroller';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import { Spinner } from '@/components/ui/spinner';
import { TooltipProvider } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import type { ApiResult } from '../shared/contracts/api';
import type {
  ChatMessage, CreateWebsiteProjectInput, RuntimeSettings, RuntimeStatus, WebsiteBuilderSnapshot,
  WebsitePurpose,
} from '../shared/contracts/website-builder';
import alvaxStudioIcon from '../../assets/icons/alvax-studio.png';

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
  const [deliveryOpen, setDeliveryOpen] = useState(false);
  const previewSeen = useRef(new Map<string, boolean>());

  const loadProject = useCallback(async (id: string) => {
    const result = await window.alvax.websiteBuilder.getProject(id);
    if (result.ok) {
      const hadPreview = previewSeen.current.get(id);
      previewSeen.current.set(id, Boolean(result.data.preview.url));
      setSnapshot(result.data);
      if (hadPreview === false && result.data.preview.url) {
        setDeliveryOpen(true);
      }
    } else setError(result.error.message);
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

  const toggleDelivery = () => {
    const nextOpen = !deliveryOpen;
    setDeliveryOpen(nextOpen);
    if (nextOpen && snapshot?.preview.url) {
      void perform(() => window.alvax.websiteBuilder.startPreview(snapshot.project.id), setSnapshot);
    }
  };

  const status = snapshot?.project.status;
  const isGenerating = status === 'generating';

  return <TooltipProvider>
    <main className="flex h-full flex-col bg-background">
      <header className="app-titlebar window-drag-region flex h-16 shrink-0 items-center justify-between border-b bg-card/80 pr-5 backdrop-blur-xl">
        <div className="flex items-center gap-3">
          <img src={alvaxStudioIcon} alt="" className="size-9 rounded-xl" />
          <div><h1 className="text-sm font-semibold tracking-tight">Alvax Studio</h1><p className="text-xs text-muted-foreground">{snapshot ? `${snapshot.project.brief.name} · Pi Agent 官网工作室` : 'Pi Agent 驱动的官网工作室'}</p></div>
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
        <ResizablePanel defaultSize={deliveryOpen ? 62 : 100} minSize={42}>
          <section className="relative flex h-full min-w-0 flex-col">
            {snapshot && <Button variant="outline" size="icon-sm" className="absolute right-4 top-4 z-10 rounded-full bg-background/90 shadow-sm backdrop-blur" onClick={toggleDelivery}>{deliveryOpen ? <ChevronRight/> : <ChevronLeft/>}<span className="sr-only">{deliveryOpen ? '折叠侧栏' : '展开侧栏'}</span></Button>}
            {snapshot ? <>
              <MessageScrollerProvider autoScroll defaultScrollPosition="end">
                <MessageScroller className="flex-1">
                  <MessageScrollerViewport>
                    <MessageScrollerContent className="mx-auto w-full max-w-3xl px-8 py-8">
                    <Marker variant="separator"><MarkerIcon><Sparkles/></MarkerIcon><MarkerContent>{snapshot.project.brief.industry} · {snapshot.project.brief.audience}</MarkerContent></Marker>
                    <ChatTimeline messages={snapshot.messages} active={snapshot.project.status === 'generating'} />
                    </MessageScrollerContent>
                  </MessageScrollerViewport>
                  <MessageScrollerButton />
                </MessageScroller>
              </MessageScrollerProvider>

              <div className="shrink-0 bg-gradient-to-t from-background via-background to-transparent px-7 pb-6 pt-3">
                <div className="mx-auto max-w-3xl">
                {snapshot.project.status === 'checking' && <AcceptanceDock snapshot={snapshot} />}
                <InputGroup className={cn('bg-card shadow-lg shadow-foreground/5', snapshot.project.status === 'checking' ? 'rounded-b-2xl rounded-t-none border-t-0' : 'rounded-2xl')}>
                  <InputGroupTextarea value={composer} onChange={(event) => setComposer(event.target.value)} placeholder="描述你想生成或修改的网站内容…" className="min-h-24 resize-none px-4 pt-4 text-sm" onKeyDown={(event) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void send(); } }} />
                  <InputGroupAddon align="block-end" className="justify-between px-3 pb-3">
                    <span className="text-xs text-muted-foreground">Enter 发送 · Shift+Enter 换行</span>
                    {isGenerating ? <InputGroupButton variant="outline" size="icon-sm" onClick={() => void perform(() => window.alvax.websiteBuilder.cancel(snapshot.project.id), setSnapshot)}><Square/></InputGroupButton> : <InputGroupButton variant="default" size="icon-sm" disabled={busy || !composer.trim()} onClick={() => void send()}><ArrowUp/></InputGroupButton>}
                  </InputGroupAddon>
                </InputGroup>
                </div>
              </div>
            </> : <EmptyWorkspace onCreate={() => setNewProjectOpen(true)} />}
          </section>
        </ResizablePanel>
        {deliveryOpen && <><ResizableHandle withHandle /><ResizablePanel defaultSize={38} minSize={28}>
          <DeliveryPanel snapshot={snapshot} busy={busy} onRun={() => snapshot && void perform(() => window.alvax.websiteBuilder.runAcceptance(snapshot.project.id), setSnapshot)} onPreview={() => snapshot && void perform(() => window.alvax.websiteBuilder.startPreview(snapshot.project.id), setSnapshot)} onOpenWindow={() => snapshot && void perform(() => window.alvax.websiteBuilder.openPreviewWindow(snapshot.project.id))} />
        </ResizablePanel></>}
      </ResizablePanelGroup>
    </main>

    <NewProjectDialog open={newProjectOpen} onOpenChange={setNewProjectOpen} onCreate={(brief) => void perform(() => window.alvax.websiteBuilder.createProject(brief), (value) => { setDeliveryOpen(false); setSnapshot(value); setNewProjectOpen(false); void load(); })} busy={busy}/>
    {runtime && <RuntimeDialog open={settingsOpen} onOpenChange={setSettingsOpen} runtime={runtime.settings} onSave={(settings) => void perform(() => window.alvax.websiteBuilder.updateRuntime(settings), (value) => { setRuntime(value); setSettingsOpen(false); })} busy={busy}/>}
  </TooltipProvider>;
}

function ChatTimeline({ messages, active }: { messages: ChatMessage[]; active: boolean }) {
  const turns = groupMessagesByTurn(messages);
  return turns.map((turn, index) => {
    const isActive = active && index === turns.length - 1;
    const user = turn[0]?.role === 'user' ? turn[0] : undefined;
    const responses = user ? turn.slice(1) : turn;
    const final = isActive ? undefined : [...responses].reverse().find((message) => message.role === 'assistant' && message.content.trim());
    const history = final ? responses.filter((message) => message.id !== final.id) : responses;
    return <MessageScrollerItem key={turn[0]?.id ?? index} messageId={turn.at(-1)!.id} scrollAnchor={isActive}>
      <div className="flex flex-col gap-3">
        {user && <ChatEntry message={user} />}
        {history.length > 0 && <AgentProcess messages={history} active={isActive} />}
        {final && <ChatEntry message={final} />}
      </div>
    </MessageScrollerItem>;
  });
}

function AgentProcess({ messages, active }: { messages: ChatMessage[]; active: boolean }) {
  const [historyOpen, setHistoryOpen] = useState(false);
  const processViewport = useRef<HTMLDivElement>(null);
  const open = active || historyOpen;
  useEffect(() => {
    if (active && processViewport.current) {
      processViewport.current.scrollTop = processViewport.current.scrollHeight;
    }
  }, [active, messages]);
  return <Collapsible open={open} onOpenChange={(value) => { if (!active) setHistoryOpen(value); }} className="ml-3 max-w-[88%]">
    <CollapsibleTrigger render={<Button variant="ghost" size="xs" />} className="text-muted-foreground">
      {active ? <LoaderCircle className="animate-spin"/> : <ChevronDown className={cn('transition-transform', open && 'rotate-180')}/>} {active ? 'Agent 正在执行' : `查看执行过程 · ${messages.length} 条`}
    </CollapsibleTrigger>
    <CollapsibleContent className="mt-1 overflow-hidden data-[ending-style]:animate-out data-[starting-style]:animate-in">
      <div ref={processViewport} className="flex max-h-56 flex-col gap-2 overflow-y-auto py-1 pl-1 pr-3">
        {messages.map((message) => <ChatEntry key={message.id} message={message} compact />)}
      </div>
    </CollapsibleContent>
  </Collapsible>;
}

function ChatEntry({ message, compact = false }: { message: ChatMessage; compact?: boolean }) {
  if (message.role === 'system') return <Marker variant={compact ? 'default' : 'border'}><MarkerIcon>{message.state === 'error' ? <CircleAlert/> : <Sparkles/>}</MarkerIcon><MarkerContent className="whitespace-pre-wrap text-xs">{message.content}</MarkerContent></Marker>;
  const isUser = message.role === 'user';
  const isTool = message.role === 'tool';
  if (compact) return <Message>
    <MessageContent>
      <div className={cn('flex min-w-0 items-start gap-2 text-xs leading-5 text-muted-foreground', message.state === 'error' && 'text-destructive')}>
        {isTool ? <TerminalSquare className="mt-0.5 size-3.5 shrink-0"/> : <Bot className="mt-0.5 size-3.5 shrink-0"/>}
        {isTool ? <span
          className="min-w-0 flex-1 cursor-ew-resize overflow-x-auto whitespace-nowrap font-mono [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          title={message.content}
          onWheel={(event) => { event.currentTarget.scrollLeft += event.deltaY || event.deltaX; }}
        ><span className="inline-block w-max whitespace-nowrap">{message.content || '正在执行…'}</span></span> : <span className="min-w-0 whitespace-pre-wrap">{message.content || '正在思考…'}</span>}
        {message.state === 'streaming' && !isTool && <span className="mt-1 inline-block h-3 w-px shrink-0 animate-pulse bg-current"/>}
      </div>
    </MessageContent>
  </Message>;
  return <Message align={isUser ? 'end' : 'start'}>
    <MessageContent>
      {!isUser && !compact && <MessageHeader>{isTool ? <TerminalSquare className="mr-1 size-3.5"/> : <Bot className="mr-1 size-3.5"/>}{isTool ? 'Agent 工具' : 'Alvax Agent'}</MessageHeader>}
      <Bubble variant={isUser ? 'default' : message.state === 'error' ? 'destructive' : 'ghost'} align={isUser ? 'end' : 'start'}>
        <BubbleContent className={cn('whitespace-pre-wrap', isTool && 'truncate font-mono text-xs')}>{message.content || <span className="flex items-center gap-2"><Spinner/>正在思考…</span>}{message.state === 'streaming' && message.content && !isTool && <span className="ml-0.5 inline-block h-4 w-px animate-pulse bg-current align-middle"/>}</BubbleContent>
      </Bubble>
    </MessageContent>
  </Message>;
}

function groupMessagesByTurn(messages: ChatMessage[]): ChatMessage[][] {
  const turns: ChatMessage[][] = [];
  for (const message of messages) {
    if (message.role === 'user' || turns.length === 0) turns.push([message]);
    else turns.at(-1)!.push(message);
  }
  return turns;
}

function AcceptanceDock({ snapshot }: { snapshot: WebsiteBuilderSnapshot }) {
  const current = snapshot.checks.find((check) => check.status === 'running')
    ?? [...snapshot.checks].reverse().find((check) => check.status === 'passed' || check.status === 'failed');
  if (!current) return null;
  return <div className="flex min-h-12 items-center gap-3 overflow-hidden rounded-t-2xl border bg-muted/40 px-4 py-2 shadow-lg shadow-foreground/5">
    <span className="text-xs font-medium text-muted-foreground">自动验收</span>
    <div key={`${current.id}-${current.status}`} className="flex min-w-0 animate-in items-center gap-2 duration-300 slide-in-from-bottom-2">
      {current.status === 'passed' ? <Check className="size-4 text-success"/> : current.status === 'failed' ? <CircleAlert className="size-4 text-destructive"/> : <LoaderCircle className="size-4 animate-spin text-primary"/>}
      <span className="truncate text-sm">{current.label}</span>
      <span className="text-xs text-muted-foreground">{current.status === 'passed' ? '已通过' : current.status === 'failed' ? '未通过' : '检查中…'}</span>
    </div>
  </div>;
}

function DeliveryPanel({ snapshot, busy, onRun, onPreview, onOpenWindow }: { snapshot: WebsiteBuilderSnapshot | undefined; busy: boolean; onRun(): void; onPreview(): void; onOpenWindow(): void }) {
  return <aside className="flex h-full min-w-0 flex-col bg-card">
      <div className="flex h-11 shrink-0 items-center justify-between border-b px-4"><span className="text-sm font-medium">预览</span>{snapshot?.preview.url && <Button variant="ghost" size="icon-sm" onClick={onOpenWindow}><PanelsTopLeft/><span className="sr-only">在新窗口中打开预览</span></Button>}</div>
      <div className="relative min-h-0 flex-1">
        {snapshot?.preview.url ? <><iframe key={`${snapshot.preview.url}-${snapshot.project.updatedAt}`} title="网站预览" src={snapshot.preview.url} sandbox="allow-scripts allow-forms allow-same-origin" className="size-full border-0 bg-background" />{busy && <div className="pointer-events-none absolute inset-0 grid place-items-center bg-background/60 backdrop-blur-sm"><Spinner/></div>}</> : <div className="grid h-full place-items-center p-8 text-center"><div><div className="mx-auto grid size-12 place-items-center rounded-xl bg-muted text-muted-foreground"><Globe2/></div><h3 className="mt-4 text-sm font-medium">网站尚未启动</h3><p className="mt-2 max-w-64 text-xs leading-5 text-muted-foreground">运行验收后会自动构建并在本地启动可访问的预览服务。</p><Button className="mt-5" size="sm" disabled={!snapshot || busy} onClick={snapshot?.checks.length ? onPreview : onRun}>{busy ? <Spinner/> : <Play/>}{snapshot?.checks.length ? '启动预览' : '验收并启动'}</Button></div></div>}
      </div>
  </aside>;
}

function EmptyWorkspace({ onCreate }: { onCreate(): void }) { return <div className="grid h-full place-items-center"><div className="max-w-md text-center"><img src={alvaxStudioIcon} alt="" className="mx-auto size-16 rounded-2xl"/><h2 className="mt-5 text-xl font-semibold tracking-tight">从一个产品想法开始</h2><p className="mt-2 text-sm leading-6 text-muted-foreground">输入行业、服务和目标用户，Alvax Studio 会创建网站结构、文案与视觉，并自动验证可运行性。</p><Button className="mt-6" onClick={onCreate}><Plus/>创建网站项目</Button></div></div>; }

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
