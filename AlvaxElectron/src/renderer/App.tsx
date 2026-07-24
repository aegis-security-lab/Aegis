import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  Activity,
  ArrowRight,
  Blocks,
  Bot,
  Box,
  Check,
  ChevronRight,
  CircleDot,
  Code2,
  Command,
  Cpu,
  FlaskConical,
  GitBranch,
  Layers3,
  Moon,
  Network,
  PanelLeft,
  Play,
  Plus,
  Radio,
  Settings,
  ShieldCheck,
  Sparkles,
  Sun,
  Trash2,
  X,
  Zap,
  type LucideIcon,
} from 'lucide-react';
import type { ApiResult } from '../shared/contracts/api';
import type {
  AgentDefinition,
  AppSettings,
  CoordinationMode,
  RunEvent,
  RunRecord,
  SystemInfo,
} from '../shared/contracts/domain';

type View = 'workspace' | 'agents' | 'runs' | 'architecture' | 'settings';

interface AgentDraft {
  name: string;
  role: string;
  description: string;
  capabilities: string;
  instructions: string;
}

const emptyAgentDraft: AgentDraft = {
  name: '',
  role: '',
  description: '',
  capabilities: '',
  instructions: '',
};

const navigation: { id: View; label: string; icon: LucideIcon }[] = [
  { id: 'workspace', label: '协同工作台', icon: Sparkles },
  { id: 'agents', label: 'Agent 目录', icon: Bot },
  { id: 'runs', label: '运行记录', icon: Activity },
  { id: 'architecture', label: '架构与接口', icon: Blocks },
  { id: 'settings', label: '项目设置', icon: Settings },
];

const modeLabels: Record<CoordinationMode, string> = {
  supervisor: '主管调度',
  'round-robin': '轮次协作',
  pipeline: '流水线',
};

const statusLabels: Record<RunRecord['status'], string> = {
  queued: '等待中',
  running: '运行中',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
};

function unwrap<T>(result: ApiResult<T>): T {
  if (!result.ok) throw new Error(result.error.message);
  return result.data;
}

