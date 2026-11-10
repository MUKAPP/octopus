import type { LLMPriceSchedule, LLMTimePrice } from '@/api/model';

export interface PriceScheduleDraft {
    timezone: string;
    excludedDates: string;
    rules: LLMTimePrice[];
}

export function toScheduleDraft(schedule?: LLMPriceSchedule | null): PriceScheduleDraft | null {
    return schedule ? {
        timezone: schedule.timezone,
        excludedDates: schedule.excluded_dates?.join(', ') ?? '',
        rules: schedule.rules.map(rule => ({ ...rule, weekdays: rule.weekdays ? [...rule.weekdays] : [] })),
    } : null;
}

export function fromScheduleDraft(draft: PriceScheduleDraft | null): LLMPriceSchedule | null {
    return draft ? {
        timezone: draft.timezone.trim(),
        excluded_dates: draft.excludedDates.split(/[,，\s]+/).filter(Boolean),
        rules: draft.rules,
    } : null;
}
