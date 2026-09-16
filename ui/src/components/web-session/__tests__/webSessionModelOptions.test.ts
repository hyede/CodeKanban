import { describe, expect, it } from 'vitest';

import {
  CLAUDE_MODEL_OPTIONS,
  CLAUDE_RUNTIME_OPTIONS,
  CODEX_ADDITIONAL_MODEL_OPTIONS,
  CODEX_MODEL_OPTIONS,
  CODEX_PRIMARY_MODEL_OPTIONS,
  CUSTOM_MODEL_VALUE,
  MORE_MODELS_VALUE,
  defaultModelForAgent,
  defaultPermissionLevelForAgent,
  defaultReasoningEffortForAgent,
  filterPiModelOptionGroups,
  parseDevinCostSummary,
  rememberCustomModel,
  rememberPiFrequentModel,
  removeCustomModel,
  resolveCCRModelOptions,
  resolveCodexReasoningEfforts,
  resolveCustomModelOptions,
  resolveDevinFusionModel,
  resolveDevinFusionModelConfigs,
  resolveDevinModelForReasoning,
  resolveDevinModelOptionGroups,
  resolveDevinModelOptions,
  resolveDevinReasoningEfforts,
  resolveDevinSelectedReasoningEffort,
  resolveDevinSpecialModelOptions,
  resolvePiModelOptionGroups,
  resolvePiModelOptions,
  resolvePiPrimaryModelOptions,
  resolvePiReasoningEfforts,
  shouldSuppressPiModelMenuClose,
} from '@/components/web-session/webSessionModelOptions';

