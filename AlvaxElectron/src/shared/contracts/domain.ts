import { z } from 'zod';

export const IdSchema = z.string().min(1).max(128);
export const TimestampSchema = z.iso.datetime();

export const AgentRuntimeSchema = z.discriminatedUnion('kind', [
  z.object({
    kind: z.literal('mock'),
    model: z.string().min(1).max(120).default('research-simulator'),
  }),
  z.object({
    kind: z.literal('openai-compatible'),
    model: z.string().min(1).max(120),
    baseUrl: z.url().startsWith('https://'),
    credentialKey: z.string().min(1).max(120),
  }),
  z.object({
    kind: z.literal('remote'),
    endpoint: z.url().startsWith('https://'),
    credentialKey: z.string().min(1).max(120).optional(),
  }),
]);

export const AgentDefinitionSchema = z.object({
  id: IdSchema,
  name: z.string().trim().min(1).max(80),
  role: z.string().trim().min(1).max(80),
  description: z.string().trim().max(500),
  capabilities: z.array(z.string().trim().min(1).max(60)).max(20),
  instructions: z.string().trim().max(10_000),
  runtime: AgentRuntimeSchema,
  status: z.enum(['draft', 'active', 'disabled']),
  createdAt: TimestampSchema,
  updatedAt: TimestampSchema,
});

export const SaveAgentInputSchema = AgentDefinitionSchema.omit({
  id: true,
  createdAt: true,
  updatedAt: true,
}).extend({
  id: IdSchema.optional(),
});

export const CoordinationModeSchema = z.enum([
  'supervisor',
  'round-robin',
  'pipeline',
]);

export const RunRecordSchema = z.object({
  id: IdSchema,
  objective: z.string().trim().min(3).max(4_000),
  agentIds: z.array(IdSchema).min(1).max(20),
  mode: CoordinationModeSchema,
  status: z.enum(['queued', 'running', 'completed', 'failed', 'cancelled']),
  summary: z.string().max(4_000).optional(),
  createdAt: TimestampSchema,
  updatedAt: TimestampSchema,
});

export const StartRunInputSchema = RunRecordSchema.pick({
  objective: true,
  agentIds: true,
  mode: true,
});

export const RunEventSchema = z.object({
  id: IdSchema,
  runId: IdSchema,
  type: z.enum([
    'run.started',
    'agent.started',
    'agent.completed',
    'run.completed',
    'run.failed',
    'run.cancelled',
  ]),
  at: TimestampSchema,
  message: z.string().max(1_000),
  agentId: IdSchema.optional(),
});

export const AppSettingsSchema = z.object({
  theme: z.enum(['system', 'light', 'dark']),
  language: z.enum(['zh-CN', 'en-US']),
  telemetryEnabled: z.boolean(),
});

export const UpdateSettingsInputSchema = AppSettingsSchema.partial();

export const SystemInfoSchema = z.object({
  appVersion: z.string(),
  platform: z.enum(['aix', 'android', 'darwin', 'freebsd', 'haiku', 'linux', 'openbsd', 'sunos', 'win32', 'cygwin', 'netbsd']),
  arch: z.string(),
  isPackaged: z.boolean(),
  logPath: z.string(),
});

export type AgentDefinition = z.infer<typeof AgentDefinitionSchema>;
export type AgentRuntimeConfig = z.infer<typeof AgentRuntimeSchema>;
export type SaveAgentInput = z.infer<typeof SaveAgentInputSchema>;
export type CoordinationMode = z.infer<typeof CoordinationModeSchema>;
export type RunRecord = z.infer<typeof RunRecordSchema>;
export type StartRunInput = z.infer<typeof StartRunInputSchema>;
export type RunEvent = z.infer<typeof RunEventSchema>;
export type AppSettings = z.infer<typeof AppSettingsSchema>;
export type UpdateSettingsInput = z.infer<typeof UpdateSettingsInputSchema>;
export type SystemInfo = z.infer<typeof SystemInfoSchema>;