export default function App() {
  const [view, setView] = useState<View>('workspace');
  const [agents, setAgents] = useState<AgentDefinition[]>([]);
  const [runs, setRuns] = useState<RunRecord[]>([]);
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [events, setEvents] = useState<RunEvent[]>([]);
  const [selectedAgents, setSelectedAgents] = useState<string[]>([]);
  const [objective, setObjective] = useState('调研多 Agent 协同产品的核心使用场景，并给出首个可验证的 MVP 假设。');
  const [mode, setMode] = useState<CoordinationMode>('supervisor');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showAgentForm, setShowAgentForm] = useState(false);
  const [agentDraft, setAgentDraft] = useState<AgentDraft>(emptyAgentDraft);

  const refreshRuns = useCallback(async () => {
    setRuns(unwrap(await window.alvax.runs.list()));
  }, []);

  const loadWorkspace = useCallback(async () => {
    try {
      const [nextAgents, nextRuns, nextSettings, nextSystemInfo] = await Promise.all([
        window.alvax.agents.list(),
        window.alvax.runs.list(),
        window.alvax.settings.get(),
        window.alvax.system.getInfo(),
      ]);
      const agentData = unwrap(nextAgents);
      setAgents(agentData);
      setSelectedAgents(agentData.filter((agent) => agent.status === 'active').map((agent) => agent.id));
      setRuns(unwrap(nextRuns));
      setSettings(unwrap(nextSettings));
      setSystemInfo(unwrap(nextSystemInfo));
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : '工作区加载失败。');
    }
  }, []);

  useEffect(() => {
    // Initial data loading is an intentional external-system synchronization.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadWorkspace();
    return window.alvax.runs.onEvent((event) => {
      setEvents((current) => [event, ...current].slice(0, 12));
      void refreshRuns();
    });
  }, [loadWorkspace, refreshRuns]);

  useEffect(() => {
    if (settings) document.documentElement.dataset.theme = settings.theme;
  }, [settings]);

  const activeRun = useMemo(
    () => runs.find((run) => run.status === 'running'),
    [runs],
  );

  const toggleAgent = (id: string) => {
    setSelectedAgents((current) =>
      current.includes(id) ? current.filter((agentId) => agentId !== id) : [...current, id],
    );
  };

  const startRun = async () => {
    setError(null);
    if (objective.trim().length < 3) {
      setError('请先写下这次协同要解决的问题。');
      return;
    }
    if (!selectedAgents.length) {
      setError('请至少选择一个 Agent。');
      return;
    }
    setBusy(true);
    try {
      const started = unwrap(
        await window.alvax.runs.start({
          objective: objective.trim(),
          agentIds: selectedAgents,
          mode,
        }),
      );
      setRuns((current) => [started, ...current]);
    } catch (runError) {
      setError(runError instanceof Error ? runError.message : '任务启动失败。');
    } finally {
      setBusy(false);
    }
  };

  const cancelRun = async () => {
    if (!activeRun) return;
    setError(null);
    try {
      const cancelled = unwrap(await window.alvax.runs.cancel(activeRun.id));
      setRuns((current) =>
        current.map((run) => (run.id === cancelled.id ? cancelled : run)),
      );
    } catch (cancelError) {
      setError(cancelError instanceof Error ? cancelError.message : '取消任务失败。');
    }
  };

  const saveAgent = async (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const saved = unwrap(
        await window.alvax.agents.save({
          name: agentDraft.name,
          role: agentDraft.role,
          description: agentDraft.description,
          capabilities: agentDraft.capabilities
            .split(/[,，]/)
            .map((item) => item.trim())
            .filter(Boolean),
          instructions: agentDraft.instructions,
          runtime: { kind: 'mock', model: 'research-simulator' },
          status: 'active',
        }),
      );
      setAgents((current) => [...current, saved]);
      setSelectedAgents((current) => [...current, saved.id]);
      setAgentDraft(emptyAgentDraft);
      setShowAgentForm(false);
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : '保存 Agent 失败。');
    } finally {
      setBusy(false);
    }
  };

  const removeAgent = async (agent: AgentDefinition) => {
    if (!window.confirm(`确定删除「${agent.name}」吗？`)) return;
    try {
      unwrap(await window.alvax.agents.remove(agent.id));
      setAgents((current) => current.filter((item) => item.id !== agent.id));
      setSelectedAgents((current) => current.filter((id) => id !== agent.id));
    } catch (removeError) {
      setError(removeError instanceof Error ? removeError.message : '删除 Agent 失败。');
    }
  };

  const updateTheme = async (theme: AppSettings['theme']) => {
    try {
      setSettings(unwrap(await window.alvax.settings.update({ theme })));
    } catch (settingsError) {
      setError(settingsError instanceof Error ? settingsError.message : '设置保存失败。');
    }
  };

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="window-drag-region" />
        <div className="brand">
          <div className="brand-mark"><Command size={18} /></div>
          <div>
            <strong>Alvax</strong><span> AI</span>
            <small>缘分 · Agent OS</small>
          </div>
        </div>

        <nav className="navigation" aria-label="主导航">
          <span className="nav-caption">Workspace</span>
          {navigation.map((item) => {
            const Icon = item.icon;
            return (
              <button
                key={item.id}
                className={view === item.id ? 'nav-item active' : 'nav-item'}
                onClick={() => setView(item.id)}
              >
                <Icon size={17} />
                <span>{item.label}</span>
                {item.id === 'runs' && activeRun ? <i className="live-dot" /> : null}
              </button>
            );
          })}
        </nav>

        <div className="sidebar-spacer" />
        <div className="research-badge">
          <div><FlaskConical size={15} /> Research build</div>
          <p>接口优先 · 模拟运行时</p>
        </div>
        <div className="sidebar-footer">
          <span className="status-dot" />
          <span>Local workspace</span>
          <small>v{systemInfo?.appVersion ?? '0.1.0'}</small>
        </div>
      </aside>

      <main className="main-area">
        <header className="topbar window-drag-region">
          <div>
            <span className="eyebrow">ALVAX / {navigation.find((item) => item.id === view)?.label}</span>
          </div>
          <div className="topbar-actions window-no-drag">
            <span className="environment"><CircleDot size={12} /> LOCAL</span>
            <button className="icon-button" aria-label="切换侧栏"><PanelLeft size={17} /></button>
          </div>
        </header>

        <div className="content">
          {error ? (
            <div className="error-banner" role="alert">
              <span>{error}</span><button onClick={() => setError(null)}><X size={15} /></button>
            </div>
          ) : null}

          {view === 'workspace' && (
            <WorkspaceView
              agents={agents}
              runs={runs}
              events={events}
              selectedAgents={selectedAgents}
              objective={objective}
              mode={mode}
              busy={busy}
              activeRun={activeRun}
              onToggleAgent={toggleAgent}
              onObjectiveChange={setObjective}
              onModeChange={setMode}
              onStart={() => void startRun()}
              onCancel={() => void cancelRun()}
              onNavigate={setView}
            />
          )}
          {view === 'agents' && (
            <AgentsView
              agents={agents}
              onAdd={() => setShowAgentForm(true)}
              onRemove={(agent) => void removeAgent(agent)}
            />
          )}
          {view === 'runs' && <RunsView runs={runs} agents={agents} />}
          {view === 'architecture' && <ArchitectureView />}
          {view === 'settings' && settings && (
            <SettingsView
              settings={settings}
              systemInfo={systemInfo}
              onTheme={(theme) => void updateTheme(theme)}
            />
          )}
        </div>
      </main>

      {showAgentForm ? (
        <AgentForm
          value={agentDraft}
          busy={busy}
          onChange={setAgentDraft}
          onClose={() => setShowAgentForm(false)}
          onSubmit={(event) => void saveAgent(event)}
        />
      ) : null}
    </div>
  );
}

