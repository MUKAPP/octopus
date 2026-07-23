import type { InfiniteData } from '@tanstack/react-query';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient, API_BASE_URL } from '../client';
import { logger } from '@/lib/logger';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

/**
 * 尝试状态
 */
export type AttemptStatus = 'success' | 'failed' | 'circuit_break' | 'skipped';

/**
 * 单次渠道尝试信息
 */
export interface ChannelAttempt {
    channel_id: number;
    channel_key_id?: number;
    channel_name: string;
    model_name: string;
    rate_multiplier: number; // 当时使用的渠道倍率
    attempt_num: number;    // 第几次尝试
    status: AttemptStatus;
    duration: number;       // 耗时(毫秒)
    sticky?: boolean;
    msg?: string;
}

/**
 * 日志数据
 */
export interface RelayLog {
    id: number;
    time: number;                // 时间戳
    request_model_name: string;  // 请求模型名称
    request_api_key_name?: string; // 请求使用的 API Key 名称
    channel: number;             // 实际使用的渠道ID
    channel_name: string;        // 渠道名称
    rate_multiplier: number;     // 当时使用的渠道倍率
    actual_model_name: string;   // 实际使用模型名称
    input_tokens: number;        // 输入Token
    output_tokens: number;       // 输出Token
    ftut: number;                // 首字时间(毫秒)
    use_time: number;            // 总用时(毫秒)
    cost: number;                // 消耗费用
    request_content: string;     // 请求内容
    response_content: string;    // 响应内容
    error: string;               // 错误信息
    attempts?: ChannelAttempt[]; // 所有尝试记录
    total_attempts?: number;     // 总尝试次数
}

/**
 * 日志列表查询参数
 */
export interface LogListParams {
    page?: number;
    page_size?: number;
    start_time?: number;
    end_time?: number;
}

/**
 * 清空日志 Hook
 * 
 * @example
 * const clearLogs = useClearLogs();
 * 
 * clearLogs.mutate();
 */
export function useClearLogs() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async () => {
            return apiClient.delete<null>('/api/v1/log/clear');
        },
        onSuccess: () => {
            logger.log('日志清空成功');
            queryClient.invalidateQueries({ queryKey: ['logs'] });
        },
        onError: (error) => {
            logger.error('日志清空失败:', error);
        },
    });
}

const logsInfiniteQueryKey = (pageSize: number) => ['logs', 'infinite', pageSize] as const;

/**
 * 日志管理 Hook
 * 整合初始加载、SSE 实时推送、滚动加载更多
 * 
 * @example
 * const { logs, isConnected, hasMore, isLoadingMore, loadMore, clear } = useLogs();
 * 
 * // logs 自动包含历史日志和实时日志，按时间倒序
 * logs.forEach(log => console.log(log.request_model_name));
 * 
 * // 滚动到底部时加载更多
 * if (hasMore && !isLoadingMore) loadMore();
 */
