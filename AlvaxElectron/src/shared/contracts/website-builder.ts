import { z } from 'zod';

export const WEBSITE_BRIEF_MESSAGE_PREFIX = '[[ALVAX_WEBSITE_BRIEF]]';
export const ALVAX_KEY_INFO_PREFIX = '[[ALVAX_KEY_INFO]]';
export const ALVAX_CONFIRMATION_PREFIX = '[[ALVAX_CONFIRMATION]]';

export function isInitialWebsiteBriefMessage(message: string): boolean {
  return message.startsWith(WEBSITE_BRIEF_MESSAGE_PREFIX);
}

export const WebsitePurposeSchema = z.enum([
  'brand',
  'product',
  'conversion',
  'content',
]);
export type WebsitePurpose = z.infer<typeof WebsitePurposeSchema>;

export const WebsiteBriefSchema = z.object({
  mode: z.enum(['create', 'reference']).default('create'),
  name: z.string().trim().min(1).max(80),
  industry: z.string().trim().min(1).max(120),
  offering: z.string().trim().min(1).max(240),
  audience: z.string().trim().min(1).max(240),
  purposes: z.array(WebsitePurposeSchema).min(1),
  notes: z.string().trim().max(2_000).default(''),
  referenceUrl: z.string().trim().default(''),
  referenceRequest: z.string().trim().max(4_000).default(''),
});
export type WebsiteBrief = z.infer<typeof WebsiteBriefSchema>;

export const ProjectStatusSchema = z.enum([
  'ready',
  'generating',
  'checking',
  'previewing',
  'failed',
]);
export type ProjectStatus = z.infer<typeof ProjectStatusSchema>;

export const WebsiteProjectSchema = z.object({
  id: z.string().uuid(),
  brief: WebsiteBriefSchema,
  status: ProjectStatusSchema,
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
});
export type WebsiteProject = z.infer<typeof WebsiteProjectSchema>;

export const ChatMessageSchema = z.object({
  id: z.string().uuid(),
  projectId: z.string().uuid(),
  role: z.enum(['user', 'assistant', 'system', 'tool']),
  content: z.string(),
  state: z.enum(['streaming', 'complete', 'error']).default('complete'),
  createdAt: z.string().datetime(),
});
export type ChatMessage = z.infer<typeof ChatMessageSchema>;

export const AcceptanceCheckSchema = z.object({
  id: z.string().uuid(),
  projectId: z.string().uuid(),
  kind: z.enum(['install', 'typecheck', 'build', 'health']),
  label: z.string(),
  status: z.enum(['pending', 'running', 'passed', 'failed']),
  summary: z.string().default(''),
  durationMs: z.number().nonnegative().default(0),
});
export type AcceptanceCheck = z.infer<typeof AcceptanceCheckSchema>;

export const DeliveryArtifactSchema = z.object({
  path: z.string(),
  kind: z.enum(['page', 'component', 'config', 'asset']),
});
export type DeliveryArtifact = z.infer<typeof DeliveryArtifactSchema>;

export const PreviewStateSchema = z.object({
  status: z.enum(['stopped', 'starting', 'running', 'failed']),
  url: z.string().url().optional(),
  error: z.string().optional(),
});
export type PreviewState = z.infer<typeof PreviewStateSchema>;

export const WebsiteBuilderSnapshotSchema = z.object({
  project: WebsiteProjectSchema,
  messages: z.array(ChatMessageSchema),
  checks: z.array(AcceptanceCheckSchema),
  artifacts: z.array(DeliveryArtifactSchema),
  preview: PreviewStateSchema,
});
export type WebsiteBuilderSnapshot = z.infer<typeof WebsiteBuilderSnapshotSchema>;

export const WebsiteUrlSchema = z.url('请输入有效的网站 URL。').max(2_000)
  .refine((value) => {
    try {
      const parsed = new URL(value);
      return (parsed.protocol === 'http:' || parsed.protocol === 'https:')
        && (parsed.hostname === 'localhost' || parsed.hostname.includes('.'));
    } catch {
      return false;
    }
  }, '请输入以 http:// 或 https:// 开头的完整网站地址，例如 https://example.com。');

export const CreateWebsiteProjectInputSchema = z.object({
  mode: z.literal('reference'),
  referenceUrl: WebsiteUrlSchema,
  referenceRequest: z.string().trim().min(1, '请描述希望重点升级的内容。').max(4_000),
});
export type CreateWebsiteProjectInput = z.infer<typeof CreateWebsiteProjectInputSchema>;

export const SendWebsiteMessageInputSchema = z.object({
  projectId: z.string().uuid(),
  message: z.string().trim().min(1).max(20_000),
});
export type SendWebsiteMessageInput = z.infer<typeof SendWebsiteMessageInputSchema>;

export const ConfirmationResponseInputSchema = z.object({
  projectId: z.string().uuid(),
  toolCallId: z.string().regex(/^[A-Za-z0-9_-]{1,128}$/),
  approved: z.boolean(),
  suggestion: z.string().trim().max(4_000).default(''),
});
export type ConfirmationResponseInput = z.infer<typeof ConfirmationResponseInputSchema>;

export const RuntimeSettingsSchema = z.object({
  nodePath: z.string().trim().min(1),
  piPath: z.string().trim().min(1),
  provider: z.string().trim().max(80).default(''),
  model: z.string().trim().max(160).default(''),
});
export type RuntimeSettings = z.infer<typeof RuntimeSettingsSchema>;

export const RuntimeStatusSchema = z.object({
  settings: RuntimeSettingsSchema,
  ready: z.boolean(),
  message: z.string(),
});
export type RuntimeStatus = z.infer<typeof RuntimeStatusSchema>;

export const WebsiteBuilderEventSchema = z.object({
  projectId: z.string().uuid(),
  type: z.enum(['snapshot', 'agent', 'acceptance', 'preview', 'error']),
  sequence: z.number().int().nonnegative(),
});
export type WebsiteBuilderEvent = z.infer<typeof WebsiteBuilderEventSchema>;
