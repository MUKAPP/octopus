import { motion, useReducedMotion } from 'motion/react';
import {
    Activity,
    MessageSquare,
    Clock,
    ArrowDownToLine,
    ChartColumnBig,
    Bot,
    ArrowUpFromLine,
    Rewind,
    DollarSign,
    FastForward,
    Loader2,
} from 'lucide-react';
import { useTranslations } from 'use-intl';
import { useStatsTotal } from '@/api/stats';
import { AnimatedNumber } from '@/components/common/AnimatedNumber';
import { EASING, REDUCED_MOTION_TRANSITION } from '@/lib/animations/fluid-transitions';


export function Total() {
    const { data: statsTotalFormatted, isLoading, isError, refetch } = useStatsTotal();
    const t = useTranslations('home.total');
    const tHome = useTranslations('home');
    const shouldReduceMotion = useReducedMotion() ?? false;

    if (isLoading && !statsTotalFormatted) {
        return (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                <div className="col-span-full flex min-h-40 items-center justify-center rounded-3xl border border-card-border bg-card text-muted-foreground">
                    <Loader2 className="size-8 animate-spin" role="status" aria-label={tHome('feedback.loading')} />
                </div>
            </div>
        );
    }

    if (isError && !statsTotalFormatted) {
        return (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                <div role="alert" className="col-span-full flex min-h-40 flex-col items-center justify-center gap-3 rounded-3xl border border-destructive/30 bg-card p-4 text-center text-sm text-muted-foreground">
                    <p>{tHome('feedback.loadFailed')}</p>
                    <button type="button" onClick={() => void refetch()} className="rounded-xl border border-border px-3 py-1.5 font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
                        {tHome('feedback.retry')}
                    </button>
                </div>
            </div>
        );
    }

    const cards = [
        {
            title: t('requestStats'),
            headerIcon: Activity,
            items: [
                {
                    label: t('requestCount'),
                    value: statsTotalFormatted?.request_count.formatted.value,
                    icon: MessageSquare,
                    color: 'text-primary',
                    bgColor: 'bg-primary/10',
                    unit: statsTotalFormatted?.request_count.formatted.unit
                },
                {
                    label: t('timeConsumed'),
                    value: statsTotalFormatted?.wait_time.formatted.value,
                    icon: Clock,
                    color: 'text-primary',
                    bgColor: 'bg-accent/10',
                    unit: statsTotalFormatted?.wait_time.formatted.unit
                }
            ]
        },
        {
            title: t('totalStats'),
            headerIcon: ChartColumnBig,
            items: [
                {
                    label: t('totalToken'),
                    value: statsTotalFormatted?.total_token.formatted.value,
                    icon: Bot,
                    color: 'text-primary',
                    bgColor: 'bg-chart-1/10',
                    unit: statsTotalFormatted?.total_token.formatted.unit
                },
                {
                    label: t('totalCost'),
                    value: statsTotalFormatted?.total_cost.formatted.value,
                    icon: DollarSign,
                    color: 'text-primary',
                    bgColor: 'bg-chart-2/10',
                    unit: statsTotalFormatted?.total_cost.formatted.unit
                }
            ]
        },
        {
            title: t('inputStats'),
            headerIcon: ArrowDownToLine,
            items: [
                {
                    label: t('inputTokens'),
                    value: statsTotalFormatted?.input_token.formatted.value,
                    icon: Rewind,
                    color: 'text-primary',
                    bgColor: 'bg-chart-3/10',
                    unit: statsTotalFormatted?.input_token.formatted.unit
                },
                {
                    label: t('inputCost'),
                    value: statsTotalFormatted?.input_cost.formatted.value,
                    icon: DollarSign,
                    color: 'text-primary',
                    bgColor: 'bg-chart-3/10',
                    unit: statsTotalFormatted?.input_cost.formatted.unit
                }
            ]
        },
        {
            title: t('outputStats'),
            headerIcon: ArrowUpFromLine,
            items: [
                {
                    label: t('outputTokens'),
                    value: statsTotalFormatted?.output_token.formatted.value,
                    icon: FastForward,
                    color: 'text-primary',
                    bgColor: 'bg-chart-4/10',
                    unit: statsTotalFormatted?.output_token.formatted.unit
                },
                {
                    label: t('outputCost'),
                    value: statsTotalFormatted?.output_cost.formatted.value,
                    icon: DollarSign,
                    color: 'text-primary',
                    bgColor: 'bg-chart-4/10',
                    unit: statsTotalFormatted?.output_cost.formatted.unit
                }
            ]
        }
    ];

    return (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            {isError && (
                <div role="alert" className="col-span-full flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-muted-foreground">
                    <span>{tHome('feedback.loadFailed')}</span>
                    <button type="button" onClick={() => void refetch()} className="shrink-0 rounded-xl border border-border px-3 py-1.5 font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
                        {tHome('feedback.retry')}
                    </button>
                </div>
            )}
            {cards.map((card, index) => (
                <motion.section
                    key={index}
                    className="rounded-3xl bg-card border-card-border border p-5 text-card-foreground flex flex-row items-center gap-4"
                    initial={shouldReduceMotion ? { opacity: 0 } : { opacity: 0, y: 20, filter: 'blur(8px)' }}
                    animate={shouldReduceMotion ? { opacity: 1 } : { opacity: 1, y: 0, filter: 'blur(0px)' }}
                    transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : {
                        duration: 0.5,
                        ease: EASING.easeOutExpo,
                        delay: index * 0.08,
                    }}
                >
                    <div className="flex flex-col items-center justify-center gap-3 border-r border-border/50 pr-4 py-1 self-stretch">
                        <card.headerIcon className="w-4 h-4" />
                        <h3 className="font-medium text-sm [writing-mode:vertical-lr]">{card.title}</h3>
                    </div>

                    <div className="flex flex-col gap-4 flex-1 min-w-0">
                        {card.items.map((item, idx) => (
                            <div key={idx} className="flex items-center gap-3">
                                <div className={`w-10 h-10 rounded-xl flex items-center justify-center shrink-0 ${item.bgColor} ${item.color}`}>
                                    <item.icon className="w-5 h-5" />
                                </div>
                                <div className="flex flex-col min-w-0">
                                    <span className="text-xs text-muted-foreground">{item.label}</span>
                                    <div className="flex items-baseline gap-1">
                                        <span className="text-xl">
                                            <AnimatedNumber value={item.value} />
                                        </span>
                                        {item.unit && (
                                            <span className="text-sm text-muted-foreground">{item.unit}</span>
                                        )}
                                    </div>
                                </div>
                            </div>
                        ))}
                    </div>
                </motion.section>
            ))}
        </div>
    );
}
