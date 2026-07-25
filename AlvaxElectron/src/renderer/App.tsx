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
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupTextarea } from '@/components/ui/input-group';
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker';
import { Message, MessageContent, MessageHeader } from '@/components/ui/message';
import { MessageScroller, MessageScrollerButton, MessageScrollerContent, MessageScrollerItem, MessageScrollerProvider, MessageScrollerViewport } from '@/components/ui/message-scroller';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Spinner } from '@/components/ui/spinner';
import { Textarea } from '@/components/ui/textarea';
import { TooltipProvider } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { MarkdownContent } from '@/components/markdown-content';
import type { ApiResult } from '../shared/contracts/api';
import type {
  ChatMessage, CreateWebsiteProjectInput, WebsiteBuilderSnapshot, WebsiteProject,
} from '../shared/contracts/website-builder';
import { WEBSITE_BRIEF_MESSAGE_PREFIX } from '../shared/contracts/website-builder';
import { ALVAX_CONFIRMATION_PREFIX, ALVAX_KEY_INFO_PREFIX } from '../shared/contracts/website-builder';
import alvaxStudioIcon from '../../assets/icons/alvax-studio.png';

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

  const respondToConfirmation = async (toolCallId: string, approved: boolean, suggestion: string) => {
    if (!snapshot) return;
    setBusy(true); setError('');
    try {
      const response = await window.alvax.websiteBuilder.respondToConfirmation({
        projectId: snapshot.project.id, toolCallId, approved, suggestion,
      });
      if (!response.ok) { setError(response.error.message); return; }
      if (!approved) {
        const cancelled = await window.alvax.websiteBuilder.cancel(snapshot.project.id);
        if (cancelled.ok) setSnapshot(cancelled.data); else setError(cancelled.error.message);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '确认操作失败。');
    } finally { setBusy(false); }
  };

  const status = snapshot?.project.status;
  const isGenerating = status === 'generating';

  return <TooltipProvider>
    <main className="flex h-full flex-col bg-background">
      <div className="fixed bottom-0 left-0 top-12 z-40 w-2" onMouseEnter={openSessions} aria-hidden="true" />
      <header className="app-titlebar window-drag-region flex h-12 shrink-0 items-center justify-between border-b bg-card/80 pr-4 backdrop-blur-xl">
        <h1 className="text-sm font-semibold tracking-tight">Alvax Studio</h1>
        <div className="window-no-drag flex items-center gap-2">
          <Button variant="ghost" size="icon-sm" onClick={() => setNewProjectOpen(true)}><Plus/><span className="sr-only">升级现有网站</span></Button>
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
                    <ChatTimeline messages={snapshot.messages} active={snapshot.project.status === 'generating'} busy={busy} onConfirmation={(toolCallId, approved, suggestion) => void respondToConfirmation(toolCallId, approved, suggestion)} />
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

function ChatTimeline({ messages, active, busy, onConfirmation }: { messages: ChatMessage[]; active: boolean; busy: boolean; onConfirmation(toolCallId: string, approved: boolean, suggestion: string): void }) {
  const turns = groupMessagesByTurn(messages);
  return turns.map((turn, index) => {
    const isActive = active && index === turns.length - 1;
    const user = turn[0]?.role === 'user' ? turn[0] : undefined;
    const responses = user ? turn.slice(1) : turn;
    const final = isActive ? undefined : [...responses].reverse().find((message) =>
      (message.role === 'assistant' && message.content.trim()) || isFinalDeliveryMessage(message),
    );
    const history = final ? responses.filter((message) => message.id !== final.id) : responses;
    return <MessageScrollerItem key={turn[0]?.id ?? index} messageId={turn.at(-1)!.id} scrollAnchor={isActive}>
      <div className="flex flex-col gap-3">
        {user && <ChatEntry message={user} />}
        {history.length > 0 && <TimelineHistory messages={history} active={isActive} busy={busy} onConfirmation={onConfirmation} />}
        {final && <ChatEntry message={final} busy={busy} onConfirmation={onConfirmation} />}
      </div>
    </MessageScrollerItem>;
  });
}

