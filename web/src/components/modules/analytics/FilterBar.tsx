import { useMemo, useState } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'use-intl';
import { RefreshCw, CalendarDays, ChevronDown, SlidersHorizontal } from 'lucide-react';
import type { UseQueryResult } from '@tanstack/react-query';
import { Highlight, HighlightItem } from '@/components/animate-ui/primitives/effects/highlight';
import { Calendar } from '@/components/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import {
    useAnalyticsDimensions,
    type AnalyticsPeriod,
    type AnalyticsResponseFormatted,
} from '@/api/endpoints/analytics';
import { useAnalyticsViewStore } from './store';
import { MetadataFilters } from './MetadataFilters';
import { cn } from '@/lib/utils';
import { InlineRetry } from '@/components/common/InlineRetry';
import { EASING, REDUCED_MOTION_TRANSITION } from '@/lib/animations/fluid-transitions';

/** UI 哨兵值：仅存在于 Select 值层，绝不发送到后端 */
const ALL_VALUE = '__all__';
/** UI 哨兵值：空名称（未知/未分配），仅存在于 Select 值层 */
const UNKNOWN_VALUE = '__unknown__';

/**
 * 解析本地日历日 YYYY-MM-DD（不做时区换算）
 */
function parseLocalDate(value: string): Date | undefined {
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
    if (!match) return undefined;
    return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
}

/**
 * 格式化为本地日历日 YYYY-MM-DD（不做时区换算）
 */
function formatLocalDate(date: Date): string {
    const y = date.getFullYear();
    const m = String(date.getMonth() + 1).padStart(2, '0');
    const d = String(date.getDate()).padStart(2, '0');
    return `${y}-${m}-${d}`;
}

interface FilterBarProps {
    query: UseQueryResult<AnalyticsResponseFormatted>;
}

