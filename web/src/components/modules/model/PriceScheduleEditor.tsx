import { useId } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { useTranslations } from 'use-intl';
import type { LLMPrice, LLMTimePrice } from '@/api/model';
import type { PriceScheduleDraft } from './price-schedule';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';

const priceFields = [
    ['input', 'input'], ['output', 'output'], ['cache_read', 'cacheRead'], ['cache_write', 'cacheWrite'],
] as const;

interface Props {
    value: PriceScheduleDraft | null;
    basePrice: LLMPrice;
    onChange: (value: PriceScheduleDraft | null) => void;
}

export function PriceScheduleEditor({ value, basePrice, onChange }: Props) {
    const t = useTranslations('model.schedule');
    const id = useId();
    const addRule = () => {
        const rule: LLMTimePrice = {
            name: '', start: '09:00', end: '12:00', weekdays: [],
            input: basePrice.input, output: basePrice.output,
            cache_read: basePrice.cache_read, cache_write: basePrice.cache_write,
        };
        onChange(value ? { ...value, rules: [...value.rules, rule] } : {
            timezone: 'Asia/Shanghai', excludedDates: '', rules: [rule],
        });
    };
    const updateRule = (index: number, patch: Partial<LLMTimePrice>) => {
        if (value) onChange({ ...value, rules: value.rules.map((rule, i) => i === index ? { ...rule, ...patch } : rule) });
    };

    return (
        <section className="min-w-0 space-y-3 border-t border-border pt-4" aria-labelledby={id}>
            <div className="flex flex-wrap items-center justify-between gap-2">
                <h4 id={id} className="text-sm font-semibold">{t('title')}</h4>
                <Button type="button" size="sm" variant="secondary" onClick={addRule}>
                    <Plus className="size-3.5" />{t('add')}
                </Button>
            </div>
            <p className="text-xs leading-relaxed text-muted-foreground">{t('hint')}</p>
            {value && <>
                <label className="grid min-w-0 gap-1 text-xs text-muted-foreground">
                    {t('timezone')}
                    <Input value={value.timezone} onChange={event => onChange({ ...value, timezone: event.target.value })}
                        placeholder="Asia/Shanghai" required className="h-9 rounded-xl text-base sm:text-sm" />
                </label>
                <label className="grid min-w-0 gap-1 text-xs text-muted-foreground">
                    {t('excludedDates')}
                    <Input value={value.excludedDates} onChange={event => onChange({ ...value, excludedDates: event.target.value })}
                        placeholder={t('datesPlaceholder')} className="h-9 rounded-xl text-base sm:text-sm" />
                    <span className="leading-relaxed">{t('datesHint')}</span>
                </label>
                {value.rules.map((rule, index) => <fieldset key={index} className="min-w-0 space-y-3 rounded-2xl border border-border p-3">
                    <legend className="px-1 text-xs font-medium">{t('rule', { number: index + 1 })}</legend>
                    <div className="flex items-center gap-2">
                        <Input aria-label={t('name')} placeholder={t('name')} value={rule.name}
                            onChange={event => updateRule(index, { name: event.target.value })} className="h-9 min-w-0 rounded-xl text-base sm:text-sm" />
                        <Button type="button" size="icon" variant="ghost" aria-label={t('remove', { number: index + 1 })}
                            onClick={() => {
                                const rules = value.rules.filter((_, i) => i !== index);
                                onChange(rules.length ? { ...value, rules } : null);
                            }}><Trash2 className="size-4" /></Button>
                    </div>
                    <div className="grid min-w-0 grid-cols-2 gap-2">
                        <label className="grid min-w-0 gap-1 text-xs text-muted-foreground">{t('start')}
                            <Input type="time" required value={rule.start} onChange={event => updateRule(index, { start: event.target.value })}
                                className="h-9 min-w-0 rounded-xl text-base sm:text-sm" />
                        </label>
                        <label className="grid min-w-0 gap-1 text-xs text-muted-foreground">{t('end')}
                            <Input type="text" required inputMode="numeric" pattern="([01][0-9]|2[0-3]):[0-5][0-9]|24:00" value={rule.end}
                                placeholder="24:00" onChange={event => updateRule(index, { end: event.target.value })}
                                className="h-9 min-w-0 rounded-xl text-base sm:text-sm" />
                        </label>
                    </div>
                    <div className="space-y-1">
                        <p className="text-xs text-muted-foreground">{t('weekdays')}</p>
                        <div className="flex flex-wrap gap-1" role="group" aria-label={t('weekdays')}>
                            {[1, 2, 3, 4, 5, 6, 0].map(day => {
                                const selected = !rule.weekdays?.length || rule.weekdays.includes(day);
                                return <button key={day} type="button" aria-pressed={selected}
                                    className={'rounded-lg border px-2 py-1.5 text-xs transition-colors motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-ring ' + (selected ? 'border-primary/30 bg-primary/10 text-primary' : 'border-border text-muted-foreground')}
                                    onClick={() => {
                                        const days = rule.weekdays?.length ? rule.weekdays : [0, 1, 2, 3, 4, 5, 6];
                                        if (selected && days.length === 1) return;
                                        updateRule(index, { weekdays: selected ? days.filter(d => d !== day) : [...days, day] });
                                    }}>{t('day' + day)}</button>;
                            })}
                        </div>
                    </div>
                    <div className="grid min-w-0 grid-cols-2 gap-2">
                        {priceFields.map(([field, key]) => <label key={field} className="grid min-w-0 gap-1 text-xs text-muted-foreground">
                            {t(key)}
                            <Input type="number" min="0" step="any" required value={rule[field]}
                                onChange={event => updateRule(index, { [field]: event.target.value === '' ? 0 : Number(event.target.value) })}
                                className="h-9 min-w-0 rounded-xl text-base sm:text-sm" />
                        </label>)}
                    </div>
                </fieldset>)}
            </>}
        </section>
    );
}
