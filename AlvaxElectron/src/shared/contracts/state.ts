import { z } from 'zod';
import {
  AgentDefinitionSchema,
  AppSettingsSchema,
  RunRecordSchema,
} from './domain';

export const WorkspaceStateSchema = z.object({
  schemaVersion: z.literal(1),
  agents: z.array(AgentDefinitionSchema),
  runs: z.array(RunRecordSchema),
  settings: AppSettingsSchema,
});

export type WorkspaceState = z.infer<typeof WorkspaceStateSchema>;
