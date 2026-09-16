import { describe, expect, it } from 'vitest';
import { cloneDeveloperConfig, sanitizeDeveloperConfig } from '@/utils/developerConfig';

describe('developer config defaults', () => {
  it('preserves version-managed sentinels for missing server fields', () => {
    const config = sanitizeDeveloperConfig();

    expect(config.webSessionCodexClientName).toBe('codekanban-web-session');
    expect(config.webSessionCodexClientTitle).toBe('Code Kanban Web Session');
    expect(config.webSessionCodexClientVersion).toBe('0.0.0');
    expect(config.webSessionCodexDefaultModel).toBe('default');
    expect(config.webSessionCodexDefaultReasoningEffort).toBe('default');
    expect(config.webSessionCodexDefaultPermissionLevel).toBe('default');
    expect(config.webSessionCodexDefaultSyncMode).toBe('default');
    expect(config.webSessionCodexContextWindow).toBe(0);
    expect(config.webSessionClaudeDefaultModel).toBe('default');
    expect(config.webSessionClaudeDefaultReasoningEffort).toBe('default');
    expect(config.webSessionPiDefaultModel).toBe('default');
    expect(config.webSessionPiDefaultReasoningEffort).toBe('default');
    expect(config.webSessionDevinDefaultModel).toBe('default');
    expect(config.webSessionDevinDefaultReasoningEffort).toBe('default');
    expect(config.webSessionAutoRetryDefaults).toEqual({
      scope: 'network_only',
      preset: 'gentle_stop',
      maxAttempts: 0,
      dispatchPendingOnFailure: false,
    });
  });

  it('trims custom models and normalizes explicit settings', () => {
    const config = sanitizeDeveloperConfig({
      webSessionCodexClientName: '  custom-client  ',
      webSessionCodexClientTitle: '  Custom Title  ',
      webSessionCodexClientVersion: '  1.2.3  ',
      webSessionCodexDefaultModel: '  custom-codex-model  ',
      webSessionCodexDefaultReasoningEffort: 'model_default',
      webSessionCodexDefaultPermissionLevel: 'standard',
      webSessionCodexDefaultSyncMode: 'deep',
      webSessionCodexContextWindow: 768000,
      webSessionAutoRetryDefaults: {
        scope: 'all_failures',
        preset: 'sustain_60s',
        maxAttempts: 150,
        dispatchPendingOnFailure: true,
      },
    });

    expect(config.webSessionCodexClientName).toBe('custom-client');
    expect(config.webSessionCodexClientTitle).toBe('Custom Title');
    expect(config.webSessionCodexClientVersion).toBe('1.2.3');
    expect(config.webSessionCodexDefaultModel).toBe('custom-codex-model');
    expect(config.webSessionCodexDefaultReasoningEffort).toBe('model_default');
    expect(config.webSessionCodexDefaultPermissionLevel).toBe('standard');
    expect(config.webSessionCodexDefaultSyncMode).toBe('deep');
    expect(config.webSessionCodexContextWindow).toBe(768000);
    expect(config.webSessionAutoRetryDefaults).toEqual({
      scope: 'all_failures',
      preset: 'sustain_60s',
      maxAttempts: 100,
      dispatchPendingOnFailure: true,
    });
  });

  it('falls back from invalid values and returns independent clones', () => {
    const source = sanitizeDeveloperConfig({
      webSessionCodexDefaultReasoningEffort: 'invalid' as never,
      webSessionCodexDefaultPermissionLevel: 'invalid' as never,
      webSessionCodexDefaultSyncMode: 'invalid' as never,
      webSessionCodexContextWindow: 123,
    });
    const clone = cloneDeveloperConfig(source);

    expect(source.webSessionCodexDefaultReasoningEffort).toBe('default');
    expect(source.webSessionCodexDefaultPermissionLevel).toBe('default');
    expect(source.webSessionCodexDefaultSyncMode).toBe('default');
    expect(source.webSessionCodexContextWindow).toBe(0);
    clone.webSessionActiveCallTimeout.callKinds.mcp = false;
    clone.webSessionAutoRetryDefaults.scope = 'all_failures';
    expect(source.webSessionActiveCallTimeout.callKinds.mcp).toBe(true);
    expect(source.webSessionAutoRetryDefaults.scope).toBe('network_only');
  });

  it('normalizes per-agent default models and reasoning efforts', () => {
    const config = sanitizeDeveloperConfig({
      webSessionClaudeDefaultModel: '  sonnet  ',
      webSessionClaudeDefaultReasoningEffort: 'high',
      webSessionPiDefaultModel: ' anthropic/claude-sonnet-4 ',
      webSessionPiDefaultReasoningEffort: 'max',
      webSessionDevinDefaultModel: 'swe-2',
      webSessionDevinDefaultReasoningEffort: 'model_default',
    });

    expect(config.webSessionClaudeDefaultModel).toBe('sonnet');
    expect(config.webSessionClaudeDefaultReasoningEffort).toBe('high');
    expect(config.webSessionPiDefaultModel).toBe('anthropic/claude-sonnet-4');
    expect(config.webSessionPiDefaultReasoningEffort).toBe('max');
    expect(config.webSessionDevinDefaultModel).toBe('swe-2');
    expect(config.webSessionDevinDefaultReasoningEffort).toBe('model_default');
  });

  it('keeps explicitly cleared client metadata as the opt-out value', () => {
    const config = sanitizeDeveloperConfig({
      webSessionCodexClientName: '   ',
      webSessionCodexClientTitle: '',
      webSessionCodexClientVersion: ' ',
    });

    expect(config.webSessionCodexClientName).toBe('');
    expect(config.webSessionCodexClientTitle).toBe('');
    expect(config.webSessionCodexClientVersion).toBe('');
  });
});
