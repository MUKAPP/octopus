import { useEffect, useState } from 'react';
import { Activity, ArrowRight, Loader2 } from 'lucide-react';
import { useLocale, useTranslations } from 'use-intl';
import type { LogStreamStatus, RelayLog } from '@/api/endpoints/log';
import { formatDuration } from '@/components/modules/log/format';
import { useLiveLogDuration } from '@/components/modules/log/use-live-duration';
import { useNavStore } from '@/components/modules/navbar';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { formatHomeMoney } from './home-ops';

interface LiveActivityCardProps {
    activeLogs: RelayLog[];
    recentLogs: RelayLog[];
    status: LogStreamStatus;
}

function ActiveActivityRow({ log, onOpenLogs }: { log: RelayLog; onOpenLogs: () => void }) {
    const statusT = useTranslations('log.status');
    const liveDuration = useLiveLogDuration(log);
    const stateLabel = log.state === 'committed' ? statusT('committed') : statusT('running');

    return (
        <button
            type="button"
            onClick={onOpenLogs}
            className="flex w-full items-center gap-2.5 rounded-2xl border border-primary/25 bg-primary/8 px-3 py-2.5 text-left transition-colors motion-reduce:transition-none hover:bg-primary/12 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
            <Badge variant="outline" className="h-5 shrink-0 gap-1.5 border-primary/30 bg-card/80 px-2 text-[11px] font-medium text-primary">
                <span className="size-1.5 rounded-full bg-primary animate-pulse motion-reduce:animate-none" aria-hidden="true" />
                {stateLabel}
            </Badge>

            <div className="min-w-0 flex-1 truncate text-sm font-medium text-card-foreground" title={`${log.request_model_name} → ${log.channel_name}`}>
                <span>{log.request_model_name}</span>
                {log.channel_name && log.channel_name !== '—' && (
                    <span className="text-muted-foreground font-normal"> → {log.channel_name}</span>
                )}
            </div>

            <span className="shrink-0 text-xs font-medium tabular-nums text-primary">
                {formatDuration(Math.round(Math.max(0, liveDuration)))}
            </span>
        </button>
    );
}

function formatRelativeTimestamp(timestampSec: number, nowSec: number, locale: string, t: ReturnType<typeof useTranslations<'home.liveActivity'>>): string {
    if (!Number.isFinite(timestampSec) || timestampSec <= 0) return '—';
    const diff = Math.max(0, Math.floor(nowSec - timestampSec));
    if (diff < 10) return t('justNow');
    if (diff < 60) return t('secondsAgo', { count: diff });
    if (diff < 3600) return t('minutesAgo', { count: Math.floor(diff / 60) });
    if (diff < 86400) return t('hoursAgo', { count: Math.floor(diff / 3600) });
    return new Date(timestampSec * 1000).toLocaleTimeString(locale, {
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
    });
}

function RecentActivityRow({
    log,
    nowSec,
    onOpenLogs,
}: {
    log: RelayLog;
    nowSec: number;
    onOpenLogs: () => void;
}) {
    const t = useTranslations('home.liveActivity');
    const statusT = useTranslations('log.status');
    const locale = useLocale();

    const state = log.state ?? (log.error ? 'failed' : 'success');
    const upstreamCode = log.upstream_status_code;
    const hasUpstreamCode = upstreamCode !== undefined && upstreamCode > 0;

    let badgeLabel: string;
    let badgeClass: string;

    if (state === 'success') {
        badgeLabel = hasUpstreamCode ? String(upstreamCode) : statusT('success');
        badgeClass = 'border-emerald-500/30 bg-emerald-500/12 text-emerald-700 dark:text-emerald-400';
    } else if (state === 'canceled') {
        badgeLabel = hasUpstreamCode && upstreamCode !== 200 ? String(upstreamCode) : statusT('canceled');
        badgeClass = 'border-amber-500/30 bg-amber-500/12 text-amber-700 dark:text-amber-300';
    } else {
        badgeLabel =
            hasUpstreamCode && upstreamCode !== 200
                ? String(upstreamCode)
                : hasUpstreamCode
                    ? `${statusT('failed')} · ${upstreamCode}`
                    : statusT('failed');
        badgeClass = 'border-destructive/30 bg-destructive/12 text-destructive';
    }

    const costDisplay = log.cost > 0 ? formatHomeMoney(log.cost).text : null;
    const timeText = formatRelativeTimestamp(log.time, nowSec, locale, t);

    return (
        <button
            type="button"
            onClick={onOpenLogs}
            className="flex w-full items-center gap-2.5 rounded-2xl px-3 py-2 text-left transition-colors motion-reduce:transition-none hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
            title={log.error || `${log.request_model_name} → ${log.channel_name}`}
        >
            <Badge
                variant="outline"
                className={cn('h-5 min-w-11 justify-center shrink-0 px-1.5 text-[11px] font-semibold tabular-nums', badgeClass)}
            >
                {badgeLabel}
            </Badge>

            <div className="min-w-0 flex-1 truncate text-sm text-card-foreground">
                <span className="font-medium">{log.request_model_name}</span>
                {log.channel_name && log.channel_name !== '—' && (
                    <span className="text-muted-foreground"> — {log.channel_name}</span>
                )}
            </div>

            <div className="flex shrink-0 items-center gap-1.5 text-xs tabular-nums text-muted-foreground">
                <span>{formatDuration(Math.round(Math.max(0, log.use_time)))}</span>
                {costDisplay && (
                    <>
                        <span className="text-border">·</span>
                        <span>{costDisplay}</span>
                    </>
                )}
                <span className="hidden sm:inline text-border">·</span>
                <span className="hidden sm:inline">{timeText}</span>
            </div>
        </button>
    );
}

