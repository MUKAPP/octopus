import { useEffect, useMemo, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { ArrowRight, Clock, Loader2 } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { type ActiveRelayRequest } from '@/api/endpoints/log';
import { getModelIcon } from '@/lib/model-icons';
import { Badge } from '@/components/ui/badge';
import { formatDuration } from './format';

/**
 * 进行中请求的已耗时，每秒刷新一次
 */
function ElapsedTime({ startTime }: { startTime: number }) {
    const [now, setNow] = useState(() => Date.now());

    useEffect(() => {
        const timer = setInterval(() => setNow(Date.now()), 1000);
        return () => clearInterval(timer);
    }, []);

    return <>{formatDuration(now - startTime * 1000)}</>;
}

function ActiveRequestCard({ request }: { request: ActiveRelayRequest }) {
    const { Avatar: ModelAvatar, color: brandColor } = useMemo(
        () => getModelIcon(request.request_model_name),
        [request.request_model_name]
    );

    return (
        <div className="flex w-[320px] shrink-0 items-center gap-3 rounded-2xl border border-border bg-card px-4 py-3">
            <ModelAvatar size={36} />
            <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                <div className="flex min-w-0 items-center gap-2 text-sm">
                    <span className="truncate font-semibold text-card-foreground" title={request.request_model_name}>
                        {request.request_model_name}
                    </span>
                    {request.channel_name && (
                        <>
                            <ArrowRight className="size-3.5 shrink-0 text-muted-foreground/50" />
                            <Badge
                                variant="secondary"
                                className="shrink-0 px-1.5 py-0 text-xs"
                                style={{ backgroundColor: `${brandColor}15`, color: brandColor }}
                            >
                                {request.channel_name}
                            </Badge>
                        </>
                    )}
                </div>
                <div className="flex items-center gap-2 text-xs tabular-nums text-muted-foreground">
                    <Loader2 className="size-3 shrink-0 animate-spin" aria-hidden="true" />
                    <span className="truncate">{request.actual_model_name || request.request_model_name}</span>
                    <span className="ml-auto flex shrink-0 items-center gap-1">
                        <Clock className="size-3" aria-hidden="true" />
                        <ElapsedTime startTime={request.time} />
                    </span>
                </div>
            </div>
        </div>
    );
}

/**
 * 日志页面顶部的进行中请求布局
 * - 无进行中请求时整体隐藏
 * - 通过 SSE 的 active_start / active_update / active_end 事件实时增删改
 */
export function ActiveRequests({ requests }: { requests: ActiveRelayRequest[] }) {
    const t = useTranslations('log.active');

    if (requests.length === 0) return null;

    return (
        <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2 px-1">
                <h2 className="text-sm font-semibold text-muted-foreground">{t('title')}</h2>
                <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
                    {requests.length}
                </span>
            </div>
            <div className="flex gap-2 overflow-x-auto pb-1">
                <AnimatePresence initial={false}>
                    {requests.map((request) => (
                        <motion.div
                            key={request.id}
                            layout
                            initial={{ opacity: 0, y: -6 }}
                            animate={{ opacity: 1, y: 0 }}
                            exit={{ opacity: 0, scale: 0.95 }}
                            transition={{ duration: 0.2 }}
                        >
                            <ActiveRequestCard request={request} />
                        </motion.div>
                    ))}
                </AnimatePresence>
            </div>
        </div>
    );
}
