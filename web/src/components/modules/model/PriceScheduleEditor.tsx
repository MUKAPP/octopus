import { useEffect, useId, useRef, useState } from 'react';
import { Clock, Plus, Trash2 } from 'lucide-react';
import { useTranslations } from 'use-intl';
import type { LLMPrice, LLMTimePrice } from '@/api/model';
import type { PriceScheduleDraft } from './price-schedule';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { cn } from '@/lib/utils';

const priceFields = [
    ['input', 'input'], ['output', 'output'], ['cache_read', 'cacheRead'], ['cache_write', 'cacheWrite'],
] as const;

const HOURS_START = Array.from({ length: 24 }, (_, i) => i);
const HOURS_END = Array.from({ length: 25 }, (_, i) => i);
const MINUTES = Array.from({ length: 60 }, (_, i) => i);
const START_PRESETS = ['00:00', '08:00', '09:00', '12:00', '18:00'] as const;
const END_PRESETS = ['09:00', '12:00', '18:00', '22:00', '24:00'] as const;

const START_PATTERN = '([01][0-9]|2[0-3]):[0-5][0-9]';
const END_PATTERN = '([01][0-9]|2[0-3]):[0-5][0-9]|24:00';

function padTimeUnit(n: number): string {
    return String(n).padStart(2, '0');
}

function parseScheduleTime(value: string, allowEndOfDay: boolean): { hour: number; minute: number } | null {
    const trimmed = value.trim();
    if (allowEndOfDay && trimmed === '24:00') {
        return { hour: 24, minute: 0 };
    }
    const match = /^([01]?\d|2[0-3]):([0-5]\d)$/.exec(trimmed);
    if (!match) return null;
    return { hour: Number(match[1]), minute: Number(match[2]) };
}

function formatScheduleTime(hour: number, minute: number): string {
    if (hour === 24) return '24:00';
    return `${padTimeUnit(hour)}:${padTimeUnit(minute)}`;
}

