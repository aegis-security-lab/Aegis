import type { AuditLevel } from '../types';

export const AUDIT_LEVELS: AuditLevel[] = ['S', 'A', 'B', 'C', 'D', 'E', 'I'];

export const AUDIT_LEVEL_META: Record<AuditLevel, { label: string; description: string; severity: string }> = {
  S: { label: 'S级', description: '无条件高权限 RCE', severity: 'critical' },
  A: { label: 'A级', description: '无条件低权限 RCE', severity: 'critical' },
  B: { label: 'B级', description: '需要条件才能实现 RCE', severity: 'high' },
  C: { label: 'C级', description: '可获取数据或形成进一步利用能力', severity: 'high' },
  D: { label: 'D级', description: '可造成 DoS 或其他系统可用性影响', severity: 'medium' },
  E: { label: 'E级', description: '有限信息泄露或低影响安全缺陷', severity: 'low' },
  I: { label: 'I级', description: '信息项、待确认或仅安全加固建议', severity: 'info' },
};

export function auditLevelFromSeverity(severity: string): AuditLevel {
  switch (severity.toLowerCase()) {
    case 'critical': return 'A';
    case 'high': return 'B';
    case 'medium': return 'C';
    case 'low': return 'E';
    default: return 'I';
  }
}