interface WorkspaceViewProps {
  agents: AgentDefinition[];
  runs: RunRecord[];
  events: RunEvent[];
  selectedAgents: string[];
  objective: string;
  mode: CoordinationMode;
  busy: boolean;
  activeRun: RunRecord | undefined;
  onToggleAgent(id: string): void;
  onObjectiveChange(value: string): void;
  onModeChange(value: CoordinationMode): void;
  onStart(): void;
  onCancel(): void;
  onNavigate(value: View): void;
}

function WorkspaceView(props: WorkspaceViewProps) {
  const completed = props.runs.filter((run) => run.status === 'completed').length;
  return (
    <>
      <section className="hero-row">
        <div>
          <div className="section-kicker"><Radio size={13} /> MULTI-AGENT LAB</div>
          <h1>让多个智能体，<br /><em>围绕同一个结果工作。</em></h1>
          <p>这是 Alvax AI 的研究基座。先验证协作模型，再接入真实模型、工具和业务流程。</p>
        </div>
        <div className="agent-orbit" aria-hidden="true">
          <div className="orbit-ring ring-one" />
          <div className="orbit-ring ring-two" />
          <div className="orbit-core"><Sparkles size={25} /></div>
          <span className="orbit-node node-one"><Bot size={14} /></span>
          <span className="orbit-node node-two"><GitBranch size={14} /></span>
          <span className="orbit-node node-three"><ShieldCheck size={14} /></span>
        </div>
      </section>

      <section className="metric-grid">
        <Metric icon={Bot} label="可用 Agent" value={String(props.agents.filter((a) => a.status === 'active').length)} note="本地注册表" />
        <Metric icon={Activity} label="协同运行" value={String(props.runs.length)} note={`${completed} 次已完成`} />
        <Metric icon={Zap} label="运行时" value="Mock" note="Adapter 可替换" accent />
      </section>

      <section className="workspace-grid">
        <div className="panel composer-panel">
          <div className="panel-heading">
            <div><span>NEW ORCHESTRATION</span><h2>发起一次协同</h2></div>
            <span className="step-tag">01 / DEFINE</span>
          </div>
          <label className="field-label" htmlFor="objective">目标 / Objective</label>
          <textarea
            id="objective"
            className="objective-input"
            value={props.objective}
            onChange={(event) => props.onObjectiveChange(event.target.value)}
            maxLength={4000}
          />

          <div className="composer-row">
            <div className="field-group agent-picker">
              <span className="field-label">参与者 · {props.selectedAgents.length}</span>
              <div className="agent-pills">
                {props.agents.filter((agent) => agent.status === 'active').map((agent, index) => {
                  const selected = props.selectedAgents.includes(agent.id);
                  return (
                    <button
                      key={agent.id}
                      className={selected ? 'agent-pill selected' : 'agent-pill'}
                      onClick={() => props.onToggleAgent(agent.id)}
                    >
                      <i>{String(index + 1).padStart(2, '0')}</i>
                      <span>{agent.name}</span>
                      {selected ? <Check size={13} /> : <Plus size={13} />}
                    </button>
                  );
                })}
              </div>
            </div>
            <div className="field-group mode-picker">
              <label className="field-label" htmlFor="mode">协作模式</label>
              <select
                id="mode"
                value={props.mode}
                onChange={(event) => props.onModeChange(event.target.value as CoordinationMode)}
              >
                {Object.entries(modeLabels).map(([value, label]) => (
                  <option key={value} value={value}>{label}</option>
                ))}
              </select>
            </div>
          </div>

          <div className="composer-footer">
            <span><ShieldCheck size={14} /> 仅本地模拟，不会发送外部请求</span>
            {props.activeRun ? (
              <button className="secondary-button danger" onClick={props.onCancel}><X size={15} />取消运行</button>
            ) : (
              <button className="primary-button" onClick={props.onStart} disabled={props.busy}>
                <Play size={15} fill="currentColor" />{props.busy ? '正在启动…' : '启动协同'}<ArrowRight size={15} />
              </button>
            )}
          </div>
        </div>

        <div className="panel activity-panel">
          <div className="panel-heading compact">
            <div><span>LIVE STREAM</span><h2>运行事件</h2></div>
            {props.activeRun ? <span className="live-label"><i /> LIVE</span> : null}
          </div>
          <div className="event-stream">
            {props.events.length ? props.events.map((event) => (
              <div className="event-item" key={event.id}>
                <div className={`event-glyph ${event.type.replace('.', '-')}`}><Activity size={13} /></div>
                <div><strong>{event.message}</strong><span>{formatTime(event.at)} · {event.type}</span></div>
              </div>
            )) : (
              <div className="empty-stream">
                <Network size={26} />
                <strong>等待协同信号</strong>
                <span>启动任务后，编排事件会实时出现在这里。</span>
              </div>
            )}
          </div>
          <button className="panel-link" onClick={() => props.onNavigate('runs')}>查看全部运行 <ChevronRight size={14} /></button>
        </div>
      </section>
    </>
  );
}

