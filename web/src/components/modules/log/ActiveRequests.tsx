import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { ArrowRight, Clock, KeyRound, Loader2 } from 'lucide-react';
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
    const requestAPIKeyName = useMemo(() => request.request_api_key_name?.trim() ?? '', [request.request_api_key_name]);

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
                    <Loader2 className="size-3 shrink-0 animate-spin translate-y-px" aria-hidden="true" />
                    <span className="truncate">{request.actual_model_name || request.request_model_name}</span>
                    {requestAPIKeyName && (
                        <>
                            <KeyRound className="size-3 shrink-0 translate-y-px text-orange-500" aria-hidden="true" />
                            <span className="max-w-24 truncate" title={requestAPIKeyName}>
                                {requestAPIKeyName}
                            </span>
                        </>
                    )}
                    <span className="ml-auto flex shrink-0 items-center gap-1">
                        <Clock className="size-3 translate-y-px" aria-hidden="true" />
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
    const scrollRef = useRef<HTMLDivElement>(null);
    const [maskImage, setMaskImage] = useState('none');

    const checkScroll = useCallback(() => {
        if (!scrollRef.current) return;
        const { scrollLeft, scrollWidth, clientWidth } = scrollRef.current;
        const isStart = scrollLeft <= 1;
        const isEnd = Math.abs(scrollWidth - clientWidth - scrollLeft) <= 1;

        if (isStart && isEnd) {
            setMaskImage('none');
        } else if (isStart) {
            setMaskImage('linear-gradient(to left, transparent, rgba(0,0,0,0) 10px, black 40px)');
        } else if (isEnd) {
            setMaskImage('linear-gradient(to right, transparent, rgba(0,0,0,0) 10px, black 40px)');
        } else {
            setMaskImage('linear-gradient(to right, transparent, rgba(0,0,0,0) 10px, black 40px, black calc(100% - 40px), rgba(0,0,0,0) calc(100% - 10px), transparent)');
        }
    }, []);

    useLayoutEffect(() => {
        checkScroll();
        window.addEventListener('resize', checkScroll);
        return () => window.removeEventListener('resize', checkScroll);
    }, [requests.length, checkScroll]);

    // 桌面端鼠标滚轮横向滚动：仅在该方向仍有滚动余量时接管，边界处交还页面纵向滚动。
    // React 的 onWheel 为 passive 监听无法 preventDefault，这里用原生非 passive 监听。
    useEffect(() => {
        const el = scrollRef.current;
        if (!el) return;
        const handleWheel = (e: WheelEvent) => {
            // 触控板横向手势（deltaX 主导）交给浏览器原生横向滚动
            if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) return;
            let step = e.deltaY;
            if (e.deltaMode === WheelEvent.DOM_DELTA_LINE) {
                step *= 16;
            } else if (e.deltaMode === WheelEvent.DOM_DELTA_PAGE) {
                step *= el.clientHeight;
            }
            const canForward = step > 0 && el.scrollLeft < el.scrollWidth - el.clientWidth - 1;
            const canBackward = step < 0 && el.scrollLeft > 1;
            if (!canForward && !canBackward) return;
            e.preventDefault();
            el.scrollLeft += step;
        };
        el.addEventListener('wheel', handleWheel, { passive: false });
        return () => el.removeEventListener('wheel', handleWheel);
    }, [requests.length]);

    if (requests.length === 0) return null;

    return (
        <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2 px-1">
                <h2 className="text-sm font-semibold text-muted-foreground">{t('title')}</h2>
                <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
                    {requests.length}
                </span>
            </div>
            <div
                ref={scrollRef}
                onScroll={checkScroll}
                className="flex gap-2 overflow-x-auto pb-1"
                style={{ maskImage, WebkitMaskImage: maskImage }}
            >
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