export function LiveActivityCard({ activeLogs, recentLogs, status }: LiveActivityCardProps) {
    const t = useTranslations('home.liveActivity');
    const { setActiveItem } = useNavStore();
    const [nowSec, setNowSec] = useState(() => Math.floor(Date.now() / 1000));

    useEffect(() => {
        if (recentLogs.length === 0) return;
        const timer = window.setInterval(() => {
            setNowSec(Math.floor(Date.now() / 1000));
        }, 15_000);
        return () => window.clearInterval(timer);
    }, [recentLogs.length]);

    const visibleRecentLogs = recentLogs.slice(0, 8);
    const hasItems = activeLogs.length > 0 || visibleRecentLogs.length > 0;

    return (
        <section className="flex min-w-0 flex-col rounded-3xl border border-border bg-card p-4 sm:p-5 text-card-foreground">
            <div className="mb-3 flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 min-w-0">
                    <h2 className="text-base font-semibold truncate">{t('title')}</h2>
                    {activeLogs.length > 0 && (
                        <Badge variant="outline" className="h-5 border-primary/25 bg-primary/10 px-2 text-[11px] font-medium text-primary tabular-nums">
                            {t('activeCount', { count: activeLogs.length })}
                        </Badge>
                    )}
                </div>

                <button
                    type="button"
                    onClick={() => setActiveItem('log')}
                    className="inline-flex shrink-0 items-center gap-1 rounded-lg px-2 py-1 text-xs font-medium text-muted-foreground transition-colors motion-reduce:transition-none hover:bg-muted/50 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                >
                    <span>{t('allLogs')}</span>
                    <ArrowRight className="size-3.5" aria-hidden="true" />
                </button>
            </div>

            {!hasItems ? (
                status === 'connecting' ? (
                    <div className="flex flex-1 min-h-48 items-center justify-center gap-2 text-xs text-muted-foreground">
                        <Loader2 className="size-5 animate-spin motion-reduce:animate-none" role="status" aria-label={t('connecting')} />
                        <span>{t('connecting')}</span>
                    </div>
                ) : (
                    <div className="flex flex-1 min-h-48 flex-col items-center justify-center gap-2 text-center text-muted-foreground">
                        <Activity className="size-8 opacity-35" aria-hidden="true" />
                        <p className="text-sm font-medium text-card-foreground/80">{t('empty')}</p>
                        <p className="text-xs">{t('emptyHint')}</p>
                    </div>
                )
            ) : (
                <div className="scrollbar flex-1 max-h-[304px] min-h-[160px] space-y-1.5 overflow-y-auto">
                    {activeLogs.map((log) => (
                        <ActiveActivityRow
                            key={`active-${log.id}`}
                            log={log}
                            onOpenLogs={() => setActiveItem('log')}
                        />
                    ))}
                    {visibleRecentLogs.map((log) => (
                        <RecentActivityRow
                            key={`recent-${log.id}`}
                            log={log}
                            nowSec={nowSec}
                            onOpenLogs={() => setActiveItem('log')}
                        />
                    ))}
                </div>
            )}
        </section>
    );
}