function Metric({ icon: Icon, label, value, note, accent = false }: { icon: LucideIcon; label: string; value: string; note: string; accent?: boolean }) {
  return (
    <div className={accent ? 'metric-card accent' : 'metric-card'}>
      <div className="metric-icon"><Icon size={17} /></div>
      <div><span>{label}</span><strong>{value}</strong></div>
      <small>{note}</small>
    </div>
  );
}

function AgentsView({ agents, onAdd, onRemove }: { agents: AgentDefinition[]; onAdd(): void; onRemove(agent: AgentDefinition): void }) {
  return (
    <section className="page-section">
      <PageHeading kicker="AGENT REGISTRY" title="Agent 目录" description="统一管理角色、能力与运行时引用；密钥不会进入渲染进程。" action={<button className="primary-button" onClick={onAdd}><Plus size={15} /> 新建 Agent</button>} />
      <div className="agent-card-grid">
        {agents.map((agent, index) => (
          <article className="agent-card" key={agent.id}>
            <div className="agent-card-top">
              <span className="agent-index">A-{String(index + 1).padStart(2, '0')}</span>
              <span className={`agent-status ${agent.status}`}><i />{agent.status}</span>
            </div>
            <div className="agent-avatar"><Bot size={22} /></div>
            <h3>{agent.name}</h3>
            <span className="agent-role">{agent.role}</span>
            <p>{agent.description}</p>
            <div className="capability-list">
              {agent.capabilities.map((capability) => <span key={capability}>{capability}</span>)}
            </div>
            <div className="agent-card-footer">
              <span><Cpu size={14} /> {agent.runtime.kind}</span>
              <button className="text-icon-button danger-text" onClick={() => onRemove(agent)} aria-label={`删除 ${agent.name}`}><Trash2 size={15} /></button>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}

function RunsView({ runs, agents }: { runs: RunRecord[]; agents: AgentDefinition[] }) {
  return (
    <section className="page-section">
      <PageHeading kicker="EXECUTION HISTORY" title="运行记录" description="当前保留最近 100 次本地运行，后续可替换为 SQLite 或远端事件存储。" />
      <div className="run-list">
        {runs.length ? runs.map((run) => (
          <article className="run-row" key={run.id}>
            <div className={`run-state ${run.status}`}><Activity size={16} /></div>
            <div className="run-main">
              <div><strong>{run.objective}</strong><span className={`status-badge ${run.status}`}>{statusLabels[run.status]}</span></div>
              <p>{run.summary ?? `${run.agentIds.length} 个 Agent · ${modeLabels[run.mode]}`}</p>
              <div className="run-agents">
                {run.agentIds.map((id) => <span key={id}>{agents.find((agent) => agent.id === id)?.name ?? id}</span>)}
              </div>
            </div>
            <div className="run-meta"><span>{formatDate(run.createdAt)}</span><code>{run.id.slice(0, 8)}</code></div>
          </article>
        )) : (
          <div className="large-empty"><Activity size={32} /><h3>还没有运行记录</h3><p>从协同工作台发起第一次研究任务。</p></div>
        )}
      </div>
    </section>
  );
}

function ArchitectureView() {
  const layers = [
    { icon: Box, name: 'Renderer', tech: 'React', text: '纯 UI，不访问 Node 或凭据' },
    { icon: ShieldCheck, name: 'Preload Bridge', tech: 'Typed IPC', text: '逐方法白名单与事件订阅' },
    { icon: Cpu, name: 'Application Core', tech: 'Ports', text: '编排、验证、生命周期管理' },
    { icon: Layers3, name: 'Adapters', tech: 'Replaceable', text: '模型、存储、工具与远端服务' },
  ];
  return (
    <section className="page-section">
      <PageHeading kicker="SYSTEM BLUEPRINT" title="架构与接口" description="依赖只向内流动。产品功能变化时，Electron 安全边界与业务契约保持稳定。" />
      <div className="architecture-flow">
        {layers.map((layer, index) => {
          const Icon = layer.icon;
          return (
            <div className="architecture-step" key={layer.name}>
              <span className="layer-number">0{index + 1}</span>
              <div className="layer-icon"><Icon size={20} /></div>
              <div><span>{layer.tech}</span><h3>{layer.name}</h3><p>{layer.text}</p></div>
              {index < layers.length - 1 ? <ArrowRight className="layer-arrow" size={18} /> : null}
            </div>
          );
        })}
      </div>
      <div className="contract-grid">
        <div className="contract-panel">
          <div className="contract-title"><Code2 size={17} /><span>Desktop API / v1</span></div>
          <code>system.getInfo()</code>
          <code>agents.list() · save() · remove()</code>
          <code>runs.list() · start() · cancel()</code>
          <code>runs.onEvent(listener)</code>
          <code>settings.get() · update()</code>
        </div>
        <div className="contract-panel principles">
          <div className="contract-title"><ShieldCheck size={17} /><span>默认约束</span></div>
          <p><Check size={14} /> contextIsolation + sandbox</p>
          <p><Check size={14} /> Zod 输入验证与统一错误</p>
          <p><Check size={14} /> Renderer 永不持有供应商密钥</p>
          <p><Check size={14} /> CSP、导航与权限全部收紧</p>
          <p><Check size={14} /> 打包时启用 Electron Fuses</p>
        </div>
      </div>
    </section>
  );
}

function SettingsView({ settings, systemInfo, onTheme }: { settings: AppSettings; systemInfo: SystemInfo | null; onTheme(theme: AppSettings['theme']): void }) {
  return (
    <section className="page-section settings-page">
      <PageHeading kicker="WORKSPACE SETTINGS" title="项目设置" description="调研阶段默认本地优先、关闭遥测。" />
      <div className="settings-panel">
        <div className="setting-row">
          <div><strong>外观主题</strong><span>跟随系统，或固定使用浅色 / 深色。</span></div>
          <div className="segmented-control">
            {([
              ['system', Cpu, '系统'],
              ['light', Sun, '浅色'],
              ['dark', Moon, '深色'],
            ] as const).map(([value, Icon, label]) => (
              <button key={value} className={settings.theme === value ? 'active' : ''} onClick={() => onTheme(value)}><Icon size={14} />{label}</button>
            ))}
          </div>
        </div>
        <div className="setting-row">
          <div><strong>匿名遥测</strong><span>脚手架不包含任何遥测 SDK，默认关闭。</span></div>
          <span className="off-badge">OFF</span>
        </div>
        <div className="setting-row">
          <div><strong>运行环境</strong><span>用于排查本地构建与打包差异。</span></div>
          <code>{systemInfo ? `${systemInfo.platform} / ${systemInfo.arch} / ${systemInfo.isPackaged ? 'packaged' : 'development'}` : 'loading'}</code>
        </div>
      </div>
    </section>
  );
}

function PageHeading({ kicker, title, description, action }: { kicker: string; title: string; description: string; action?: React.ReactNode }) {
  return (
    <div className="page-heading">
      <div><span>{kicker}</span><h1>{title}</h1><p>{description}</p></div>
      {action}
    </div>
  );
}

function AgentForm({ value, busy, onChange, onClose, onSubmit }: { value: AgentDraft; busy: boolean; onChange(value: AgentDraft): void; onClose(): void; onSubmit(event: FormEvent): void }) {
  const setField = (field: keyof AgentDraft, fieldValue: string) => onChange({ ...value, [field]: fieldValue });
  return (
    <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <form className="modal" onSubmit={onSubmit}>
        <div className="modal-heading"><div><span>AGENT / NEW</span><h2>新建 Agent</h2></div><button type="button" onClick={onClose}><X size={18} /></button></div>
        <div className="form-grid">
          <label><span>显示名称</span><input required maxLength={80} value={value.name} onChange={(event) => setField('name', event.target.value)} placeholder="例如：用户研究员" /></label>
          <label><span>角色标识</span><input required maxLength={80} value={value.role} onChange={(event) => setField('role', event.target.value)} placeholder="例如：Researcher" /></label>
          <label className="full"><span>角色说明</span><textarea maxLength={500} value={value.description} onChange={(event) => setField('description', event.target.value)} placeholder="这个 Agent 负责什么？" /></label>
          <label className="full"><span>能力标签</span><input value={value.capabilities} onChange={(event) => setField('capabilities', event.target.value)} placeholder="访谈分析，事实核查，机会识别" /></label>
          <label className="full"><span>系统指令</span><textarea maxLength={10000} value={value.instructions} onChange={(event) => setField('instructions', event.target.value)} placeholder="定义它的目标、原则和输出格式。" /></label>
        </div>
        <div className="modal-note"><FlaskConical size={14} /> 调研期新 Agent 默认使用 research-simulator。</div>
        <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>取消</button><button className="primary-button" disabled={busy}><Plus size={15} /> 创建 Agent</button></div>
      </form>
    </div>
  );
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value));
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value));
}
