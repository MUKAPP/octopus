import { useTranslations } from 'use-intl';
import type { AnalyticsDimensions } from '@/api/endpoints/analytics';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useAnalyticsViewStore } from './store';

interface MetadataFiltersProps {
    dimensions: AnalyticsDimensions | undefined;
    disabled: boolean;
}

const ALL = '__all__';
const UNKNOWN = '__unknown__';

export function MetadataFilters({ dimensions, disabled }: MetadataFiltersProps) {
    const t = useTranslations('analytics.detail');
    const tFilter = useTranslations('analytics.filter');
    const { stream, setStream, reasoningEffort, setReasoningEffort, taskType, setTaskType,
        clientProtocol, setClientProtocol, errorCategory, setErrorCategory } = useAnalyticsViewStore();
    const filters = [
        { key: 'reasoningEffort', value: reasoningEffort, set: setReasoningEffort, options: dimensions?.reasoning_efforts ?? [] },
        { key: 'taskType', value: taskType, set: setTaskType, options: dimensions?.task_types ?? [] },
        { key: 'clientProtocol', value: clientProtocol, set: setClientProtocol, options: dimensions?.client_protocols ?? [] },
        { key: 'errorCategory', value: errorCategory, set: setErrorCategory, options: dimensions?.error_categories ?? [] },
    ];
    return (
        <div className="mt-4 grid grid-cols-1 gap-3 border-t border-border/50 pt-4 sm:grid-cols-2 xl:grid-cols-5">
            <div className="flex min-w-0 flex-col gap-1.5">
                <span className="text-xs text-muted-foreground">{t('stream')}</span>
                <Select value={stream} onValueChange={(value) => setStream(value as typeof stream)}>
                    <SelectTrigger aria-label={t('stream')} className="w-full rounded-xl"><SelectValue /></SelectTrigger>
                    <SelectContent className="rounded-xl">
                        <SelectItem value="all">{tFilter('all')}</SelectItem>
                        <SelectItem value="true">{t('streaming')}</SelectItem>
                        <SelectItem value="false">{t('nonStreaming')}</SelectItem>
                    </SelectContent>
                </Select>
            </div>
            {filters.map((filter) => {
                const options = [...new Set([...filter.options, ...(filter.value === null ? [] : [filter.value])])];
                return (
                    <div key={filter.key} className="flex min-w-0 flex-col gap-1.5">
                        <span className="text-xs text-muted-foreground">{t(filter.key)}</span>
                        <Select value={filter.value === null ? ALL : filter.value === '' ? UNKNOWN : filter.value}
                            onValueChange={(value) => filter.set(value === ALL ? null : value === UNKNOWN ? '' : value)} disabled={disabled}>
                            <SelectTrigger aria-label={t(filter.key)} className="w-full rounded-xl"><SelectValue /></SelectTrigger>
                            <SelectContent className="rounded-xl">
                                <SelectItem value={ALL}>{tFilter('all')}</SelectItem>
                                {options.map((option) => (
                                    <SelectItem key={option || UNKNOWN} value={option || UNKNOWN}>
                                        {option === '' ? tFilter('unknown') : filter.key === 'errorCategory' && t.has(`error.${option}`) ? t(`error.${option}`) : filter.key === 'taskType' && t.has(`task.${option}`) ? t(`task.${option}`) : option}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                );
            })}
        </div>
    );
}