function TimelineHistory({ messages, active, busy, onConfirmation }: { messages: ChatMessage[]; active: boolean; busy: boolean; onConfirmation(toolCallId: string, approved: boolean, suggestion: string): void }) {
  const groups: Array<{ kind: 'pinned'; message: ChatMessage } | { kind: 'process'; messages: ChatMessage[] }> = [];
  for (const message of messages) {
    if (isPinnedAgentMessage(message)) {
      groups.push({ kind: 'pinned', message });
      continue;
    }
    const previous = groups.at(-1);
    if (previous?.kind === 'process') previous.messages.push(message);
    else groups.push({ kind: 'process', messages: [message] });
  }
  return groups.map((group, index) => group.kind === 'pinned'
    ? <ChatEntry key={group.message.id} message={group.message} busy={busy} onConfirmation={onConfirmation} />
    : <AgentProcess key={group.messages[0]!.id} messages={group.messages} active={active && index === groups.length - 1} busy={busy} onConfirmation={onConfirmation} />);
}

function isPinnedAgentMessage(message: ChatMessage): boolean {
  return message.role === 'tool'
    && (message.content.startsWith(ALVAX_KEY_INFO_PREFIX) || message.content.startsWith(ALVAX_CONFIRMATION_PREFIX));
}

function isFinalDeliveryMessage(message: ChatMessage): boolean {
  return message.role === 'tool'
    && message.content.startsWith(ALVAX_KEY_INFO_PREFIX)
    && message.content.includes('"type":"final_delivery"');
}

function formatSessionTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value));
}

function AgentProcess({ messages, active, busy, onConfirmation }: { messages: ChatMessage[]; active: boolean; busy: boolean; onConfirmation(toolCallId: string, approved: boolean, suggestion: string): void }) {
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
        {messages.map((message) => <ChatEntry key={message.id} message={message} compact busy={busy} onConfirmation={onConfirmation} />)}
      </div>
    </CollapsibleContent>
  </Collapsible>;
}

function ChatEntry({ message, compact = false, busy = false, onConfirmation = () => undefined }: { message: ChatMessage; compact?: boolean; busy?: boolean; onConfirmation?(toolCallId: string, approved: boolean, suggestion: string): void }) {
  if (message.role === 'system') return <Marker variant={compact ? 'default' : 'border'} className={cn(compact && 'shrink-0')}><MarkerIcon>{message.state === 'error' ? <CircleAlert/> : <Sparkles/>}</MarkerIcon><MarkerContent className="whitespace-pre-wrap text-xs">{message.content}</MarkerContent></Marker>;
  const isUser = message.role === 'user';
  const isTool = message.role === 'tool';
  if (isTool && message.content.startsWith(ALVAX_KEY_INFO_PREFIX)) {
    return <KeyInfoCard content={message.content.slice(ALVAX_KEY_INFO_PREFIX.length)} />;
  }
  if (isTool && message.content.startsWith(ALVAX_CONFIRMATION_PREFIX)) {
    return <ConfirmationCard content={message.content.slice(ALVAX_CONFIRMATION_PREFIX.length)} pending={message.state === 'streaming'} busy={busy} onRespond={onConfirmation} />;
  }
  if (isUser && message.content.startsWith(WEBSITE_BRIEF_MESSAGE_PREFIX)) {
    return <RequirementCard content={message.content} />;
  }
  const displayContent = isTool ? normalizeToolMessage(message.content) : message.content;
  if (compact) return <Message className="shrink-0">
    <MessageContent>
      <div className={cn('flex min-w-0 items-start gap-2 text-xs leading-5 text-muted-foreground', message.state === 'error' && 'text-destructive')}>
        {isTool ? <TerminalSquare className="mt-0.5 size-3.5 shrink-0"/> : <Bot className="mt-0.5 size-3.5 shrink-0"/>}
        {isTool ? <span
          className="min-w-0 flex-1 cursor-ew-resize overflow-x-auto whitespace-nowrap font-mono [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          title={displayContent}
          onWheel={(event) => { event.currentTarget.scrollLeft += event.deltaY || event.deltaX; }}
        ><span className="inline-block w-max whitespace-nowrap">{displayContent || '正在执行…'}</span></span> : <MarkdownContent compact className="flex-1">{displayContent || '正在思考…'}</MarkdownContent>}
        {message.state === 'streaming' && !isTool && <span className="mt-1 inline-block h-3 w-px shrink-0 animate-pulse bg-current"/>}
      </div>
    </MessageContent>
  </Message>;
  return <Message align={isUser ? 'end' : 'start'}>
    <MessageContent>
      {!isUser && !compact && <MessageHeader>{isTool ? <TerminalSquare className="mr-1 size-3.5"/> : <Bot className="mr-1 size-3.5"/>}{isTool ? 'Agent 工具' : 'Alvax Agent'}</MessageHeader>}
      <Bubble variant={isUser ? 'default' : message.state === 'error' ? 'destructive' : 'ghost'} align={isUser ? 'end' : 'start'}>
        <BubbleContent className={cn(isTool && 'truncate font-mono text-xs')}>{message.content ? (isUser || isTool ? message.content : <MarkdownContent>{message.content}</MarkdownContent>) : <span className="flex items-center gap-2"><Spinner/>正在思考…</span>}{message.state === 'streaming' && message.content && !isTool && <span className="ml-0.5 inline-block h-4 w-px animate-pulse bg-current align-middle"/>}</BubbleContent>
      </Bubble>
    </MessageContent>
  </Message>;
}

