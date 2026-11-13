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
    const fieldClass = 'flex min-w-0 flex-col gap-1.5';
    return (
        <div className="grid grid-cols-2 gap-2 border-t border-border/50 pt-3 sm:grid-cols-3 xl:grid-cols-5">
            <div className={fieldClass}>
                <span className="text-xs text-muted-foreground">{t('stream')}</span>
                <Select value={stream} onValueChange={(value) => setStream(value as typeof stream)} disabled={disabled}>
                    <SelectTrigger aria-label={t('stream')} className="w-full min-w-0"><SelectValue /></SelectTrigger>
                    <SelectContent>
                        <SelectItem value="all">{tFilter('all')}</SelectItem>
                        {dimensions?.streams.map((value) => (
                            <SelectItem key={value} value={value}>{t(value === 'true' ? 'streaming' : 'nonStreaming')}</SelectItem>
                        ))}
                    </SelectContent>
                </Select>
            </div>
            {filters.map((filter) => {
                const options = filter.options;
                return (
                    <div key={filter.key} className={fieldClass}>
                        <span className="text-xs text-muted-foreground">{t(filter.key)}</span>
                        <Select value={filter.value === null ? ALL : filter.value === '' ? UNKNOWN : filter.value}
                            onValueChange={(value) => filter.set(value === ALL ? null : value === UNKNOWN ? '' : value)} disabled={disabled}>
                            <SelectTrigger aria-label={t(filter.key)} className="w-full min-w-0"><SelectValue /></SelectTrigger>
                            <SelectContent>
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
