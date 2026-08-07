import { motion, useReducedMotion } from 'motion/react';
import { MessageSquare, Bot, DollarSign, Gauge, Loader2 } from 'lucide-react';
import { useTranslations } from 'use-intl';
import type { UseQueryResult } from '@tanstack/react-query';
import { AnimatedNumber } from '@/components/common/AnimatedNumber';
import { EASING, REDUCED_MOTION_TRANSITION } from '@/lib/animations/fluid-transitions';
import type { AnalyticsResponseFormatted } from '@/api/endpoints/analytics';

interface SummaryCardsProps {
    query: UseQueryResult<AnalyticsResponseFormatted>;
}

export function SummaryCards({ query }: SummaryCardsProps) {
    const t = useTranslations('analytics.summary');
    const tFeedback = useTranslations('analytics.feedback');
    const shouldReduceMotion = useReducedMotion() ?? false;

    if (query.isLoading && !query.data) {
        return (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                <div className="col-span-full flex min-h-40 items-center justify-center rounded-3xl border border-card-border bg-card text-muted-foreground">
                    <Loader2 className="size-8 animate-spin" role="status" aria-label={tFeedback('loading')} />
                </div>
            </div>
        );
    }

    if (query.isError && !query.data) {
        return (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                <div role="alert" className="col-span-full flex min-h-40 flex-col items-center justify-center gap-3 rounded-3xl border border-destructive/30 bg-card p-4 text-center text-sm text-muted-foreground">
                    <p>{tFeedback('loadFailed')}</p>
                    <button type="button" onClick={() => void query.refetch()} className="rounded-xl border border-border px-3 py-1.5 font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
                        {tFeedback('retry')}
                    </button>
                </div>
            </div>
        );
    }

    const summary = query.data?.summary;

    const cards = [
        {
            title: t('requests'),
            icon: MessageSquare,
            color: 'text-primary',
            bgColor: 'bg-primary/10',
            value: summary?.request_count.formatted.value,
            unit: summary?.request_count.formatted.unit,
        },
        {
            title: t('totalTokens'),
            icon: Bot,
            color: 'text-primary',
            bgColor: 'bg-chart-1/10',
            value: summary?.total_token.formatted.value,
            unit: summary?.total_token.formatted.unit,
        },
        {
            title: t('totalCost'),
            icon: DollarSign,
            color: 'text-primary',
            bgColor: 'bg-chart-2/10',
            value: summary?.total_cost.formatted.value,
            unit: summary?.total_cost.formatted.unit,
        },
        {
            title: t('successRate'),
            icon: Gauge,
            color: 'text-primary',
            bgColor: 'bg-chart-3/10',
            value: summary?.success_rate.formatted,
            unit: undefined,
        },
    ];

    return (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            {query.isError && (
                <div role="alert" className="col-span-full flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-muted-foreground">
                    <span>{tFeedback('loadFailed')}</span>
                    <button type="button" onClick={() => void query.refetch()} className="shrink-0 rounded-xl border border-border px-3 py-1.5 font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
                        {tFeedback('retry')}
                    </button>
                </div>
            )}
            {cards.map((card, index) => (
                <motion.section
                    key={card.title}
                    className="rounded-3xl bg-card border-card-border border p-5 text-card-foreground flex flex-row items-center gap-4"
                    initial={shouldReduceMotion ? { opacity: 0 } : { opacity: 0, y: 20, filter: 'blur(8px)' }}
                    animate={shouldReduceMotion ? { opacity: 1 } : { opacity: 1, y: 0, filter: 'blur(0px)' }}
                    transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : {
                        duration: 0.5,
                        ease: EASING.easeOutExpo,
                        delay: index * 0.08,
                    }}
                >
                    <div className={`w-10 h-10 rounded-xl flex items-center justify-center shrink-0 ${card.bgColor} ${card.color}`}>
                        <card.icon className="w-5 h-5" />
                    </div>
                    <div className="flex flex-col min-w-0">
                        <span className="text-xs text-muted-foreground">{card.title}</span>
                        <div className="flex items-baseline gap-1">
                            <span className="text-xl">
                                {card.value === undefined ? (
                                    '-'
                                ) : card.unit === undefined ? (
                                    card.value
                                ) : (
                                    <AnimatedNumber value={card.value} />
                                )}
                            </span>
                            {card.unit && (
                                <span className="text-sm text-muted-foreground">{card.unit}</span>
                            )}
                        </div>
                    </div>
                </motion.section>
            ))}
        </div>
    );
}