function RequirementCard({ content }: { content: string }) {
  const fields = content.split('\n').slice(1).map((line) => {
    const separator = line.indexOf('：');
    return separator < 0 ? ['', line] : [line.slice(0, separator), line.slice(separator + 1)];
  });
  return <Card size="sm" className="max-w-2xl bg-muted/30">
    <CardHeader>
      <CardTitle className="flex items-center gap-2"><Sparkles/>网站升级任务已提交</CardTitle>
      <CardDescription>Alvax Agent 将依次完成专业诊断、竞品调研、升级方案与改版交付。</CardDescription>
    </CardHeader>
    <CardContent>
      <dl className="grid gap-x-4 gap-y-2 sm:grid-cols-[5rem_1fr]">
        {fields.map(([label, value]) => <div key={`${label}-${value}`} className="grid min-w-0 grid-cols-[5rem_1fr] gap-2 sm:col-span-2">
          <dt className="text-xs text-muted-foreground">{label}</dt>
          <dd className="min-w-0 text-xs leading-5 wrap-break-word">{value}</dd>
        </div>)}
      </dl>
    </CardContent>
  </Card>;
}

function KeyInfoCard({ content }: { content: string }) {
  const data = parseCardPayload<{ type?: string; title?: string; content?: string }>(content);
  const labels: Record<string, string> = {
    analysis: 'AI 专业诊断', competitor_research: '竞品网站调研', suggestion: '网站升级方案', final_delivery: '最终交付报告',
  };
  const label = labels[data.type ?? ''] ?? '关键信息';
  return <Card size="sm" className="shrink-0 bg-muted/30">
    <CardHeader>
      <CardTitle className="flex items-center gap-2"><Sparkles/>{data.title || label}</CardTitle>
    </CardHeader>
    <CardContent><MarkdownContent className="text-muted-foreground">{data.content || 'AI 正在整理关键信息…'}</MarkdownContent></CardContent>
  </Card>;
}

function ConfirmationCard({ content, pending, busy, onRespond }: { content: string; pending: boolean; busy: boolean; onRespond(toolCallId: string, approved: boolean, suggestion: string): void }) {
  const data = parseCardPayload<{ toolCallId?: string; title?: string; description?: string }>(content);
  const [suggestion, setSuggestion] = useState('');
  return <Card size="sm" className="shrink-0">
    <CardHeader>
      <CardTitle>{data.title || '确认工作方向'}</CardTitle>
      <CardDescription className="whitespace-pre-wrap leading-5">{data.description || 'AI 正在等待你确认下一步工作。'}</CardDescription>
      <CardAction><Badge variant="outline">{pending ? '等待确认' : '已处理'}</Badge></CardAction>
    </CardHeader>
    {pending && <>
      <CardContent>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor={`confirmation-${data.toolCallId}`}>补充建议</FieldLabel>
            <Textarea id={`confirmation-${data.toolCallId}`} value={suggestion} onChange={(event) => setSuggestion(event.target.value)} placeholder="可选：告诉 AI 需要调整或特别注意的内容…" disabled={busy} />
            <FieldDescription>确认后，AI 会结合你的建议，在原网站基础上开始专业升级。</FieldDescription>
          </Field>
        </FieldGroup>
      </CardContent>
      <CardFooter className="justify-end gap-2">
        <Button variant="outline" disabled={busy || !data.toolCallId} onClick={() => data.toolCallId && onRespond(data.toolCallId, false, suggestion)}>取消任务</Button>
        <Button disabled={busy || !data.toolCallId} onClick={() => data.toolCallId && onRespond(data.toolCallId, true, suggestion)}>{busy ? <Spinner/> : <Check/>}确认升级方案</Button>
      </CardFooter>
    </>}
  </Card>;
}