describe('webSessionModelOptions', () => {
  it('uses the Devin catalog for model, reasoning, and pricing metadata', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
        supportedReasoningEfforts: ['high' as const],
        costTier: 'Free',
      },
      {
        model: 'swe-2-medium',
        displayName: 'SWE-2 Medium',
        family: 'SWE-2',
        defaultReasoningEffort: 'medium' as const,
        supportedReasoningEfforts: ['medium' as const],
        costSummary: '$1 / 1M Output',
      },
    ];
    expect(resolveDevinModelOptions(models)[1].description).toBe('$1 / 1M Output');
    expect(resolveDevinReasoningEfforts(models, 'swe-2-high')).toEqual(['high', 'medium']);
    expect(resolveDevinModelForReasoning(models, 'swe-2-high', 'medium')).toBe('swe-2-medium');
    expect(resolveDevinSelectedReasoningEffort(models, 'swe-2-high', 'default')).toBe('high');
    expect(resolveDevinModelOptionGroups(models).map(group => group.label)).toEqual(['All Models']);
  });

  it('collapses Devin family variants into a single option named by the family', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
      },
      {
        model: 'swe-2-medium',
        displayName: 'SWE-2 Medium',
        family: 'SWE-2',
        defaultReasoningEffort: 'medium' as const,
      },
      {
        model: 'glm-5-2-max',
        displayName: 'GLM-5.2 Max',
        family: 'GLM-5.2',
        defaultReasoningEffort: 'max' as const,
        recommended: true,
      },
    ];
    const groups = resolveDevinModelOptionGroups(models);
    expect(groups.map(group => group.label)).toEqual(['Recommended', 'All Models']);
    const all = groups.find(group => group.key === 'devin-all');
    expect(all?.children.map(option => option.label)).toEqual(['SWE-2']);
    expect(all?.children[0].value).toBe('swe-2-high');
    expect(all?.children[0].detail?.efforts).toEqual(['medium', 'high']);
  });

  it('points a Devin family option at the currently selected variant', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
      },
      {
        model: 'swe-2-low',
        displayName: 'SWE-2 Low',
        family: 'SWE-2',
        defaultReasoningEffort: 'low' as const,
      },
    ];
    const groups = resolveDevinModelOptionGroups(models, [], 'swe-2-low');
    expect(groups[0].children[0].value).toBe('swe-2-low');
    expect(groups[0].children[0].label).toBe('SWE-2');
  });

  it('maps Devin catalog status fields to option badges without implying recommended', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
        isNew: true,
      },
      {
        model: 'glm-5-2-max',
        displayName: 'GLM-5.2 Max',
        family: 'GLM-5.2',
        defaultReasoningEffort: 'max' as const,
        isBeta: true,
        costTier: 'promotion',
      },
    ];
    const options = resolveDevinModelOptions(models);
    expect(options[0].badges).toEqual(['new']);
    expect(options[1].badges).toEqual(['promo', 'beta']);
    // Status badges do not populate the Recommended group.
    expect(resolveDevinModelOptionGroups(models).map(group => group.label)).toEqual(['All Models']);
  });

  it('dedupes Devin recent models and resolves recently used variants', () => {
    const models = [
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
        defaultReasoningEffort: 'high' as const,
      },
      {
        model: 'swe-2-low',
        displayName: 'SWE-2 Low',
        family: 'SWE-2',
        defaultReasoningEffort: 'low' as const,
      },
    ];
    const groups = resolveDevinModelOptionGroups(
      models,
      ['swe-2-low', 'swe-2-low', 'missing-model', ' swe-2-high '],
      'swe-2-low'
    );
    const recent = groups.find(group => group.key === 'devin-recent');
    expect(recent?.label).toBe('Recently Used');
    expect(recent?.children.map(option => option.value)).toEqual(['swe-2-low', 'swe-2-high']);
    expect(recent?.children[0].label).toBe('SWE-2 Low');
    expect(recent?.children[0].menuLabel).toBe('SWE-2');
    expect(recent?.children[0].accentLabel).toBe('Low');
    expect(recent?.children[0].removable).toBe(true);
  });

  it('marks removable Devin recent options without an accent for special families', () => {
    const models = [
      {
        model: 'fusion-a',
        displayName: 'Fusion (Claude Fable 5.1 Medium + SWE-2 Medium)',
        family: 'Fusion',
      },
      {
        model: 'adaptive-a',
        displayName: 'Adaptive (Auto)',
        family: 'Adaptive',
      },
    ];
    const groups = resolveDevinModelOptionGroups(models, ['fusion-a', 'adaptive-a']);
    const recent = groups.find(group => group.key === 'devin-recent');
    expect(recent?.children.map(option => option.value)).toEqual(['fusion-a', 'adaptive-a']);
    expect(recent?.children[0].removable).toBe(true);
    expect(recent?.children[0].menuLabel).toBe('Fusion (Claude Fable 5.1 Medium + SWE-2 Medium)');
    expect(recent?.children[0].accentLabel).toBeUndefined();
    expect(recent?.children[1].accentLabel).toBeUndefined();
  });

  it('exposes the Devin input price on model options', () => {
    const models = [
      {
        model: 'glm-5-2-max',
        displayName: 'GLM-5.2 Max',
        family: 'GLM-5.2',
        costSummary:
          '$10 / 1M Input · $30 / 1M Output · $1 / 1M Cache Read · $5 / 1M Cache Write · $2 / 1M Refresh · $4 / 1M Long Context',
      },
      {
        model: 'swe-2-high',
        displayName: 'SWE-2 High',
        family: 'SWE-2',
      },
    ];
    const options = resolveDevinModelOptions(models);
    expect(options[0].inputPrice).toBe('$10/1M');
    expect(options[1].inputPrice).toBeUndefined();
  });

  it('follows the selected special Devin model for fusion and adaptive options', () => {
    const models = [
      {
        model: 'fusion-a',
        displayName: 'Fusion (Claude Fable 5.1 Medium + SWE-2 Medium)',
        family: 'Fusion',
      },
      {
        model: 'fusion-b',
        displayName: 'Fusion (Claude Fable 5.1 High + SWE-2 Low)',
        family: 'Fusion',
      },
    ];
    expect(resolveDevinSpecialModelOptions(models)[0].value).toBe('fusion-a');
    expect(resolveDevinSpecialModelOptions(models, 'fusion-b')[0].value).toBe('fusion-b');
  });

  it('parses Devin fusion configs with sidekick effort and resolves selections', () => {
    const models = [
      {
        model: 'fusion-a',
        displayName: 'Fusion (Claude Fable 5.1 Medium + SWE-2 Medium)',
        family: 'Fusion',
      },
      {
        model: 'fusion-b',
        displayName: 'Fusion (Claude Fable 5.1 Medium + SWE-2 High)',
        family: 'Fusion',
      },
      {
        model: 'fusion-c',
        displayName: 'Fusion (Claude Fable 5.1 Medium Thinking + SWE-2 Medium Fast)',
        family: 'Fusion',
      },
    ];
    const configs = resolveDevinFusionModelConfigs(models);
    expect(configs).toEqual([
      {
        model: 'fusion-a',
        lead: 'Claude Fable 5.1',
        effort: 'Medium',
        sidekick: 'SWE-2',
        sidekickEffort: 'Medium',
        fast: false,
      },
      {
        model: 'fusion-b',
        lead: 'Claude Fable 5.1',
        effort: 'Medium',
        sidekick: 'SWE-2',
        sidekickEffort: 'High',
        fast: false,
      },
      {
        model: 'fusion-c',
        lead: 'Claude Fable 5.1',
        effort: 'Medium',
        sidekick: 'SWE-2',
        sidekickEffort: 'Medium',
        fast: true,
      },
    ]);
    expect(
      resolveDevinFusionModel(configs, {
        lead: 'Claude Fable 5.1',
        effort: 'Medium',
        sidekick: 'SWE-2',
        sidekickEffort: 'High',
        fast: false,
      })
    ).toBe('fusion-b');
    expect(
      resolveDevinFusionModel(configs, {
        lead: 'Missing',
        effort: 'Low',
        sidekick: 'None',
        fast: false,
      })
    ).toBe('fusion-a');
  });
  it('shows only the popular codex models by default', () => {
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.value)).toEqual([
      'gpt-5.5',
      'gpt-5.6-luna',
      'gpt-5.6-terra',
      'gpt-5.6-sol',
      'gpt-6-astra',
    ]);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.label)).toEqual([
      '5.5',
      '5.6L',
      '5.6T',
      '5.6S',
      '6A',
    ]);
    expect(CODEX_PRIMARY_MODEL_OPTIONS.map(option => option.menuLabel)).toEqual([
      'GPT-5.5',
      'GPT-5.6 Luna',
      'GPT-5.6 Terra',
      'GPT-5.6 Sol',
      'GPT-6 Astra',
    ]);
  });

  it('keeps less common codex models available without nano', () => {
    const additionalValues = CODEX_ADDITIONAL_MODEL_OPTIONS.map(option => option.value);
    const additionalLabels = CODEX_ADDITIONAL_MODEL_OPTIONS.map(option => option.label);
    const allValues = CODEX_MODEL_OPTIONS.map(option => option.value);

    expect(additionalValues).toEqual([
      'gpt-5.3-codex',
      'gpt-5.3-codex-spark',
      'gpt-5.4-mini',
      'gpt-5.4-pro',
      'gpt-5.5-pro',
    ]);
    expect(additionalLabels).toEqual(['5.3Codex', '5.3Spark', '5.4mini', '5.4Pro', '5.5Pro']);
    expect(CODEX_ADDITIONAL_MODEL_OPTIONS.map(option => option.menuLabel)).toEqual([
      'GPT-5.3 Codex',
      'GPT-5.3 Codex Spark',
      'GPT-5.4 mini',
      'GPT-5.4 Pro',
      'GPT-5.5 Pro',
    ]);
    expect(allValues).toEqual(
      expect.arrayContaining(['gpt-5.6-sol', 'gpt-5.6-luna', 'gpt-5.6-terra'])
    );
    expect(allValues).not.toContain('gpt-5.6');
    expect(allValues).not.toContain('gpt-5.4');
    expect(allValues).not.toContain('gpt-5.4-nano');
  });

  it('keeps claude models unchanged', () => {
    expect(CLAUDE_MODEL_OPTIONS.map(option => option.value)).toEqual(['opus', 'sonnet', 'haiku']);
  });

  it('uses compact claude runtime labels with full menu labels', () => {
    expect(CLAUDE_RUNTIME_OPTIONS.map(option => option.value)).toEqual(['claude', 'ccr']);
    expect(CLAUDE_RUNTIME_OPTIONS.map(option => option.label)).toEqual(['CC', 'CCR']);
    expect(CLAUDE_RUNTIME_OPTIONS.map(option => option.menuLabel)).toEqual([
      'Claude Code',
      'Claude Code Router',
    ]);
  });

  it('exports model picker sentinels', () => {
    expect(CUSTOM_MODEL_VALUE).toBe('__custom_model__');
    expect(MORE_MODELS_VALUE).toBe('__more_models__');
  });

  it('uses configurable Codex defaults without changing Claude or Pi defaults', () => {
    expect(defaultModelForAgent('codex')).toBe('gpt-5.6-sol');
    expect(defaultModelForAgent('codex', 'custom-codex-model')).toBe('custom-codex-model');
    expect(defaultModelForAgent('claude')).toBe('opus');
    expect(defaultReasoningEffortForAgent('codex')).toBe('xhigh');
    expect(defaultReasoningEffortForAgent('codex', 'high')).toBe('high');
    expect(defaultReasoningEffortForAgent('codex', 'model_default')).toBe('default');
    expect(defaultReasoningEffortForAgent('claude', 'high')).toBe('default');
    expect(defaultPermissionLevelForAgent('codex')).toBe('elevated');
    expect(defaultPermissionLevelForAgent('codex', 'standard')).toBe('default');
    expect(defaultPermissionLevelForAgent('codex', 'yolo')).toBe('yolo');
    expect(defaultPermissionLevelForAgent('claude', 'standard')).toBe('elevated');
    expect(defaultModelForAgent('pi')).toBe('');
    expect(defaultModelForAgent('devin')).toBe('swe-2-high');
    expect(defaultReasoningEffortForAgent('pi', 'high')).toBe('default');
    expect(defaultReasoningEffortForAgent('devin', 'high')).toBe('high');
    expect(defaultPermissionLevelForAgent('pi', 'standard')).toBe('elevated');
    expect(defaultPermissionLevelForAgent('devin', 'standard')).toBe('elevated');
  });

  it('uses model-specific reasoning efforts from the Codex catalog', () => {
    const catalog = [
      {
        model: 'gpt-5.6-sol',
        supportedReasoningEfforts: ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'] as const,
      },
      {
        model: 'gpt-5.6-luna',
        supportedReasoningEfforts: ['low', 'medium', 'high', 'xhigh', 'max'] as const,
      },
    ].map(item => ({ ...item, supportedReasoningEfforts: [...item.supportedReasoningEfforts] }));

    expect(resolveCodexReasoningEfforts('gpt-5.6-sol', catalog)).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-5.6-luna', catalog)).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
    ]);
  });

  it('maps the Pi catalog to stable provider/model values and supported thinking levels', () => {
    const catalog = [
      {
        provider: 'anthropic',
        id: 'claude-sonnet-4',
        name: 'Claude Sonnet 4',
        reasoning: true,
        input: ['text', 'image'],
        contextWindow: 200000,
      },
      {
        provider: 'openai',
        id: 'gpt-4.1',
        name: 'GPT-4.1',
        reasoning: false,
        input: ['text'],
        contextWindow: 1000000,
      },
    ];

    expect(resolvePiModelOptions(catalog)).toEqual([
      {
        label: 'Claude Sonnet 4',
        value: 'anthropic/claude-sonnet-4',
        menuLabel: 'Claude Sonnet 4',
      },
      { label: 'GPT-4.1', value: 'openai/gpt-4.1', menuLabel: 'GPT-4.1' },
    ]);
    expect(resolvePiModelOptionGroups(catalog)).toEqual([
      {
        type: 'group',
        key: 'pi-provider-anthropic',
        label: 'anthropic',
        children: [
          {
            label: 'Claude Sonnet 4',
            value: 'anthropic/claude-sonnet-4',
            menuLabel: 'Claude Sonnet 4',
          },
        ],
      },
      {
        type: 'group',
        key: 'pi-provider-openai',
        label: 'openai',
        children: [{ label: 'GPT-4.1', value: 'openai/gpt-4.1', menuLabel: 'GPT-4.1' }],
      },
    ]);
    expect(resolvePiReasoningEfforts(catalog, 'anthropic/claude-sonnet-4')).toContain('max');
    expect(resolvePiReasoningEfforts(catalog, 'openai/gpt-4.1')).toEqual(['default', 'none']);
  });

  it('builds a concise Pi primary list and keeps selected catalog models there', () => {
    const model = (provider: string, id: string, name = id) => ({
      provider,
      id,
      name,
      reasoning: true,
      input: ['text'],
      contextWindow: 200000,
      maxTokens: 32000,
    });
    const catalog = [
      model('openrouter', 'gpt-5.4', 'Routed GPT-5.4'),
      model('openai', 'gpt-5.4', 'GPT-5.4'),
      model('openai', 'gpt-5.5', 'GPT-5.5'),
      model('openai', 'gpt-5.6-sol', 'GPT-5.6 Sol'),
      model('openai', 'gpt-5.6-terra', 'GPT-5.6 Terra'),
      model('openai', 'gpt-5.6-luna', 'GPT-5.6 Luna'),
      model('deepseek', 'deepseek-v4-flash', 'DeepSeek V4 Flash'),
      model('deepseek', 'deepseek-v4-pro', 'DeepSeek V4 Pro'),
      model('moonshot', 'kimi-k2.5', 'Kimi K2.5'),
    ];

    expect(resolvePiPrimaryModelOptions(catalog, ['moonshot/kimi-k2.5'])).toEqual([
      { label: 'GPT-5.4', value: 'openai/gpt-5.4', menuLabel: 'GPT-5.4' },
      { label: 'GPT-5.5', value: 'openai/gpt-5.5', menuLabel: 'GPT-5.5' },
      { label: 'GPT-5.6 Sol', value: 'openai/gpt-5.6-sol', menuLabel: 'GPT-5.6 Sol' },
      { label: 'GPT-5.6 Terra', value: 'openai/gpt-5.6-terra', menuLabel: 'GPT-5.6 Terra' },
      { label: 'GPT-5.6 Luna', value: 'openai/gpt-5.6-luna', menuLabel: 'GPT-5.6 Luna' },
      {
        label: 'DeepSeek V4 Flash',
        value: 'deepseek/deepseek-v4-flash',
        menuLabel: 'DeepSeek V4 Flash',
      },
      {
        label: 'DeepSeek V4 Pro',
        value: 'deepseek/deepseek-v4-pro',
        menuLabel: 'DeepSeek V4 Pro',
      },
      { label: 'Kimi K2.5', value: 'moonshot/kimi-k2.5', menuLabel: 'Kimi K2.5' },
    ]);

    expect(
      resolvePiPrimaryModelOptions([
        model('gateway', 'deepseek-next-vision-exp', 'DeepSeek Next Vision Exp'),
        model('gateway', 'deepseek-next', 'DeepSeek Next'),
      ]).map(option => option.value)
    ).toEqual(['gateway/deepseek-next']);

    const customModels = Array.from({ length: 7 }, (_, index) => model('custom', `model-${index}`));
    const frequentValues = customModels.map(item => `${item.provider}/${item.id}`);
    expect(
      resolvePiPrimaryModelOptions([...catalog, ...customModels], frequentValues)
        .map(option => option.value)
        .filter(value => value.startsWith('custom/'))
    ).toEqual(frequentValues.slice(0, 6));
  });

  it('keeps the Pi catalog open for IME composition without blocking explicit closes', () => {
    expect(
      shouldSuppressPiModelMenuClose('show-change', {
        catalogOpen: true,
        searchFocused: true,
        searchComposing: true,
      })
    ).toBe(true);
    expect(
      shouldSuppressPiModelMenuClose('pointer-leave', {
        catalogOpen: true,
        searchFocused: true,
        searchComposing: false,
      })
    ).toBe(true);
    expect(
      shouldSuppressPiModelMenuClose('show-change', {
        catalogOpen: true,
        searchFocused: true,
        searchComposing: false,
      })
    ).toBe(false);
    expect(
      shouldSuppressPiModelMenuClose('pointer-leave', {
        catalogOpen: false,
        searchFocused: true,
        searchComposing: true,
      })
    ).toBe(false);
  });

  it('filters the full Pi catalog by provider, model name, or model id', () => {
    const groups = resolvePiModelOptionGroups([
      {
        provider: 'anthropic',
        id: 'claude-sonnet-4',
        name: 'Claude Sonnet 4',
        reasoning: true,
        input: ['text'],
        contextWindow: 200000,
        maxTokens: 32000,
      },
      {
        provider: 'openai',
        id: 'gpt-4.1',
        name: 'GPT 4.1',
        reasoning: false,
        input: ['text'],
        contextWindow: 1000000,
        maxTokens: 32000,
      },
    ]);

    expect(filterPiModelOptionGroups(groups, 'OPENAI')).toEqual([groups[1]]);
    expect(filterPiModelOptionGroups(groups, 'sonnet')).toEqual([groups[0]]);
    expect(filterPiModelOptionGroups(groups, 'gpt-4.1')).toEqual([groups[1]]);
    expect(filterPiModelOptionGroups(groups, 'missing')).toEqual([]);
  });

  it('keeps Pi user-selected primary models unique and bounded', () => {
    expect(rememberPiFrequentModel(['provider/one', 'provider/two'], ' provider/two ')).toEqual([
      'provider/two',
      'provider/one',
    ]);
    expect(rememberPiFrequentModel(['provider/one', 'provider/two'], 'provider/three', 2)).toEqual([
      'provider/three',
      'provider/one',
    ]);
  });

  it('remembers custom models at the front without duplicates', () => {
    expect(rememberCustomModel(['glm-5.3', 'gpt-oss'], 'glm-5.3')).toEqual(['glm-5.3', 'gpt-oss']);
    expect(rememberCustomModel(['glm-5.3'], ' new-model ')).toEqual(['new-model', 'glm-5.3']);
    expect(rememberCustomModel([], '   ')).toEqual([]);
    expect(
      rememberCustomModel(
        Array.from({ length: 20 }, (_, index) => `model-${index}`),
        'model-new'
      )
    ).toEqual(['model-new', ...Array.from({ length: 19 }, (_, index) => `model-${index}`)]);
  });

  it('removes custom models while ignoring blank or missing entries', () => {
    expect(removeCustomModel(['glm-5.3', 'gpt-oss'], ' glm-5.3 ')).toEqual(['gpt-oss']);
    expect(removeCustomModel(['glm-5.3'], 'missing')).toEqual(['glm-5.3']);
    expect(removeCustomModel(['glm-5.3'], '')).toEqual(['glm-5.3']);
    expect(removeCustomModel(undefined as unknown as string[], 'glm-5.3')).toEqual([]);
  });

  it('maps stored custom models to options and skips reserved or duplicate values', () => {
    expect(resolveCustomModelOptions(['glm-5.3', ' opus ', 'glm-5.3', ''], ['opus'])).toEqual([
      { label: 'glm-5.3', value: 'glm-5.3' },
    ]);
    expect(resolveCustomModelOptions(undefined as unknown as string[])).toEqual([]);
  });

  it('falls back per current Codex model without exposing unsupported efforts', () => {
    expect(resolveCodexReasoningEfforts('gpt-6-astra')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-5.6-terra')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
      'ultra',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-5.6-luna')).toEqual([
      'low',
      'medium',
      'high',
      'xhigh',
      'max',
    ]);
    expect(resolveCodexReasoningEfforts('gpt-5.6-luna')).not.toContain('none');
    expect(resolveCodexReasoningEfforts('gpt-5.6-luna')).not.toContain('ultra');
  });
});

