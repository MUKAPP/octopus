import { useMemo, useState } from 'react';
import { useTranslations } from 'use-intl';
import { RefreshCw, CalendarDays } from 'lucide-react';
import type { UseQueryResult } from '@tanstack/react-query';
import { Calendar } from '@/components/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import {
    useAnalyticsDimensions,
    type AnalyticsPeriod,
    type AnalyticsResponseFormatted,
} from '@/api/endpoints/analytics';
import { useAnalyticsViewStore } from './store';
import { cn } from '@/lib/utils';

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
    const [customOpen, setCustomOpen] = useState(false);

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

    const dimensionsQuery = useAnalyticsDimensions();
    const dimensions = dimensionsQuery.data;
    const dimensionsFailed = dimensionsQuery.isError && !dimensions;

    const presetButtons: { value: AnalyticsPeriod; label: string }[] = [
        { value: 'today', label: t('filter.datePreset.today') },
        { value: '7d', label: t('filter.datePreset.last7Days') },
        { value: '30d', label: t('filter.datePreset.last30Days') },
        { value: 'all', label: t('filter.datePreset.all') },
    ];

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

    const chips = useMemo(() => {
        const list: { key: string; label: string }[] = [];
        if (period !== '30d') {
            list.push({ key: 'period', label: period === 'custom' ? customLabel : t(`filter.datePreset.${period}`) });
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
        return list;
    }, [period, customLabel, model, actualModel, apiKey, channel, status, t]);

    const presetButtonClass = (active: boolean) =>
        cn(
            'rounded-xl border px-3 py-1.5 text-sm transition-colors',
            active
                ? 'bg-primary text-primary-foreground border-primary/30'
                : 'border-border bg-muted/20 text-foreground hover:bg-muted/30'
        );

    return (
        <div className="rounded-3xl bg-card border-card-border border p-4 text-card-foreground custom-shadow">
            <div className="flex flex-wrap items-end gap-4">
                {/* 日期 */}
                <div className="flex flex-col gap-1.5">
                    <span className="text-xs text-muted-foreground">{t('filter.date')}</span>
                    <div className="flex flex-wrap items-center gap-2">
                        {presetButtons.map((preset) => (
                            <button
                                key={preset.value}
                                type="button"
                                onClick={() => selectPreset(preset.value)}
                                aria-pressed={period === preset.value}
                                className={presetButtonClass(period === preset.value)}
                            >
                                {preset.label}
                            </button>
                        ))}
                        <Popover open={customOpen} onOpenChange={setCustomOpen}>
                            <PopoverTrigger asChild>
                                <button
                                    type="button"
                                    onClick={() => selectPreset('custom')}
                                    aria-pressed={period === 'custom'}
                                    className={cn(presetButtonClass(period === 'custom'), 'flex items-center gap-1.5')}
                                >
                                    <CalendarDays className="size-4" />
                                    <span className="max-w-48 truncate">{customLabel}</span>
                                </button>
                            </PopoverTrigger>
                            <PopoverContent
                                align="start"
                                side="bottom"
                                sideOffset={8}
                                className="w-fit rounded-2xl border border-border/60 shadow-xl overflow-hidden bg-card p-0"
                            >
                                <Calendar
                                    mode="range"
                                    selected={customRange}
                                    onSelect={handleRangeSelect}
                                    classNames={{ today: '' }}
                                />
                            </PopoverContent>
                        </Popover>
                    </div>
                    {customInvalid && (
                        <p role="alert" className="text-xs text-destructive">{customInvalid}</p>
                    )}
                </div>

                {/* 请求模型 */}
                <div className="flex grow flex-col gap-1.5 sm:grow-0">
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
                        <SelectTrigger aria-label={t('filter.requestModel')} className="min-w-40 w-full rounded-xl">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl">
                            <SelectItem className="rounded-xl" value={ALL_VALUE}>{t('filter.all')}</SelectItem>
                            {modelOptions.map((name) => (
                                <SelectItem className="rounded-xl" key={name || UNKNOWN_VALUE} value={name === '' ? UNKNOWN_VALUE : name}>
                                    {name === '' ? t('filter.unknown') : name}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </div>

                {/* 实际上游模型 */}
                <div className="flex grow flex-col gap-1.5 sm:grow-0">
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
                        <SelectTrigger aria-label={t('filter.actualModel')} className="min-w-40 w-full rounded-xl">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl">
                            <SelectItem className="rounded-xl" value={ALL_VALUE}>{t('filter.all')}</SelectItem>
                            {actualModelOptions.map((name) => (
                                <SelectItem className="rounded-xl" key={name || UNKNOWN_VALUE} value={name === '' ? UNKNOWN_VALUE : name}>
                                    {name === '' ? t('filter.unknown') : name}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </div>

                {/* API Key */}
                <div className="flex grow flex-col gap-1.5 sm:grow-0">
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
                        <SelectTrigger aria-label={t('filter.apiKey')} className="min-w-40 w-full rounded-xl">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl">
                            <SelectItem className="rounded-xl" value={ALL_VALUE}>{t('filter.all')}</SelectItem>
                            {apiKeyOptions.map((item) => (
                                <SelectItem className="rounded-xl" key={item.id} value={String(item.id)}>
                                    {item.name || t('filter.unassigned')}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </div>

                {/* 渠道 */}
                <div className="flex grow flex-col gap-1.5 sm:grow-0">
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
                        <SelectTrigger aria-label={t('filter.channel')} className="min-w-40 w-full rounded-xl">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl">
                            <SelectItem className="rounded-xl" value={ALL_VALUE}>{t('filter.all')}</SelectItem>
                            {channelOptions.map((item) => (
                                <SelectItem className="rounded-xl" key={item.id} value={String(item.id)}>
                                    {item.name || t('filter.unassigned')}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </div>

                {/* 状态 */}
                <div className="flex grow flex-col gap-1.5 sm:grow-0">
                    <span className="text-xs text-muted-foreground">{t('filter.status')}</span>
                    <Select value={status} onValueChange={(value) => setStatus(value as typeof status)}>
                        <SelectTrigger aria-label={t('filter.status')} className="min-w-40 w-full rounded-xl">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl">
                            <SelectItem className="rounded-xl" value="all">{t('filter.statusOption.all')}</SelectItem>
                            <SelectItem className="rounded-xl" value="success">{t('filter.statusOption.success')}</SelectItem>
                            <SelectItem className="rounded-xl" value="failed">{t('filter.statusOption.failed')}</SelectItem>
                        </SelectContent>
                    </Select>
                </div>
            </div>

            {dimensionsFailed && (
                <div role="alert" className="mt-3 flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-2.5 text-sm text-muted-foreground">
                    <span>{t('filter.dimensionLoadFailed')}</span>
                    <button
                        type="button"
                        onClick={() => void dimensionsQuery.refetch()}
                        className="shrink-0 rounded-xl border border-border px-3 py-1.5 font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                    >
                        {t('filter.dimensionRetry')}
                    </button>
                </div>
            )}

            {chips.length > 0 && (
                <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-border/50 pt-3">
                    <div role="group" aria-label={t('filter.activeFilters')} className="flex flex-wrap items-center gap-2">
                        {chips.map((chip) => (
                            <span
                                key={chip.key}
                                className="rounded-full border border-border bg-muted/20 px-3 py-1 text-xs text-muted-foreground"
                            >
                                {chip.label}
                            </span>
                        ))}
                        <button
                            type="button"
                            onClick={clearFilters}
                            className="rounded-full border border-border bg-muted/20 px-3 py-1 text-xs text-foreground transition-colors hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                        >
                            {t('feedback.clearFilters')}
                        </button>
                    </div>
                </div>
            )}
            {hasMetadata && (
                <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-border/50 pt-3 text-xs text-muted-foreground">
                    {data?.available_from && (
                        <span>{t('availableFrom', { date: formatDisplayDate(data.available_from) })}</span>
                    )}
                    {showResolvedRange && data && (
                        <span>
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
                        className="ml-auto rounded-xl border border-border p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                    >
                        <RefreshCw className={cn('size-4', query.isFetching && 'animate-spin')} />
                    </button>
                </div>
            )}
        </div>
    );
}