function parseCardPayload<T extends object>(value: string): T {
  try { return JSON.parse(value) as T; } catch { return {} as T; }
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
        {snapshot?.preview.url ? <><iframe key={`${snapshot.preview.url}-${snapshot.project.updatedAt}`} title="网站预览" src={snapshot.preview.url} sandbox="allow-scripts allow-forms allow-same-origin" className="size-full border-0 bg-background" />{busy && <div className="pointer-events-none absolute inset-0 grid place-items-center bg-background/60 backdrop-blur-sm"><Spinner/></div>}</> : <div className="grid h-full place-items-center p-8 text-center"><div><div className="mx-auto grid size-12 place-items-center rounded-xl bg-muted text-muted-foreground"><Globe2/></div><h3 className="mt-4 text-sm font-medium">{snapshot?.preview.status === 'failed' ? '预览启动失败' : '网站尚未启动'}</h3><p className={cn('mt-2 max-w-72 whitespace-pre-wrap text-xs leading-5', snapshot?.preview.status === 'failed' ? 'text-destructive' : 'text-muted-foreground')}>{snapshot?.preview.status === 'failed' ? snapshot.preview.error : '运行验收后会自动构建并在本地启动可访问的预览服务。'}</p><Button className="mt-5" size="sm" disabled={!snapshot || busy} onClick={snapshot?.checks.length ? onPreview : onRun}>{busy ? <Spinner/> : <Play/>}{snapshot?.preview.status === 'failed' ? '重新启动' : snapshot?.checks.length ? '启动预览' : '验收并启动'}</Button></div></div>}
      </div>
  </aside>;
}

function EmptyWorkspace({ onCreate }: { onCreate(): void }) { return <div className="grid h-full place-items-center"><div className="max-w-md text-center"><img src={alvaxStudioIcon} alt="" className="mx-auto size-16 rounded-2xl"/><h2 className="mt-5 text-xl font-semibold tracking-tight">让现有网站完成一次专业升级</h2><p className="mt-2 text-sm leading-6 text-muted-foreground">提交你的网站，Alvax Agent 会完成深度诊断、竞品对比与升级方案，并在你确认后交付可直接预览的改版网站。</p><Button className="mt-6" onClick={onCreate}><Plus/>升级我的网站</Button></div></div>; }

function NewProjectDialog({ open, onOpenChange, onCreate, busy }: { open: boolean; onOpenChange(value: boolean): void; onCreate(brief: CreateWebsiteProjectInput): void; busy: boolean }) {
  const [referenceUrl, setReferenceUrl] = useState('');
  const [referenceRequest, setReferenceRequest] = useState('');
  const valid = /^https?:\/\/\S+$/i.test(referenceUrl.trim()) && Boolean(referenceRequest.trim());
  const submit = () => onCreate({ mode: 'reference', referenceUrl: referenceUrl.trim(), referenceRequest: referenceRequest.trim() });
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="sm:max-w-xl"><DialogHeader><DialogTitle>升级你的现有网站</DialogTitle><DialogDescription>交给 Alvax Agent：它会像一支专业的网站策略与设计团队，诊断问题、研究竞品、制定方案，并在你确认后完成网站升级。</DialogDescription></DialogHeader><FieldGroup><Field><FieldLabel htmlFor="website-url">现有网站 URL</FieldLabel><Input id="website-url" type="url" value={referenceUrl} onChange={(event) => setReferenceUrl(event.target.value)} placeholder="https://www.yourwebsite.com"/><FieldDescription>AI 将深入理解你当前的网站、业务定位、品牌内容和转化路径。</FieldDescription></Field><Field><FieldLabel htmlFor="upgrade-request">这次最希望改善什么？</FieldLabel><Textarea id="upgrade-request" value={referenceRequest} onChange={(event) => setReferenceRequest(event.target.value)} placeholder="例如：品牌看起来不够专业，首页信息层级混乱，希望提升视觉质感、产品表达和咨询转化率。" rows={5}/><FieldDescription>可以描述当前问题、业务目标、希望保留的内容，或对新版本的期待。</FieldDescription></Field></FieldGroup><DialogFooter><Button variant="outline" onClick={() => onOpenChange(false)}>暂不升级</Button><Button disabled={!valid || busy} onClick={submit}>{busy ? <Spinner/> : <Sparkles/>}开始专业诊断</Button></DialogFooter></DialogContent></Dialog>;
}