describe('resolveCCRModelOptions', () => {
  it('prefers display names as labels and keeps gateway ids as values', () => {
    expect(
      resolveCCRModelOptions([
        { model: 'ZCode API/GLM-5.3', provider: 'ZCode API' },
        {
          model: 'DeepSeek/deepseek-flash',
          provider: 'DeepSeek',
          displayName: 'DeepSeek V4.1 Flash',
        },
        { model: '   ', provider: 'broken' },
      ])
    ).toEqual([
      {
        label: 'ZCode API/GLM-5.3',
        value: 'ZCode API/GLM-5.3',
        menuLabel: 'ZCode API/GLM-5.3',
      },
      {
        label: 'DeepSeek V4.1 Flash',
        value: 'DeepSeek/deepseek-flash',
        menuLabel: 'DeepSeek/deepseek-flash',
      },
    ]);
  });

  it('returns no options when nothing has been sniffed', () => {
    expect(resolveCCRModelOptions([])).toEqual([]);
  });
});

describe('parseDevinCostSummary', () => {
  it('splits the real fusion cost summary into labeled price items', () => {
    const summary =
      '$10 / 1M Input · $0.25 / 1M Cached input · $50 / 1M Output · $0.2 / 1M Sidekick input · $0.02 / 1M Sidekick cached input · $1.2 / 1M Sidekick output';
    expect(parseDevinCostSummary(summary)).toEqual([
      { price: '$10', unit: '1M', label: 'Input' },
      { price: '$0.25', unit: '1M', label: 'Cached input' },
      { price: '$50', unit: '1M', label: 'Output' },
      { price: '$0.2', unit: '1M', label: 'Sidekick input' },
      { price: '$0.02', unit: '1M', label: 'Sidekick cached input' },
      { price: '$1.2', unit: '1M', label: 'Sidekick output' },
    ]);
  });

  it('returns unmatched entries as label-only items with empty price and unit', () => {
    expect(parseDevinCostSummary('Flat rate · $1 / 1M Output')).toEqual([
      { price: '', unit: '', label: 'Flat rate' },
      { price: '$1', unit: '1M', label: 'Output' },
    ]);
  });

  it('returns an empty list for undefined or blank summaries', () => {
    expect(parseDevinCostSummary(undefined)).toEqual([]);
    expect(parseDevinCostSummary('   ')).toEqual([]);
    expect(parseDevinCostSummary('')).toEqual([]);
  });
});