export function useLogs(options: { pageSize?: number } = {}) {
    const { pageSize = 20 } = options;

    const [isConnected, setIsConnected] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const eventSourceRef = useRef<EventSource | null>(null);
    const reconnectAttemptRef = useRef(0);
    const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const connectGenerationRef = useRef(0);

    const queryClient = useQueryClient();

    const logsQuery = useInfiniteQuery({
        queryKey: logsInfiniteQueryKey(pageSize),
        initialPageParam: 1,
        queryFn: async ({ pageParam }) => {
            const params = new URLSearchParams();
            params.set('page', String(pageParam));
            params.set('page_size', String(pageSize));
            const result = await apiClient.get<RelayLog[] | null>(`/api/v1/log/list?${params.toString()}`);
            return result ?? [];
        },
        getNextPageParam: (lastPage, allPages) => {
            if (!lastPage || lastPage.length < pageSize) return undefined;
            return allPages.length + 1;
        },
        staleTime: Infinity,
        refetchOnMount: 'always',
    });

    const logs = useMemo(() => {
        const pages = logsQuery.data?.pages ?? [];
        const seen = new Set<number>();
        const merged: RelayLog[] = [];

        for (const page of pages) {
            for (const log of page) {
                if (seen.has(log.id)) continue;
                seen.add(log.id);
                merged.push(log);
            }
        }

        merged.sort((a, b) => b.time - a.time);
        return merged;
    }, [logsQuery.data]);

    const loadMore = useCallback(async () => {
        if (!logsQuery.hasNextPage) return;
        if (logsQuery.isFetchingNextPage) return;

        try {
            await logsQuery.fetchNextPage();
        } catch (e) {
            logger.error('加载更多日志失败:', e);
        }
    }, [logsQuery]);

    const prependLogs = useCallback((incoming: RelayLog[]) => {
        if (incoming.length === 0) return;

        queryClient.setQueryData(
            logsInfiniteQueryKey(pageSize),
            (old: InfiniteData<RelayLog[], number> | undefined) => {
                if (!old) {
                    const sorted = [...incoming].sort((a, b) => b.time - a.time);
                    return { pages: [sorted], pageParams: [1] };
                }

                const existingIds = new Set<number>();
                for (const page of old.pages) {
                    for (const log of page) {
                        existingIds.add(log.id);
                    }
                }

                const freshLogs = incoming
                    .filter((log) => !existingIds.has(log.id))
                    .sort((a, b) => b.time - a.time);

                if (freshLogs.length === 0) return old;

                const firstPage = old.pages[0] ?? [];
                return {
                    ...old,
                    pages: [[...freshLogs, ...firstPage], ...old.pages.slice(1)],
                };
            }
        );
    }, [pageSize, queryClient]);

    const catchUpLatestLogs = useCallback(async () => {
        try {
            // 断线期间可能漏推，回前台/重连后补拉最近一页
            const catchUpPageSize = Math.min(100, Math.max(pageSize, 50));
            const params = new URLSearchParams();
            params.set('page', '1');
            params.set('page_size', String(catchUpPageSize));
            const result = await apiClient.get<RelayLog[] | null>(`/api/v1/log/list?${params.toString()}`);
            if (result?.length) {
                prependLogs(result);
            }
        } catch (e) {
            logger.error('补拉最新日志失败:', e);
        }
    }, [pageSize, prependLogs]);

    useEffect(() => {
        let cancelled = false;
        const maxReconnectDelayMs = 30_000;

        const clearReconnectTimer = () => {
            if (reconnectTimerRef.current !== null) {
                clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = null;
            }
        };

        const closeEventSource = () => {
            if (eventSourceRef.current) {
                eventSourceRef.current.onopen = null;
                eventSourceRef.current.onmessage = null;
                eventSourceRef.current.onerror = null;
                eventSourceRef.current.close();
                eventSourceRef.current = null;
            }
            setIsConnected(false);
        };

        const scheduleReconnect = () => {
            if (cancelled || reconnectTimerRef.current !== null) return;

            const attempt = reconnectAttemptRef.current;
            const delayMs = Math.min(1000 * 2 ** attempt, maxReconnectDelayMs);
            reconnectAttemptRef.current = attempt + 1;

            reconnectTimerRef.current = setTimeout(() => {
                reconnectTimerRef.current = null;
                // 后台标签页里重连意义不大，等回到前台再连，节省资源
                if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
                    return;
                }
                void connect();
            }, delayMs);
        };

        const connect = async (options?: { catchUp?: boolean }) => {
            if (cancelled) return;

            clearReconnectTimer();
            closeEventSource();

            const generation = ++connectGenerationRef.current;
            const shouldCatchUp = options?.catchUp ?? reconnectAttemptRef.current > 0;

            try {
                const { token } = await apiClient.get<{ token: string }>('/api/v1/log/stream-token');
                if (cancelled || generation !== connectGenerationRef.current) return;

                const eventSource = new EventSource(`${API_BASE_URL}/api/v1/log/stream?token=${token}`);
                eventSourceRef.current = eventSource;

                eventSource.onopen = () => {
                    if (cancelled || generation !== connectGenerationRef.current) return;
                    setIsConnected(true);
                    setError(null);
                    reconnectAttemptRef.current = 0;
                    if (shouldCatchUp) {
                        void catchUpLatestLogs();
                    }
                };

                eventSource.onmessage = (event) => {
                    try {
                        const log: RelayLog = JSON.parse(event.data);
                        prependLogs([log]);
                    } catch (e) {
                        logger.error('解析日志数据失败:', e);
                    }
                };

                eventSource.onerror = () => {
                    if (cancelled || generation !== connectGenerationRef.current) return;
                    setIsConnected(false);
                    setError(new Error('SSE 连接断开'));
                    // stream token 一次性使用，浏览器默认重连同一 URL 会 401，需主动重新取 token
                    closeEventSource();
                    scheduleReconnect();
                };
            } catch (e) {
                if (cancelled) return;
                setError(e instanceof Error ? e : new Error('获取 stream token 失败'));
                logger.error('获取 stream token 失败:', e);
                scheduleReconnect();
            }
        };

        const handleVisibilityOrOnline = () => {
            if (cancelled) return;
            if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;

            const readyState = eventSourceRef.current?.readyState;
            const isOpen = readyState === EventSource.OPEN;

            if (!isOpen) {
                // 切回前台后立刻重连，并补拉断线期间的日志
                reconnectAttemptRef.current = 0;
                clearReconnectTimer();
                void connect({ catchUp: true });
                return;
            }

            // 连接仍显示 OPEN 时也可能漏消息，补拉最近日志兜底
            void catchUpLatestLogs();
        };

        void connect();

        if (typeof document !== 'undefined') {
            document.addEventListener('visibilitychange', handleVisibilityOrOnline);
        }
        if (typeof window !== 'undefined') {
            window.addEventListener('online', handleVisibilityOrOnline);
            window.addEventListener('focus', handleVisibilityOrOnline);
        }

        return () => {
            cancelled = true;
            connectGenerationRef.current += 1;
            clearReconnectTimer();
            closeEventSource();

            if (typeof document !== 'undefined') {
                document.removeEventListener('visibilitychange', handleVisibilityOrOnline);
            }
            if (typeof window !== 'undefined') {
                window.removeEventListener('online', handleVisibilityOrOnline);
                window.removeEventListener('focus', handleVisibilityOrOnline);
            }
        };
    }, [catchUpLatestLogs, pageSize, prependLogs]);

    const clear = useCallback(() => {
        queryClient.removeQueries({ queryKey: logsInfiniteQueryKey(pageSize) });
    }, [pageSize, queryClient]);

    return {
        logs,
        isConnected,
        error,
        hasMore: !!logsQuery.hasNextPage,
        isLoading: logsQuery.isLoading,
        isLoadingMore: logsQuery.isFetchingNextPage,
        loadMore,
        clear,
    };
}
