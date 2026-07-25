import { useCallback, useEffect, useRef, useState } from 'react';
import {
  ArrowUp, Bot, Check, ChevronDown, ChevronLeft, ChevronRight, CircleAlert,
  Globe2, LoaderCircle, MessagesSquare, PanelsTopLeft, Play, Plus,
  Sparkles, Square, TerminalSquare, Trash2, X,
} from 'lucide-react';
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Bubble, BubbleContent } from '@/components/ui/bubble';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupTextarea } from '@/components/ui/input-group';
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker';
import { Message, MessageContent, MessageHeader } from '@/components/ui/message';
import { MessageScroller, MessageScrollerButton, MessageScrollerContent, MessageScrollerItem, MessageScrollerProvider, MessageScrollerViewport } from '@/components/ui/message-scroller';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Spinner } from '@/components/ui/spinner';
import { TooltipProvider } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import type { ApiResult } from '../shared/contracts/api';
import type {
  ChatMessage, CreateWebsiteProjectInput, WebsiteBuilderSnapshot, WebsiteProject,
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
  const [projects, setProjects] = useState<WebsiteProject[]>([]);
  const [composer, setComposer] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [newProjectOpen, setNewProjectOpen] = useState(false);
  const [deliveryOpen, setDeliveryOpen] = useState(false);
  const [sessionsOpen, setSessionsOpen] = useState(false);
  const previewRunningSeen = useRef(new Map<string, boolean>());
  const composerIsComposing = useRef(false);
  const sessionCloseTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const loadProject = useCallback(async (id: string): Promise<WebsiteBuilderSnapshot | undefined> => {
    const result = await window.alvax.websiteBuilder.getProject(id);
    if (result.ok) {
      const wasPreviewing = previewRunningSeen.current.get(id);
      const isPreviewing = result.data.project.status === 'previewing' && result.data.preview.status === 'running';
      previewRunningSeen.current.set(id, isPreviewing);
      setSnapshot(result.data);
      setProjects((current) => current.map((project) => project.id === id ? result.data.project : project));
      if (wasPreviewing === false && isPreviewing) {
        setDeliveryOpen(true);
      }
      return result.data;
    } else setError(result.error.message);
    return undefined;
  }, []);

  const load = useCallback(async () => {
    const projectResult = await window.alvax.websiteBuilder.listProjects();
    if (projectResult.ok) {
      setProjects(projectResult.data);
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
  useEffect(() => () => {
    if (sessionCloseTimer.current) clearTimeout(sessionCloseTimer.current);
  }, []);

  const perform = async <T,>(operation: () => Promise<ApiResult<T>>, onSuccess?: (value: T) => void | Promise<void>) => {
    setBusy(true); setError('');
    try {
      const result = await operation();
      if (result.ok) await onSuccess?.(result.data); else setError(result.error.message);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : '操作失败，请查看本地日志。';
      setError(message);
      void window.alvax.system.logError({
        scope: 'Renderer operation',
        message,
        ...(cause instanceof Error && cause.stack ? { stack: cause.stack } : {}),
      });
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

  const openSessions = () => {
    if (sessionCloseTimer.current) clearTimeout(sessionCloseTimer.current);
    setSessionsOpen(true);
  };

  const scheduleSessionsClose = () => {
    sessionCloseTimer.current = setTimeout(() => setSessionsOpen(false), 180);
  };

  const selectProject = async (id: string) => {
    setSessionsOpen(false);
    setComposer('');
    const selected = await loadProject(id);
    if (deliveryOpen && selected?.preview.url) {
      await perform(() => window.alvax.websiteBuilder.startPreview(id), setSnapshot);
    }
  };

  const removeProject = async (id: string) => {
    const remaining = projects.filter((project) => project.id !== id);
    await perform(() => window.alvax.websiteBuilder.removeProject(id), async () => {
      previewRunningSeen.current.delete(id);
      setProjects(remaining);
      if (snapshot?.project.id !== id) return;
      setDeliveryOpen(false);
      const next = remaining[0];
      if (next) await loadProject(next.id);
      else {
        setSnapshot(undefined);
        setNewProjectOpen(true);
      }
    });
  };

  const status = snapshot?.project.status;
  const isGenerating = status === 'generating';

  return <TooltipProvider>
    <main className="flex h-full flex-col bg-background">
      <div className="fixed bottom-0 left-0 top-12 z-40 w-2" onMouseEnter={openSessions} aria-hidden="true" />
      <header className="app-titlebar window-drag-region flex h-12 shrink-0 items-center justify-between border-b bg-card/80 pr-4 backdrop-blur-xl">
        <h1 className="text-sm font-semibold tracking-tight">Alvax Studio</h1>
        <div className="window-no-drag flex items-center gap-2">
          <Button variant="ghost" size="icon-sm" onClick={() => setNewProjectOpen(true)}><Plus/><span className="sr-only">新建网站</span></Button>
        </div>
      </header>

      {error && <div className="flex items-center gap-2 border-b border-destructive/20 bg-destructive/8 px-5 py-2 text-xs text-destructive"><CircleAlert className="size-4"/><span className="flex-1">{error}</span><Button variant="ghost" size="icon-xs" onClick={() => setError('')}><X/></Button></div>}

      <ResizablePanelGroup orientation="horizontal" className="min-h-0 flex-1">
        <ResizablePanel defaultSize={deliveryOpen ? 62 : 100} minSize={42}>
          <section className="relative flex h-full min-w-0 flex-col">
            {snapshot && <Button variant="outline" size="icon-sm" className={cn('absolute right-4 top-3 z-10 rounded-full bg-background/90 shadow-sm backdrop-blur', snapshot.preview.status === 'running' && 'border-success/40 bg-success/10 text-success hover:bg-success/15 hover:text-success')} onClick={toggleDelivery}>{deliveryOpen ? <ChevronRight/> : <ChevronLeft/>}<span className="sr-only">{deliveryOpen ? '折叠侧栏' : '展开侧栏'}{snapshot.preview.status === 'running' ? '，预览运行中' : ''}</span></Button>}
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
                  <InputGroupTextarea
                    value={composer}
                    onChange={(event) => setComposer(event.target.value)}
                    onCompositionStart={() => { composerIsComposing.current = true; }}
                    onCompositionEnd={() => { composerIsComposing.current = false; }}
                    onKeyDown={(event) => {
                      const isComposing = composerIsComposing.current
                        || event.nativeEvent.isComposing
                        || event.nativeEvent.keyCode === 229;
                      if (event.key === 'Enter' && !event.shiftKey && !isComposing) {
                        event.preventDefault();
                        void send();
                      }
                    }}
                    placeholder="描述你想生成或修改的网站内容…"
                    className="min-h-24 resize-none px-4 pt-4 text-sm"
                  />
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

    <SessionDrawer open={sessionsOpen} projects={projects} selectedId={snapshot?.project.id} busy={busy} onOpenChange={setSessionsOpen} onMouseEnter={openSessions} onMouseLeave={scheduleSessionsClose} onSelect={(id) => void selectProject(id)} onDelete={(id) => void removeProject(id)} />
    <NewProjectDialog open={newProjectOpen} onOpenChange={setNewProjectOpen} onCreate={(brief) => void perform(() => window.alvax.websiteBuilder.createProject(brief), (value) => {
      setDeliveryOpen(false);
      setSnapshot(value);
      setProjects((current) => [value.project, ...current.filter((project) => project.id !== value.project.id)]);
      setNewProjectOpen(false);
    })} busy={busy}/>
  </TooltipProvider>;
}

function SessionDrawer({ open, projects, selectedId, busy, onOpenChange, onMouseEnter, onMouseLeave, onSelect, onDelete }: {
  open: boolean;
  projects: WebsiteProject[];
  selectedId: string | undefined;
  busy: boolean;
  onOpenChange(value: boolean): void;
  onMouseEnter(): void;
  onMouseLeave(): void;
  onSelect(id: string): void;
  onDelete(id: string): void;
}) {
  const [deleteTarget, setDeleteTarget] = useState<WebsiteProject>();
  return <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
    <SheetContent side="left" showCloseButton={false} showOverlay={false} className="w-80 gap-0 p-0 sm:max-w-80" onMouseEnter={onMouseEnter} onMouseLeave={onMouseLeave}>
      <SheetHeader className="border-b px-4 pb-3 pt-14">
        <div className="flex items-center gap-2"><MessagesSquare className="size-4 text-muted-foreground"/><SheetTitle>会话</SheetTitle></div>
        <SheetDescription className="text-xs">切换网站会话和对应预览</SheetDescription>
      </SheetHeader>
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col gap-1 p-2">
          {projects.map((project) => <div key={project.id} className="group/session relative">
            <Button
              variant={project.id === selectedId ? 'secondary' : 'ghost'}
              className="h-auto w-full justify-start py-2.5 pl-3 pr-10 text-left"
              onClick={() => onSelect(project.id)}
            >
              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="truncate text-sm font-medium">{project.brief.name}</span>
                <span className="truncate text-xs font-normal text-muted-foreground">{project.brief.industry} · {formatSessionTime(project.updatedAt)}</span>
              </span>
              {project.status === 'previewing' && <span className="size-1.5 shrink-0 rounded-full bg-success transition-opacity group-hover/session:opacity-0"/>}
            </Button>
            <Button variant="ghost" size="icon-sm" className="absolute right-2 top-1/2 -translate-y-1/2 opacity-0 group-hover/session:opacity-100 focus-visible:opacity-100" disabled={busy} onClick={() => setDeleteTarget(project)}>
              <Trash2/><span className="sr-only">删除 {project.brief.name}</span>
            </Button>
          </div>)}
        </div>
      </ScrollArea>
    </SheetContent>
    <AlertDialog open={Boolean(deleteTarget)} onOpenChange={(value) => { if (!value) setDeleteTarget(undefined); }}>
      <AlertDialogContent size="sm" onMouseEnter={onMouseEnter}>
        <AlertDialogHeader>
          <AlertDialogTitle>删除“{deleteTarget?.brief.name}”？</AlertDialogTitle>
          <AlertDialogDescription>会话记录和本地生成的网站代码将被永久删除，此操作无法撤销。</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction variant="destructive" disabled={busy} onClick={() => { if (deleteTarget) onDelete(deleteTarget.id); setDeleteTarget(undefined); }}>删除</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </Sheet>;
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

function formatSessionTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value));
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
  if (message.role === 'system') return <Marker variant={compact ? 'default' : 'border'} className={cn(compact && 'shrink-0')}><MarkerIcon>{message.state === 'error' ? <CircleAlert/> : <Sparkles/>}</MarkerIcon><MarkerContent className="whitespace-pre-wrap text-xs">{message.content}</MarkerContent></Marker>;
  const isUser = message.role === 'user';
  const isTool = message.role === 'tool';
  const displayContent = isTool ? normalizeToolMessage(message.content) : message.content;
  if (compact) return <Message className="shrink-0">
    <MessageContent>
      <div className={cn('flex min-w-0 items-start gap-2 text-xs leading-5 text-muted-foreground', message.state === 'error' && 'text-destructive')}>
        {isTool ? <TerminalSquare className="mt-0.5 size-3.5 shrink-0"/> : <Bot className="mt-0.5 size-3.5 shrink-0"/>}
        {isTool ? <span
          className="min-w-0 flex-1 cursor-ew-resize overflow-x-auto whitespace-nowrap font-mono [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          title={displayContent}
          onWheel={(event) => { event.currentTarget.scrollLeft += event.deltaY || event.deltaX; }}
        ><span className="inline-block w-max whitespace-nowrap">{displayContent || '正在执行…'}</span></span> : <span className="min-w-0 whitespace-pre-wrap">{displayContent || '正在思考…'}</span>}
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

function normalizeToolMessage(content: string): string {
  return content.replace(/^工具 · (?:Edit|Read|Write|终端) · /, '工具 · ');
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