function ScheduleTimeField({
    label,
    value,
    allowEndOfDay = false,
    onChange,
}: {
    label: string;
    value: string;
    allowEndOfDay?: boolean;
    onChange: (value: string) => void;
}) {
    const t = useTranslations('model.schedule');
    const inputId = useId();
    const [open, setOpen] = useState(false);
    const [touched, setTouched] = useState(false);
    const hourListRef = useRef<HTMLDivElement | null>(null);
    const minuteListRef = useRef<HTMLDivElement | null>(null);
    const activeHourButtonRef = useRef<HTMLButtonElement | null>(null);
    const activeMinuteButtonRef = useRef<HTMLButtonElement | null>(null);

    const parsed = parseScheduleTime(value, allowEndOfDay);
    const hours = allowEndOfDay ? HOURS_END : HOURS_START;
    const presets = allowEndOfDay ? END_PRESETS : START_PRESETS;
    const isExactValid = parseScheduleTime(value, allowEndOfDay) !== null &&
        (!value.trim().includes(':') || /^(?:[01]\d|2[0-3]):[0-5]\d$|^24:00$/.test(value.trim())) &&
        (allowEndOfDay || value.trim() !== '24:00');
    const isInvalid = touched && !isExactValid;

    useEffect(() => {
        if (!open) return;
        const frame = requestAnimationFrame(() => {
            const scrollToCenter = (list: HTMLDivElement | null, target: HTMLButtonElement | null) => {
                if (!list || !target) return;
                list.scrollTop = Math.max(0, target.offsetTop - (list.clientHeight - target.offsetHeight) / 2);
            };
            scrollToCenter(hourListRef.current, activeHourButtonRef.current);
            scrollToCenter(minuteListRef.current, activeMinuteButtonRef.current);
        });
        return () => cancelAnimationFrame(frame);
    }, [open]);

    const handleBlur = () => {
        setTouched(true);
        const normalized = parsed ? formatScheduleTime(parsed.hour, parsed.minute) : value.trim();
        if (normalized !== value) {
            onChange(normalized);
        }
    };

    const handleSelectHour = (hour: number) => {
        setTouched(false);
        if (hour === 24) {
            onChange('24:00');
            return;
        }
        const nextMinute = parsed && parsed.hour !== 24 ? parsed.minute : 0;
        onChange(formatScheduleTime(hour, nextMinute));
    };

    const handleSelectMinute = (minute: number) => {
        if (parsed?.hour === 24 && minute !== 0) return;
        setTouched(false);
        const nextHour = parsed && parsed.hour !== 24 ? parsed.hour : 0;
        onChange(formatScheduleTime(nextHour, minute));
    };

    return (
        <div className="grid min-w-0 gap-1">
            <label htmlFor={inputId} className="text-xs text-muted-foreground">
                {label}
            </label>
            <div className="relative flex min-w-0 items-center">
                <Input
                    id={inputId}
                    type="text"
                    inputMode="numeric"
                    autoComplete="off"
                    required
                    pattern={allowEndOfDay ? END_PATTERN : START_PATTERN}
                    placeholder={allowEndOfDay ? '24:00' : '09:00'}
                    aria-invalid={isInvalid || undefined}
                    value={value}
                    onChange={(event) => {
                        setTouched(false);
                        onChange(event.target.value);
                    }}
                    onBlur={handleBlur}
                    className="h-9 min-w-0 rounded-xl pr-9 tabular-nums text-base sm:text-sm"
                />
                <Popover open={open} onOpenChange={setOpen}>
                    <PopoverTrigger asChild>
                        <button
                            type="button"
                            aria-label={t('pickTime', { label })}
                            className="focus-visible:outline-2 focus-visible:outline-ring absolute right-1 inline-flex size-7 items-center justify-center rounded-lg text-muted-foreground transition-colors motion-reduce:transition-none hover:bg-muted hover:text-foreground"
                        >
                            <Clock className="size-4" aria-hidden="true" />
                        </button>
                    </PopoverTrigger>
                    <PopoverContent
                        align="end"
                        side="bottom"
                        sideOffset={6}
                        className="z-[70] w-64 rounded-2xl border border-border/60 bg-card p-3 shadow-xl"
                    >
                        <div className="flex items-center justify-between gap-2 pb-2">
                            <div className="flex items-baseline gap-2 min-w-0">
                                <span className="text-xs font-medium text-muted-foreground">{label}</span>
                                <span className="text-xs font-semibold tabular-nums text-foreground">
                                    {parsed ? formatScheduleTime(parsed.hour, parsed.minute) : (allowEndOfDay ? '00:00–24:00' : '00:00–23:59')}
                                </span>
                            </div>
                            <Button
                                type="button"
                                size="sm"
                                variant="secondary"
                                className="h-6 rounded-lg px-2 text-xs"
                                onClick={() => setOpen(false)}
                            >
                                {t('done')}
                            </Button>
                        </div>

                        <div className="flex flex-wrap gap-1 border-y border-border/60 py-2">
                            {presets.map((preset) => {
                                const isActive = parsed !== null && formatScheduleTime(parsed.hour, parsed.minute) === preset;
                                return (
                                    <button
                                        key={preset}
                                        type="button"
                                        onClick={() => {
                                            setTouched(false);
                                            onChange(preset);
                                            setOpen(false);
                                        }}
                                        className={cn(
                                            'rounded-lg border px-2 py-1 text-xs tabular-nums transition-colors motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-ring',
                                            isActive
                                                ? 'border-primary/30 bg-primary/10 font-medium text-primary'
                                                : 'border-border/70 bg-muted/30 text-muted-foreground hover:bg-muted hover:text-foreground'
                                        )}
                                    >
                                        {preset}
                                    </button>
                                );
                            })}
                        </div>

                        <div className="grid grid-cols-2 gap-2 pt-2">
                            <div className="min-w-0">
                                <p className="px-1.5 pb-1 text-[11px] font-medium text-muted-foreground">{t('hour')}</p>
                                <div ref={hourListRef} className="scrollbar relative h-44 space-y-0.5 overflow-y-auto px-0.5">
                                    {hours.map((hour) => {
                                        const selected = parsed?.hour === hour;
                                        return (
                                            <button
                                                key={hour}
                                                ref={selected ? activeHourButtonRef : undefined}
                                                type="button"
                                                onClick={() => handleSelectHour(hour)}
                                                className={cn(
                                                    'flex h-7 w-full items-center justify-center rounded-lg text-xs tabular-nums transition-colors motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-ring',
                                                    selected
                                                        ? 'bg-primary text-primary-foreground font-medium'
                                                        : 'text-foreground hover:bg-muted'
                                                )}
                                            >
                                                {padTimeUnit(hour)}
                                            </button>
                                        );
                                    })}
                                </div>
                            </div>
                            <div className="min-w-0">
                                <p className="px-1.5 pb-1 text-[11px] font-medium text-muted-foreground">{t('minute')}</p>
                                <div ref={minuteListRef} className="scrollbar relative h-44 space-y-0.5 overflow-y-auto px-0.5">
                                    {MINUTES.map((minute) => {
                                        const selected = parsed?.minute === minute;
                                        const disabled = parsed?.hour === 24 && minute !== 0;
                                        return (
                                            <button
                                                key={minute}
                                                ref={selected ? activeMinuteButtonRef : undefined}
                                                type="button"
                                                disabled={disabled}
                                                onClick={() => handleSelectMinute(minute)}
                                                className={cn(
                                                    'flex h-7 w-full items-center justify-center rounded-lg text-xs tabular-nums transition-colors motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-ring',
                                                    selected
                                                        ? 'bg-primary text-primary-foreground font-medium'
                                                        : 'text-foreground hover:bg-muted',
                                                    disabled && 'cursor-not-allowed opacity-35 hover:bg-transparent'
                                                )}
                                            >
                                                {padTimeUnit(minute)}
                                            </button>
                                        );
                                    })}
                                </div>
                            </div>
                        </div>
                    </PopoverContent>
                </Popover>
            </div>
        </div>
    );
}