export function FilterBar({ query }: FilterBarProps) {
    const data = query.data;

    const showResolvedRange = Boolean(data?.resolved_start_date && data.resolved_end_date);

    const formatDisplayDate = (value: string) => {
        if (value.length !== 8) return value;
        return `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
    };

    const hasMetadata = Boolean(data?.available_from || showResolvedRange);

    const t = useTranslations('analytics');
    const shouldReduceMotion = useReducedMotion() ?? false;
    const [customOpen, setCustomOpen] = useState(false);
    const [advancedOpen, setAdvancedOpen] = useState(false);

    const period = useAnalyticsViewStore((state) => state.period);
    const setPeriod = useAnalyticsViewStore((state) => state.setPeriod);
    const customStartDate = useAnalyticsViewStore((state) => state.customStartDate);
    const customEndDate = useAnalyticsViewStore((state) => state.customEndDate);
    const setCustomStartDate = useAnalyticsViewStore((state) => state.setCustomStartDate);
    const setCustomEndDate = useAnalyticsViewStore((state) => state.setCustomEndDate);
    const model = useAnalyticsViewStore((state) => state.model);
    const setModel = useAnalyticsViewStore((state) => state.setModel);
    const actualModel = useAnalyticsViewStore((state) => state.actualModel);
    const setActualModel = useAnalyticsViewStore((state) => state.setActualModel);
    const apiKey = useAnalyticsViewStore((state) => state.apiKey);
    const setApiKey = useAnalyticsViewStore((state) => state.setApiKey);
    const channel = useAnalyticsViewStore((state) => state.channel);
    const setChannel = useAnalyticsViewStore((state) => state.setChannel);
    const status = useAnalyticsViewStore((state) => state.status);
    const setStatus = useAnalyticsViewStore((state) => state.setStatus);
    const clearFilters = useAnalyticsViewStore((state) => state.clearFilters);
    const stream = useAnalyticsViewStore((state) => state.stream);
    const reasoningEffort = useAnalyticsViewStore((state) => state.reasoningEffort);
    const taskType = useAnalyticsViewStore((state) => state.taskType);
    const clientProtocol = useAnalyticsViewStore((state) => state.clientProtocol);
    const errorCategory = useAnalyticsViewStore((state) => state.errorCategory);

    const dimensionsQuery = useAnalyticsDimensions();
    const dimensions = dimensionsQuery.data;
    const dimensionsFailed = dimensionsQuery.isError && !dimensions;

    const presetButtons = useMemo<{ value: AnalyticsPeriod; label: string }[]>(
        () => [
            { value: 'today', label: t('filter.datePreset.today') },
            { value: '7d', label: t('filter.datePreset.last7Days') },
            { value: '30d', label: t('filter.datePreset.last30Days') },
            { value: 'all', label: t('filter.datePreset.all') },
        ],
        [t],
    );

    const customRange = useMemo(() => {
        if (period !== 'custom') return undefined;
        return {
            from: customStartDate ? parseLocalDate(customStartDate) : undefined,
            to: customEndDate ? parseLocalDate(customEndDate) : undefined,
        };
    }, [period, customStartDate, customEndDate]);

    const handleRangeSelect = (range: { from?: Date; to?: Date } | undefined) => {
        setCustomStartDate(range?.from ? formatLocalDate(range.from) : null);
        setCustomEndDate(range?.to ? formatLocalDate(range.to) : null);
    };

    const selectPreset = (value: AnalyticsPeriod) => {
        setPeriod(value);
        if (value !== 'custom') {
            setCustomOpen(false);
        }
    };

    // 维度选项（后端已排序；补充当前已选但已不存在的实体，保证持久化选择可回显）
    const modelOptions = useMemo(() => {
        if (!dimensions) return [];
        const list = [...dimensions.models];
        if (model !== null && model !== '' && !list.includes(model)) {
            list.push(model);
        }
        return list.sort((a, b) => a.localeCompare(b));
    }, [dimensions, model]);

    const actualModelOptions = useMemo(() => {
        if (!dimensions) return [];
        const list = [...dimensions.actual_models];
        if (actualModel !== null && actualModel !== '' && !list.includes(actualModel)) {
            list.push(actualModel);
        }
        return list.sort((a, b) => a.localeCompare(b));
    }, [dimensions, actualModel]);

    const apiKeyOptions = useMemo(() => {
        if (!dimensions) return [];
        const list = [...dimensions.api_keys];
        if (apiKey && !list.some((item) => item.id === apiKey.id)) {
            list.push(apiKey);
        }
        return list.sort((a, b) => a.name.localeCompare(b.name) || a.id - b.id);
    }, [dimensions, apiKey]);

    const channelOptions = useMemo(() => {
        if (!dimensions) return [];
        const list = [...dimensions.channels];
        if (channel && !list.some((item) => item.id === channel.id)) {
            list.push(channel);
        }
        return list.sort((a, b) => a.name.localeCompare(b.name) || a.id - b.id);
    }, [dimensions, channel]);

    const customLabel = useMemo(() => {
        if (customStartDate && customEndDate) {
            return `${customStartDate} – ${customEndDate}`;
        }
        return t('filter.datePreset.custom');
    }, [customStartDate, customEndDate, t]);

    const customInvalid = useMemo(() => {
        if (period !== 'custom') return null;
        if (!customStartDate || !customEndDate) return t('filter.custom.incompleteRange');
        return customStartDate > customEndDate ? t('filter.custom.invalidRange') : null;
    }, [period, customStartDate, customEndDate, t]);

    const advancedCount = [stream !== 'all', reasoningEffort, taskType, clientProtocol, errorCategory].filter(Boolean).length;

    const chips = useMemo(() => {
        const list: { key: string; label: string }[] = [];
        if (period !== '30d') {
            const presetLabel = presetButtons.find((item) => item.value === period)?.label;
            list.push({ key: 'period', label: period === 'custom' ? customLabel : (presetLabel ?? customLabel) });
        }
        if (model !== null) {
            list.push({ key: 'model', label: model === '' ? t('filter.unknown') : model });
        }
        if (actualModel !== null) {
            list.push({ key: 'actualModel', label: actualModel === '' ? t('filter.unknown') : actualModel });
        }
        if (apiKey !== null) {
            list.push({ key: 'apiKey', label: apiKey.name || t('filter.unassigned') });
        }
        if (channel !== null) {
            list.push({ key: 'channel', label: channel.name || t('filter.unassigned') });
        }
        if (status !== 'all') {
            list.push({ key: 'status', label: t(`filter.statusOption.${status}`) });
        }
        if (stream !== 'all') list.push({ key: 'stream', label: t(`detail.${stream === 'true' ? 'streaming' : 'nonStreaming'}`) });
        for (const [key, value] of Object.entries({ reasoningEffort, taskType, clientProtocol, errorCategory })) {
            if (value !== null) list.push({ key, label: `${t(`detail.${key}`)}: ${value || t('filter.unknown')}` });
        }
        return list;
    }, [period, presetButtons, customLabel, model, actualModel, apiKey, channel, status, stream, reasoningEffort, taskType, clientProtocol, errorCategory, t]);

    return (
        <div className="rounded-3xl border border-border bg-card p-3 text-card-foreground sm:p-4">
            {/* 主筛选：日期分段 + 关键维度 */}
            <div className="flex flex-col gap-3">
                <div className="flex flex-wrap items-center gap-2">
                    <div className="flex items-center gap-1 rounded-2xl bg-muted/40 p-1">
                        <Highlight
                            controlledItems
                            value={period}
                            click={false}
                            transition={shouldReduceMotion ? { duration: 0 } : { type: 'spring', stiffness: 200, damping: 25 }}
                            className="absolute inset-0 z-0 rounded-xl bg-card shadow-sm"
                        >
                            {presetButtons.map((preset) => (
                                <HighlightItem key={preset.value} value={preset.value}>
                                    <button
                                        type="button"
                                        onClick={() => selectPreset(preset.value)}
                                        aria-pressed={period === preset.value}
                                        className={cn(
                                            'rounded-xl px-2.5 py-1.5 text-sm transition-colors duration-150 ease-out motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                                            period === preset.value
                                                ? 'font-medium text-foreground'
                                                : 'text-muted-foreground hover:text-foreground'
                                        )}
                                    >
                                        {preset.label}
                                    </button>
                                </HighlightItem>
                            ))}
                            <Popover open={customOpen} onOpenChange={setCustomOpen}>
                                <HighlightItem value="custom">
                                    <PopoverTrigger asChild>
                                        <button
                                            type="button"
                                            onClick={() => selectPreset('custom')}
                                            aria-pressed={period === 'custom'}
                                            aria-label={t('filter.date')}
                                            className={cn(
                                                'flex items-center gap-1.5 rounded-xl px-2.5 py-1.5 text-sm transition-colors duration-150 ease-out motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                                                period === 'custom'
                                                    ? 'font-medium text-foreground'
                                                    : 'text-muted-foreground hover:text-foreground'
                                            )}
                                        >
                                            <CalendarDays className="size-4" />
                                            <span className="max-w-40 truncate">{customLabel}</span>
                                        </button>
                                    </PopoverTrigger>
                                </HighlightItem>
                                <PopoverContent
                                    align="start"
                                    side="bottom"
                                    sideOffset={8}
                                    className="w-fit overflow-hidden rounded-2xl border border-border/60 bg-card p-0 shadow-xl"
                                >
                                    <Calendar
                                        mode="range"
                                        selected={customRange}
                                        onSelect={handleRangeSelect}
                                        classNames={{ today: '' }}
                                    />
                                </PopoverContent>
                            </Popover>
                        </Highlight>
                    </div>
                    {customInvalid && <p role="alert" className="text-xs text-destructive">{customInvalid}</p>}
                </div>

                <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6">
                    <div className="flex min-w-0 flex-col gap-1.5">
                        <span className="text-xs text-muted-foreground">{t('filter.requestModel')}</span>
                        <Select
                            value={model === null ? ALL_VALUE : model === '' ? UNKNOWN_VALUE : model}
                            onValueChange={(value) => {
                                if (value === ALL_VALUE) setModel(null);
                                else if (value === UNKNOWN_VALUE) setModel('');
                                else setModel(value);
                            }}
                            disabled={dimensionsFailed}
                        >
                            <SelectTrigger aria-label={t('filter.requestModel')} className="w-full min-w-0">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value={ALL_VALUE}>{t('filter.all')}</SelectItem>
                                {modelOptions.map((name) => (
                                    <SelectItem key={name || UNKNOWN_VALUE} value={name === '' ? UNKNOWN_VALUE : name}>
                                        {name === '' ? t('filter.unknown') : name}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>

                    <div className="flex min-w-0 flex-col gap-1.5">
                        <span className="text-xs text-muted-foreground">{t('filter.actualModel')}</span>
                        <Select
                            value={actualModel === null ? ALL_VALUE : actualModel === '' ? UNKNOWN_VALUE : actualModel}
                            onValueChange={(value) => {
                                if (value === ALL_VALUE) setActualModel(null);
                                else if (value === UNKNOWN_VALUE) setActualModel('');
                                else setActualModel(value);
                            }}
                            disabled={dimensionsFailed}
                        >
                            <SelectTrigger aria-label={t('filter.actualModel')} className="w-full min-w-0">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value={ALL_VALUE}>{t('filter.all')}</SelectItem>
                                {actualModelOptions.map((name) => (
                                    <SelectItem key={name || UNKNOWN_VALUE} value={name === '' ? UNKNOWN_VALUE : name}>
                                        {name === '' ? t('filter.unknown') : name}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>

                    <div className="flex min-w-0 flex-col gap-1.5">
                        <span className="text-xs text-muted-foreground">{t('filter.apiKey')}</span>
                        <Select
                            value={apiKey === null ? ALL_VALUE : String(apiKey.id)}
                            onValueChange={(value) => {
                                if (value === ALL_VALUE) {
                                    setApiKey(null);
                                    return;
                                }
                                const selected = apiKeyOptions.find((item) => String(item.id) === value);
                                setApiKey(selected ? { id: selected.id, name: selected.name } : null);
                            }}
                            disabled={dimensionsFailed}
                        >
                            <SelectTrigger aria-label={t('filter.apiKey')} className="w-full min-w-0">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value={ALL_VALUE}>{t('filter.all')}</SelectItem>
                                {apiKeyOptions.map((item) => (
                                    <SelectItem key={item.id} value={String(item.id)}>
                                        {item.name || t('filter.unassigned')}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>

                    <div className="flex min-w-0 flex-col gap-1.5">
                        <span className="text-xs text-muted-foreground">{t('filter.channel')}</span>
                        <Select
                            value={channel === null ? ALL_VALUE : String(channel.id)}
                            onValueChange={(value) => {
                                if (value === ALL_VALUE) {
                                    setChannel(null);
                                    return;
                                }
                                const selected = channelOptions.find((item) => String(item.id) === value);
                                setChannel(selected ? { id: selected.id, name: selected.name } : null);
                            }}
                            disabled={dimensionsFailed}
                        >
                            <SelectTrigger aria-label={t('filter.channel')} className="w-full min-w-0">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value={ALL_VALUE}>{t('filter.all')}</SelectItem>
                                {channelOptions.map((item) => (
                                    <SelectItem key={item.id} value={String(item.id)}>
                                        {item.name || t('filter.unassigned')}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>

                    <div className="flex min-w-0 flex-col gap-1.5">
                        <span className="text-xs text-muted-foreground">{t('filter.status')}</span>
                        <Select value={status} onValueChange={(value) => setStatus(value as typeof status)}>
                            <SelectTrigger aria-label={t('filter.status')} className="w-full min-w-0">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="all">{t('filter.statusOption.all')}</SelectItem>
                                <SelectItem value="success">{t('filter.statusOption.success')}</SelectItem>
                                <SelectItem value="failed">{t('filter.statusOption.failed')}</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>

                    <div className="col-span-2 flex items-end sm:col-span-1">
                        <button
                            type="button"
                            onClick={() => setAdvancedOpen((value) => !value)}
                            aria-expanded={advancedOpen}
                            className={cn( 'flex h-9 w-full items-center justify-center gap-1.5 rounded-xl border px-3 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                                advancedOpen || advancedCount > 0
                                    ? 'border-primary/30 bg-primary/10 text-foreground'
                                    : 'border-border bg-muted/20 text-muted-foreground hover:bg-muted/30 hover:text-foreground'
                            )}
                        >
                            <SlidersHorizontal className="size-4" />
                            <span>{t('filter.advanced')}</span>
                            {advancedCount > 0 && (
                                <span className="rounded-full bg-primary px-1.5 text-xs font-medium text-primary-foreground">{advancedCount}</span>
                            )}
                            <ChevronDown className={cn('size-4 transition-transform duration-200 motion-reduce:transition-none', advancedOpen && 'rotate-180')} />
                        </button>
                    </div>
                </div>
            </div>

            <AnimatePresence initial={false}>
                {advancedOpen && (
                    <motion.div
                        key="advanced-filters"
                        initial={shouldReduceMotion ? { opacity: 0 } : { height: 0, opacity: 0, filter: 'blur(4px)' }}
                        animate={shouldReduceMotion ? { opacity: 1 } : { height: 'auto', opacity: 1, filter: 'blur(0px)' }}
                        exit={shouldReduceMotion ? { opacity: 0 } : { height: 0, opacity: 0, filter: 'blur(4px)' }}
                        transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: 0.24, ease: EASING.easeOutCubic }}
                        className="-mx-1 -mb-1 overflow-hidden px-1 pb-1"
                    >
                        <div className="pt-3">
                            <MetadataFilters dimensions={dimensions} disabled={dimensionsFailed} />
                        </div>
                    </motion.div>
                )}
            </AnimatePresence>

            {dimensionsFailed && (
                <div role="alert" className="mt-3 flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-2.5 text-sm text-muted-foreground">
                    <span>{t('filter.dimensionLoadFailed')}</span>
                    <InlineRetry onRetry={() => void dimensionsQuery.refetch()} label={t('filter.dimensionRetry')} className="shrink-0" />
                </div>
            )}

            {(chips.length > 0 || hasMetadata) && (
                <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-border/50 pt-3">
                    {chips.length > 0 && (
                        <div role="group" aria-label={t('filter.activeFilters')} className="flex flex-wrap items-center gap-1.5">
                            {chips.map((chip) => (
                                <span
                                    key={chip.key}
                                    className="max-w-56 truncate rounded-full border border-border bg-muted/20 px-2.5 py-1 text-xs text-muted-foreground"
                                >
                                    {chip.label}
                                </span>
                            ))}
                            <button
                                type="button"
                                onClick={clearFilters}
                                className="rounded-full border border-border bg-muted/20 px-2.5 py-1 text-xs text-foreground transition-colors hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                            >
                                {t('feedback.clearFilters')}
                            </button>
                        </div>
                    )}
                    <div className="ml-auto flex items-center gap-3 text-xs text-muted-foreground">
                        {data?.available_from && (
                            <span>{t('availableFrom', { date: formatDisplayDate(data.available_from) })}</span>
                        )}
                        {showResolvedRange && data && (
                            <span className="hidden sm:inline">
                                {t('resolvedRange', {
                                    start: formatDisplayDate(data.resolved_start_date),
                                    end: formatDisplayDate(data.resolved_end_date),
                                })}
                            </span>
                        )}
                        <button
                            type="button"
                            onClick={() => void query.refetch()}
                            aria-label={t('feedback.refresh')}
                            className="rounded-xl border border-border p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                        >
                            <RefreshCw className={cn('size-3.5', query.isFetching && 'animate-spin motion-reduce:animate-none')} />
                        </button>
                    </div>
                </div>
            )}
        </div>
    );
}