interface Props {
    value: PriceScheduleDraft | null;
    basePrice: LLMPrice;
    onChange: (value: PriceScheduleDraft | null) => void;
}

export function PriceScheduleEditor({ value, basePrice, onChange }: Props) {
    const t = useTranslations('model.schedule');
    const id = useId();
    const timezoneId = useId();
    const excludedDatesId = useId();
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
                <Button type="button" size="sm" variant="secondary" className="rounded-xl" onClick={addRule}>
                    <Plus className="size-3.5" />{t('add')}
                </Button>
            </div>
            <p className="text-xs leading-relaxed text-muted-foreground">{t('hint')}</p>
            {value && <>
                <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
                    <div className="grid min-w-0 gap-1">
                        <label htmlFor={timezoneId} className="text-xs text-muted-foreground">
                            {t('timezone')}
                        </label>
                        <Input
                            id={timezoneId}
                            value={value.timezone}
                            onChange={event => onChange({ ...value, timezone: event.target.value })}
                            placeholder="Asia/Shanghai"
                            required
                            className="h-9 rounded-xl text-base sm:text-sm"
                        />
                    </div>
                    <div className="grid min-w-0 gap-1">
                        <label htmlFor={excludedDatesId} className="text-xs text-muted-foreground">
                            {t('excludedDates')}
                        </label>
                        <Input
                            id={excludedDatesId}
                            value={value.excludedDates}
                            onChange={event => onChange({ ...value, excludedDates: event.target.value })}
                            placeholder={t('datesPlaceholder')}
                            className="h-9 rounded-xl text-base sm:text-sm"
                        />
                    </div>
                    <p className="sm:col-span-2 text-xs leading-relaxed text-muted-foreground">{t('datesHint')}</p>
                </div>
                {value.rules.map((rule, index) => <fieldset key={index} className="min-w-0 space-y-3 rounded-2xl border border-border/80 bg-muted/20 p-3.5">
                    <legend className="px-1.5 text-xs font-medium text-muted-foreground">{t('rule', { number: index + 1 })}</legend>
                    <div className="flex items-center gap-2">
                        <Input aria-label={t('name')} placeholder={t('name')} value={rule.name}
                            onChange={event => updateRule(index, { name: event.target.value })} className="h-9 min-w-0 rounded-xl text-base sm:text-sm" />
                        <Button type="button" size="icon" variant="ghost" aria-label={t('remove', { number: index + 1 })}
                            className="shrink-0 rounded-xl text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                            onClick={() => {
                                const rules = value.rules.filter((_, i) => i !== index);
                                onChange(rules.length ? { ...value, rules } : null);
                            }}><Trash2 className="size-4" /></Button>
                    </div>
                    <div className="grid min-w-0 grid-cols-1 gap-2.5 min-[360px]:grid-cols-2">
                        <ScheduleTimeField
                            label={t('start')}
                            value={rule.start}
                            onChange={(start) => updateRule(index, { start })}
                        />
                        <ScheduleTimeField
                            label={t('end')}
                            allowEndOfDay
                            value={rule.end}
                            onChange={(end) => updateRule(index, { end })}
                        />
                    </div>
                    <div className="space-y-1.5">
                        <p className="text-xs text-muted-foreground">{t('weekdays')}</p>
                        <div className="flex flex-wrap gap-1" role="group" aria-label={t('weekdays')}>
                            {[1, 2, 3, 4, 5, 6, 0].map(day => {
                                const selected = !rule.weekdays?.length || rule.weekdays.includes(day);
                                return <button key={day} type="button" aria-pressed={selected}
                                    className={'rounded-lg border px-2.5 py-1.5 text-xs transition-colors motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-ring ' + (selected ? 'border-primary/30 bg-primary/10 text-primary hover:bg-primary/15' : 'border-border text-muted-foreground hover:bg-muted/60 hover:text-foreground')}
                                    onClick={() => {
                                        const days = rule.weekdays?.length ? rule.weekdays : [0, 1, 2, 3, 4, 5, 6];
                                        if (selected && days.length === 1) return;
                                        updateRule(index, { weekdays: selected ? days.filter(d => d !== day) : [...days, day] });
                                    }}>{t('day' + day)}</button>;
                            })}
                        </div>
                    </div>
                    <div className="grid min-w-0 grid-cols-2 gap-2.5 sm:grid-cols-4">
                        {priceFields.map(([field, key]) => <label key={field} className="grid min-w-0 gap-1 text-xs text-muted-foreground">
                            {t(key)}
                            <Input type="number" min="0" step="any" required value={rule[field]}
                                onChange={event => updateRule(index, { [field]: event.target.value === '' ? 0 : Number(event.target.value) })}
                                className="h-9 min-w-0 rounded-xl tabular-nums text-base sm:text-sm" />
                        </label>)}
                    </div>
                </fieldset>)}
            </>}
        </section>
    );
}
