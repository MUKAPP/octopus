import type { InfiniteData } from '@tanstack/react-query';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiRequest } from '../client';
import { logger } from '@/lib/logger';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

/** 日志请求的生命周期状态。 */
export type RequestState = 'running' | 'committed' | 'success' | 'failed' | 'canceled';

/** 概览流发送的单条日志（正文和尝试详情可能按需补充）。 */
export interface RelayLogOverview {
    id: number;
    state: RequestState;
    started_at: string;
    completed_at?: string;
    duration: number;
    request_model: string;
    actual_model: string;
    client_protocol: string;
    stream: boolean;
    final_channel_name: string;
    final_rate_multiplier?: number;
    rate_multiplier?: number;
    input_tokens: number;
    output_tokens: number;
    cache_read_tokens: number;
    cache_write_tokens: number;
    total_cost: number;
    error?: string;
    attempts?: unknown[];
    history?: unknown[];
    request_api_key_name?: string;
    request_content?: string;
    response_content?: string;
}

/** 详情流中的一次渠道尝试。 */
export interface RelayAttemptEvent {
    attempt_index: number;
    channel_name: string;
    model_name: string;
    error?: string;
    status?: string;
    duration?: number;
    rate_multiplier?: number;
    sticky?: boolean;
    channel_id?: number;
    channel_key_id?: number;
}

function asRecord(value: unknown): Record<string, unknown> | null {
    return value && typeof value === 'object' ? value as Record<string, unknown> : null;
}

function numberValue(value: unknown, fallback = 0): number {
    const result = typeof value === 'number' ? value : Number(value);
    return Number.isFinite(result) ? result : fallback;
}

function stringValue(value: unknown, fallback = ''): string {
    return typeof value === 'string' ? value : value == null ? fallback : String(value);
}

function boolValue(value: unknown, fallback = false): boolean {
    return typeof value === 'boolean' ? value : fallback;
}

function timestampSeconds(value: unknown, fallback = 0): number {
    if (typeof value === 'number') {
        return value > 1e12 ? value / 1000 : value;
    }
    if (typeof value === 'string') {
        const parsed = Date.parse(value);
        if (Number.isFinite(parsed)) return parsed / 1000;
        const numeric = Number(value);
        if (Number.isFinite(numeric)) return numeric > 1e12 ? numeric / 1000 : numeric;
    }
    return fallback;
}

/** Go time.Duration 以纳秒序列化；概览时长统一转换为毫秒。 */
function durationNanosecondsToMilliseconds(value: unknown): number {
    const duration = numberValue(value);
    return duration > 0 ? duration / 1_000_000 : 0;
}

function normalizeAttempt(value: unknown, index: number, durationIsNanoseconds: boolean): ChannelAttempt {
    const record = asRecord(value) ?? {};
    const rawStatus = stringValue(record.status);
    const status: AttemptStatus = rawStatus === 'running'
        || rawStatus === 'success'
        || rawStatus === 'failed'
        || rawStatus === 'canceled'
        || rawStatus === 'circuit_break'
        || rawStatus === 'skipped'
        ? rawStatus
        : record.error ? 'failed' : 'success';
    const duration = durationIsNanoseconds
        ? durationNanosecondsToMilliseconds(record.duration)
        : numberValue(record.duration);
    return {
        channel_id: numberValue(record.channel_id),
        channel_key_id: record.channel_key_id == null ? undefined : numberValue(record.channel_key_id),
        channel_name: stringValue(record.channel_name, '—'),
        model_name: stringValue(record.model_name),
        rate_multiplier: numberValue(record.rate_multiplier),
        attempt_num: numberValue(record.attempt_num, numberValue(record.attempt_index, index + 1)),
        attempt_index: numberValue(record.attempt_index, numberValue(record.attempt_num, index + 1)),
        status,
        duration,
        sticky: record.sticky == null ? undefined : boolValue(record.sticky),
        msg: stringValue(record.msg ?? record.error),
    };
}

/** 将新概览或旧列表记录统一为当前日志卡片使用的结构。 */
export function normalizeRelayLog(value: RelayLog | RelayLogOverview | unknown): RelayLog {
    const record = asRecord(value) ?? {};
    if ('request_model' in record || 'started_at' in record || 'state' in record) {
        const hasHistory = Array.isArray(record.history);
        const rawAttempts = hasHistory
            ? record.history as unknown[]
            : Array.isArray(record.attempts) ? record.attempts : [];
        const startedAt = timestampSeconds(record.started_at ?? record.time);
        const state = stringValue(record.state, 'success') as RequestState;
        const requestModel = stringValue(record.request_model ?? record.request_model_name);
        const actualModel = stringValue(record.actual_model ?? record.actual_model_name, requestModel);
        const cacheRead = numberValue(record.cache_read_tokens ?? record.cached_tokens);
        const duration = durationNanosecondsToMilliseconds(record.duration);
        const attempts = rawAttempts.map((attempt, index) => normalizeAttempt(attempt, index, false));
        const finalAttempt = attempts[attempts.length - 1];
        const finalChannel = stringValue(record.final_channel_name ?? record.channel_name, finalAttempt?.channel_name ?? '—');
        const finalRate = numberValue(record.final_rate_multiplier ?? record.rate_multiplier ?? finalAttempt?.rate_multiplier);
        return {
            id: numberValue(record.id),
            time: startedAt,
            request_model_name: requestModel,
            request_api_key_name: stringValue(record.request_api_key_name ?? record.api_key_name) || undefined,
            channel: numberValue(record.channel ?? finalAttempt?.channel_id),
            channel_name: finalChannel,
            rate_multiplier: finalRate,
            actual_model_name: actualModel,
            input_tokens: numberValue(record.input_tokens),
            output_tokens: numberValue(record.output_tokens),
            cached_tokens: cacheRead,
            cache_read_tokens: cacheRead,
            cache_write_tokens: numberValue(record.cache_write_tokens),
            ftut: numberValue(record.ftut ?? record.first_token_time),
            use_time: duration,
            cost: numberValue(record.total_cost ?? record.cost),
            request_content: stringValue(record.request_content ?? record.request_body),
            response_content: stringValue(record.response_content ?? record.response_body),
            request_content_truncated: boolValue(record.request_content_truncated),
            response_content_truncated: boolValue(record.response_content_truncated),
            error: stringValue(record.error),
            attempts,
            total_attempts: numberValue(record.total_attempts, attempts.length),
            state,
            started_at: stringValue(record.started_at),
            completed_at: stringValue(record.completed_at),
            client_protocol: stringValue(record.client_protocol),
            stream: boolValue(record.stream),
            final_channel_name: finalChannel,
            final_rate_multiplier: finalRate,
            is_overview: true,
        };
    }

    const old = record as unknown as RelayLog;
    const attempts = Array.isArray(old.attempts)
        ? old.attempts.map((attempt, index) => normalizeAttempt(attempt, index, false))
        : undefined;
    return { ...old, attempts };
}

/**
 * 尝试状态
 */
export type AttemptStatus = 'running' | 'success' | 'failed' | 'canceled' | 'circuit_break' | 'skipped';

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
    attempt_index?: number; // 新详情流使用的尝试序号
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
    cached_tokens?: number;      // 缓存读取 Token；历史日志可能未采集
    cache_read_tokens?: number;
    cache_write_tokens?: number;
    ftut: number;                // 首字时间(毫秒)
    use_time: number;            // 总用时(毫秒)
    cost: number;                // 消耗费用
    request_content: string;     // 请求内容
    response_content: string;    // 响应内容
    request_content_truncated: boolean;  // 请求内容是否被截断
    response_content_truncated: boolean; // 响应内容是否被截断
    error: string;               // 错误信息
    attempts?: ChannelAttempt[]; // 所有尝试记录
    total_attempts?: number;     // 总尝试次数
    state?: RequestState;
    started_at?: string;
    completed_at?: string;
    client_protocol?: string;
    stream?: boolean;
    final_channel_name?: string;
    final_rate_multiplier?: number;
    is_overview?: boolean;
}
function getEffectiveLogState(log: RelayLog): RequestState {
    return log.state ?? (log.error ? 'failed' : 'success');
}

function isTerminalLogState(state: RequestState): boolean {
    return state === 'success' || state === 'failed' || state === 'canceled';
}

function mergeRelayLog(existing: RelayLog, incoming: RelayLog): RelayLog {
    const existingState = getEffectiveLogState(existing);
    const incomingState = getEffectiveLogState(incoming);
    const existingTerminal = isTerminalLogState(existingState);
    const incomingTerminal = isTerminalLogState(incomingState);

    if (existingTerminal !== incomingTerminal) {
        return incomingTerminal ? incoming : existing;
    }
    if (existingTerminal && incomingTerminal) {
        if (existingState === 'success' && incomingState !== 'success') return existing;
        if (incomingState === 'success' && existingState !== 'success') return incoming;
    }
    return incoming;
}

function mergeRelayLogList(logs: RelayLog[]): RelayLog[] {
    const byID = new Map<number, RelayLog>();
    for (const log of logs) {
        const existing = byID.get(log.id);
        byID.set(log.id, existing ? mergeRelayLog(existing, log) : log);
    }
    return Array.from(byID.values()).sort((a, b) => b.time - a.time || b.id - a.id);
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
 * 进行中请求数据
 */
export interface ActiveRelayRequest {
    id: number;                    // Snowflake ID，与完成后的日志 ID 一致
    time: number;                  // 开始时间戳（秒）
    request_model_name: string;    // 请求模型名称
    request_api_key_name?: string; // 请求使用的 API Key 名称
    channel_name?: string;         // 当前尝试的渠道名称
    actual_model_name?: string;    // 当前尝试的实际上游模型名称
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
            return apiRequest<null>('/api/v1/log/clear', { method: 'DELETE' });
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

/** 中止指定请求当前正在执行的渠道尝试。 */
export function useStopAttempt() {
    return useMutation({
        mutationFn: ({ requestId, attemptIndex }: { requestId: number; attemptIndex: number }) =>
            apiRequest<null>(`/api/v1/log/${requestId}/${attemptIndex}/stop`, { method: 'POST' }),
    });
}

export interface RelayLogBody {
    content?: string;
    truncated?: boolean;
}

function normalizeLogBody(value: unknown): RelayLogBody {
    if (typeof value === 'string') return { content: value };
    const record = asRecord(value) ?? {};
    const content = record.content ?? record.body;
    return {
        content: content == null ? undefined : String(content),
        truncated: record.truncated === true,
    };
}

/** 在打开详情时按需获取请求正文。 */
export function useLogRequestBody(id: number, startedAt: string | undefined, enabled: boolean) {
    return useQuery({
        queryKey: ['logs', id, startedAt ?? '', 'request-body'],
        queryFn: async () => normalizeLogBody(await apiRequest<unknown>(`/api/v1/log/${id}/request-body`)),
        enabled,
        staleTime: Infinity,
    });
}

/** 在打开详情时按需获取最终响应正文。 */
export function useLogResponseBody(id: number, startedAt: string | undefined, enabled: boolean) {
    return useQuery({
        queryKey: ['logs', id, startedAt ?? '', 'response-body'],
        queryFn: async () => normalizeLogBody(await apiRequest<unknown>(`/api/v1/log/${id}/response-body`)),
        enabled,
        staleTime: Infinity,
    });
}

function attemptFromDetail(value: unknown, index = 0): ChannelAttempt {
    const envelope = asRecord(value) ?? {};
    const record = asRecord(envelope.attempt) ?? envelope;
    return normalizeAttempt({
        ...record,
        attempt_num: record.attempt_num ?? record.attempt_index,
    }, index, false);
}

function detailAttemptsFromPayload(value: unknown): ChannelAttempt[] {
    const record = asRecord(value);
    const candidates = record && (Array.isArray(record.history) ? record.history : record.attempts);
    if (!Array.isArray(candidates)) return [];
    return candidates.map((attempt, index) => attemptFromDetail(attempt, index));
}

function detailPayload(value: unknown): Record<string, unknown> {
    const record = asRecord(value);
    const nested = record?.data;
    return asRecord(nested) ?? record ?? {};
}

/** 为日志详情弹窗订阅尝试历史，兼容运行中和已完成记录。 */
export function useLogDetailStream(id: number, state: RequestState | undefined, enabled: boolean) {
    const [attempts, setAttempts] = useState<ChannelAttempt[]>([]);
    const [runningAttempt, setRunningAttempt] = useState<ChannelAttempt | null>(null);
    const [isCommitted, setIsCommitted] = useState(state === 'committed' || state === 'success');
    const [isConnected, setIsConnected] = useState(false);
    const [error, setError] = useState<Error | null>(null);

    useEffect(() => {
        let cancelled = false;
        let source: EventSource | null = null;
        let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
        let reconnectAttempt = 0;

        setAttempts([]);
        setRunningAttempt(null);
        setIsCommitted(state === 'committed' || state === 'success');
        setIsConnected(false);
        setError(null);

        const terminal = state === 'success' || state === 'failed' || state === 'canceled';
        if (!enabled || terminal) {
            return () => {
                cancelled = true;
            };
        }

        const closeSource = () => {
            if (!source) return;
            source.onopen = null;
            source.onerror = null;
            source.onmessage = null;
            source.close();
            source = null;
        };

        const scheduleReconnect = () => {
            if (cancelled || reconnectTimer !== null) return;
            const delay = Math.min(1000 * 2 ** reconnectAttempt, 30_000);
            reconnectAttempt += 1;
            reconnectTimer = setTimeout(() => {
                reconnectTimer = null;
                void connect();
            }, delay);
        };

        const applyAttempt = (value: unknown, eventType: 'started' | 'finished') => {
            const parsed = attemptFromDetail(value);
            const next: ChannelAttempt = eventType === 'started' ? { ...parsed, status: 'running' } : parsed;
            setAttempts((current) => {
                const index = current.findIndex((attempt) => (
                    (attempt.attempt_index ?? attempt.attempt_num) === (next.attempt_index ?? next.attempt_num)
                ));
                if (index < 0) return [...current, next].sort((a, b) => (a.attempt_index ?? a.attempt_num) - (b.attempt_index ?? b.attempt_num));
                const updated = [...current];
                updated[index] = { ...updated[index], ...next };
                return updated;
            });
            if (eventType === 'started' || next.status === 'running') {
                setRunningAttempt(next);
            } else {
                setRunningAttempt((current) => (
                    current && (current.attempt_index ?? current.attempt_num) === (next.attempt_index ?? next.attempt_num)
                        ? null
                        : current
                ));
            }
        };

        const applySnapshot = (value: unknown) => {
            const payload = detailPayload(value);
            const history = detailAttemptsFromPayload(payload);
            if (history.length) setAttempts(history);
            const currentAttempt = payload.current_attempt;
            const currentIndex = numberValue(payload.current_attempt_index, -1);
            if (currentAttempt) {
                setRunningAttempt(attemptFromDetail(currentAttempt));
            } else if (currentIndex >= 0) {
                setRunningAttempt(history.find((attempt) => (attempt.attempt_index ?? attempt.attempt_num) === currentIndex) ?? null);
            }
            if (payload.state === 'committed' || payload.state === 'success') setIsCommitted(true);
        };

        const connect = async () => {
            if (cancelled || !enabled) return;
            try {
                source = new EventSource(`/api/v1/log/${id}/stream`, { withCredentials: true });
                source.onopen = () => {
                    if (cancelled) return;
                    reconnectAttempt = 0;
                    setIsConnected(true);
                    setError(null);
                };
                source.onmessage = (event) => {
                    try {
                        applySnapshot(JSON.parse(event.data));
                    } catch (cause) {
                        logger.error('解析日志详情快照失败:', cause);
                    }
                };
                source.addEventListener('log', (event) => {
                    try {
                        applySnapshot(JSON.parse((event as MessageEvent<string>).data));
                    } catch (cause) {
                        logger.error('解析日志详情快照失败:', cause);
                    }
                });
                source.addEventListener('attempt.started', (event) => {
                    try {
                        applyAttempt(JSON.parse((event as MessageEvent<string>).data), 'started');
                    } catch (cause) {
                        logger.error('解析日志尝试失败:', cause);
                    }
                });
                source.addEventListener('attempt.finished', (event) => {
                    try {
                        applyAttempt(JSON.parse((event as MessageEvent<string>).data), 'finished');
                    } catch (cause) {
                        logger.error('解析日志尝试失败:', cause);
                    }
                });
                source.addEventListener('response.committed', (event) => {
                    setRunningAttempt(null);
                    setIsCommitted(true);
                    if ((event as MessageEvent<string>).data) {
                        try {
                            applySnapshot(JSON.parse((event as MessageEvent<string>).data));
                        } catch {
                            // response.committed 可能没有 data。
                        }
                    }
                });
                source.onerror = () => {
                    if (cancelled) return;
                    setIsConnected(false);
                    setError(new Error('日志详情流已断开'));
                    closeSource();
                    scheduleReconnect();
                };
            } catch (cause) {
                if (cancelled) return;
                setIsConnected(false);
                setError(cause instanceof Error ? cause : new Error('创建日志详情流失败'));
                scheduleReconnect();
            }
        };

        if (enabled) void connect();
        return () => {
            cancelled = true;
            if (reconnectTimer !== null) clearTimeout(reconnectTimer);
            closeSource();
        };
    }, [enabled, id, state]);

    return { attempts, runningAttempt, isCommitted, isConnected, error };
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
    const [activeRequests, setActiveRequests] = useState<ActiveRelayRequest[]>([]);
    const [overviewLogs, setOverviewLogs] = useState<RelayLog[]>([]);
    const [overviewUsable, setOverviewUsable] = useState(false);
    const overviewUsableRef = useRef(false);
    const eventSourceRef = useRef<EventSource | null>(null);
    const reconnectAttemptRef = useRef(0);
    const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const connectGenerationRef = useRef(0);
    const activeEventVersionRef = useRef(0);
    const activeFetchInFlightRef = useRef(false);
    // 后台期间 JS 定时器会被冻结/节流，用标记 + 隐藏时长判断是否必须强制重建连接
    const needsReconnectRef = useRef(false);
    const hiddenAtRef = useRef<number | null>(null);
    const connectingRef = useRef(false);
    const catchUpInFlightRef = useRef(false);
    const overviewSourceRef = useRef<EventSource | null>(null);

    const queryClient = useQueryClient();

    const logsQuery = useInfiniteQuery({
        queryKey: logsInfiniteQueryKey(pageSize),
        initialPageParam: 1,
        queryFn: async ({ pageParam }) => {
            const params = new URLSearchParams();
            params.set('page', String(pageParam));
            params.set('page_size', String(pageSize));
            const result = await apiRequest<unknown[] | null>(`/api/v1/log/list?${params.toString()}`);
            return (result ?? []).map(normalizeRelayLog);
        },
        getNextPageParam: (lastPage, allPages) => {
            if (!lastPage || lastPage.length < pageSize) return undefined;
            return allPages.length + 1;
        },
        staleTime: Infinity,
        // 切回日志页时不重新加载所有历史分页；由初始查询完成后的最新日志补拉同步增量，
        // 避免分页 refetch 的旧响应覆盖 SSE/补拉刚写入的最新日志。
        refetchOnMount: false,
    });

    const fallbackLogs = useMemo(() => {
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
    const mergedLogs = useMemo(() => {
        if (!overviewUsable) return fallbackLogs;
        return mergeRelayLogList([...fallbackLogs, ...overviewLogs]);
    }, [fallbackLogs, overviewLogs, overviewUsable]);

    // 新日志流提供完整的进程内快照和运行中请求更新；连接不可用时继续使用旧分页/日志流。
    useEffect(() => {
        let cancelled = false;
        let source: EventSource | null = null;
        let retryTimer: ReturnType<typeof setTimeout> | null = null;
        let retryAttempt = 0;
        let opened = false;

        const closeSource = () => {
            if (!source) return;
            source.onopen = null;
            source.onerror = null;
            source.onmessage = null;
            source.close();
            if (overviewSourceRef.current === source) overviewSourceRef.current = null;
            source = null;
        };

        const disableOverview = () => {
            overviewUsableRef.current = false;
            setOverviewUsable(false);
            setOverviewLogs([]);
        };

        const applyOverview = (event: MessageEvent<string>) => {
            try {
                const parsed = JSON.parse(event.data) as unknown;
                const next = normalizeRelayLog(parsed);
                if (!next.id) return;
                setOverviewLogs((current) => mergeRelayLogList([...current, next]));
            } catch (cause) {
                logger.error('解析日志概览失败:', cause);
            }
        };

        const scheduleReconnect = () => {
            if (cancelled || retryTimer !== null) return;
            // 新路由不存在时让旧 /list + /stream 继续工作，不在后台无限制造请求。
            if (!opened && retryAttempt >= 3) return;
            const delay = Math.min(1000 * 2 ** retryAttempt, 30_000);
            retryAttempt += 1;
            retryTimer = setTimeout(() => {
                retryTimer = null;
                void connect();
            }, delay);
        };

        const connect = async () => {
            if (cancelled) return;
            try {
                source = new EventSource('/api/v1/log/overview/stream', { withCredentials: true });
                overviewSourceRef.current = source;
                source.onopen = () => {
                    if (cancelled) return;
                    opened = true;
                    retryAttempt = 0;
                    overviewUsableRef.current = true;
                    setOverviewUsable(true);
                    setIsConnected(true);
                    setError(null);
                };
                source.onmessage = applyOverview;
                source.addEventListener('log', applyOverview);
                source.onerror = () => {
                    if (cancelled) return;
                    disableOverview();
                    closeSource();
                    if (opened) {
                        setIsConnected(false);
                        setError(new Error('日志概览流已断开'));
                    }
                    scheduleReconnect();
                };
            } catch (cause) {
                if (cancelled) return;
                if (opened) {
                    disableOverview();
                    setError(cause instanceof Error ? cause : new Error('创建日志概览流失败'));
                }
                scheduleReconnect();
            }
        };

        void connect();
        return () => {
            cancelled = true;
            if (retryTimer !== null) clearTimeout(retryTimer);
            closeSource();
        };
    }, []);


    const overviewActiveRequests = useMemo<ActiveRelayRequest[]>(() => {
        return overviewLogs
            .filter((log) => log.state === 'running' || log.state === 'committed')
            .map((log) => {
                const currentAttempt = log.attempts?.find((attempt) => attempt.status === 'running') ?? log.attempts?.[log.attempts.length - 1];
                return {
                    id: log.id,
                    time: log.time,
                    request_model_name: log.request_model_name,
                    request_api_key_name: log.request_api_key_name,
                    channel_name: currentAttempt?.channel_name ?? log.channel_name,
                    actual_model_name: currentAttempt?.model_name ?? log.actual_model_name,
                };
            });
    }, [overviewLogs]);

    const logs = mergedLogs;
    const visibleActiveRequests = overviewUsable ? overviewActiveRequests : activeRequests;
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

    const applyActiveEvent = useCallback((event: { type: 'start' | 'update' | 'end'; request?: ActiveRelayRequest; id?: number }) => {
        activeEventVersionRef.current += 1;
        setActiveRequests((prev) => {
            switch (event.type) {
                case 'start':
                    if (!event.request) return prev;
                    if (prev.some((r) => r.id === event.request!.id)) return prev;
                    return [...prev, event.request!];
                case 'update': {
                    if (!event.request) return prev;
                    const exists = prev.some((r) => r.id === event.request!.id);
                    if (!exists) return [...prev, event.request!];
                    return prev.map((r) => (r.id === event.request!.id ? event.request! : r));
                }
                case 'end':
                    if (event.id === undefined) return prev;
                    return prev.filter((r) => r.id !== event.id);
                default:
                    return prev;
            }
        });
    }, []);

    const fetchActive = useCallback(async () => {
        if (activeFetchInFlightRef.current) return;
        activeFetchInFlightRef.current = true;
        const eventVersion = activeEventVersionRef.current;
        try {
            const result = await apiRequest<ActiveRelayRequest[] | null>('/api/v1/log/active');
            if (eventVersion === activeEventVersionRef.current) {
                setActiveRequests(result ?? []);
            }
        } catch (e) {
            logger.error('获取进行中请求失败:', e);
        } finally {
            activeFetchInFlightRef.current = false;
        }
    }, []);

    const catchUpLatestLogs = useCallback(async () => {
        if (catchUpInFlightRef.current) return;
        catchUpInFlightRef.current = true;
        try {
            // 断线期间可能漏推，回前台/重连后补拉最近一页
            const catchUpPageSize = Math.min(100, Math.max(pageSize, 50));
            const params = new URLSearchParams();
            params.set('page', '1');
            params.set('page_size', String(catchUpPageSize));
            const result = await apiRequest<unknown[] | null>(`/api/v1/log/list?${params.toString()}`);
            if (result?.length) {
                prependLogs(result.map(normalizeRelayLog));
            }
        } catch (e) {
            logger.error('补拉最新日志失败:', e);
        } finally {
            catchUpInFlightRef.current = false;
        }
    }, [pageSize, prependLogs]);

    useEffect(() => {
        if (logsQuery.isFetched) {
            void catchUpLatestLogs();
        }
    }, [logsQuery.isFetched, catchUpLatestLogs]);

    useEffect(() => {
        let cancelled = false;
        const maxReconnectDelayMs = 30_000;
        // 后台超过该时长后，即使 readyState 仍是 OPEN 也强制重建（避免僵尸连接）
        const forceReconnectAfterHiddenMs = 5_000;

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
            connectingRef.current = false;
        };

        const scheduleReconnect = () => {
            if (cancelled || reconnectTimerRef.current !== null) return;

            needsReconnectRef.current = true;

            // 后台标签页里定时器不可靠，只记标记，等 visibilitychange 再连
            if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
                return;
            }

            const attempt = reconnectAttemptRef.current;
            const delayMs = Math.min(1000 * 2 ** attempt, maxReconnectDelayMs);
            reconnectAttemptRef.current = attempt + 1;

            reconnectTimerRef.current = setTimeout(() => {
                reconnectTimerRef.current = null;
                if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
                    needsReconnectRef.current = true;
                    return;
                }
                void connect();
            }, delayMs);
        };

        const connect = async (options?: { catchUp?: boolean; force?: boolean }) => {
            if (cancelled) return;
            // force 时允许打断卡住的连接过程（长时间后台后 connecting 可能残留）
            if (connectingRef.current && !options?.force) return;

            clearReconnectTimer();
            closeEventSource();

            connectingRef.current = true;
            const generation = ++connectGenerationRef.current;
            const shouldCatchUp =
                Boolean(options?.catchUp)
                || needsReconnectRef.current
                || reconnectAttemptRef.current > 0;

            try {
                if (cancelled || generation !== connectGenerationRef.current) {
                    if (generation === connectGenerationRef.current) {
                        connectingRef.current = false;
                    }
                    return;
                }

                const eventSource = new EventSource('/api/v1/log/stream', { withCredentials: true });
                eventSourceRef.current = eventSource;
                // EventSource 已创建后就释放 connecting 锁；是否连上由 onopen/onerror 处理
                connectingRef.current = false;

                eventSource.onopen = () => {
                    if (cancelled || generation !== connectGenerationRef.current) return;
                    setIsConnected(true);
                    setError(null);
                    reconnectAttemptRef.current = 0;
                    needsReconnectRef.current = false;
                    void fetchActive();
                    if (shouldCatchUp) {
                        void catchUpLatestLogs();
                    }
                };

                eventSource.onmessage = (event) => {
                    try {
                        const log = normalizeRelayLog(JSON.parse(event.data));
                        prependLogs([log]);
                        // 请求结束的日志到达时同步移除进行中条目（active_end 事件丢失时的兜底）
                        setActiveRequests((prev) => prev.filter((r) => r.id !== log.id));
                    } catch (e) {
                        logger.error('解析日志数据失败:', e);
                    }
                };

                eventSource.addEventListener('active_start', (event) => {
                    try {
                        const request: ActiveRelayRequest = JSON.parse(event.data);
                        applyActiveEvent({ type: 'start', request });
                    } catch (e) {
                        logger.error('解析进行中请求事件失败:', e);
                    }
                });

                eventSource.addEventListener('active_update', (event) => {
                    try {
                        const request: ActiveRelayRequest = JSON.parse(event.data);
                        applyActiveEvent({ type: 'update', request });
                    } catch (e) {
                        logger.error('解析进行中请求事件失败:', e);
                    }
                });

                eventSource.addEventListener('active_end', (event) => {
                    try {
                        const data: { id?: number } = JSON.parse(event.data);
                        if (typeof data.id === 'number') {
                            applyActiveEvent({ type: 'end', id: data.id });
                        }
                    } catch (e) {
                        logger.error('解析进行中请求事件失败:', e);
                    }
                });

                eventSource.onerror = () => {
                    if (cancelled || generation !== connectGenerationRef.current) return;
                    setIsConnected(false);
                    setError(new Error('SSE 连接断开'));
                    // Reconnect with a fresh EventSource so the browser re-sends the auth cookie.
                    closeEventSource();
                    scheduleReconnect();
                };
            } catch (e) {
                if (cancelled || generation !== connectGenerationRef.current) return;
                connectingRef.current = false;
                setError(e instanceof Error ? e : new Error('创建日志流失败'));
                logger.error('创建日志流失败:', e);
                scheduleReconnect();
            }
        };

        const resumeStream = (reason: string) => {
            if (cancelled) return;
            if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;

            const readyState = eventSourceRef.current?.readyState;
            const isOpen = readyState === EventSource.OPEN;
            const hiddenForMs =
                hiddenAtRef.current !== null ? Date.now() - hiddenAtRef.current : 0;
            const wasHiddenLongEnough = hiddenForMs >= forceReconnectAfterHiddenMs;
            const shouldForceReconnect =
                !isOpen || needsReconnectRef.current || wasHiddenLongEnough;

            logger.log(`日志 SSE 恢复: ${reason}`, {
                isOpen,
                needsReconnect: needsReconnectRef.current,
                hiddenForMs,
                shouldForceReconnect,
            });

            // 无论 SSE 是否可用，先用列表接口补齐，避免长时间后台后“既不推也不拉”
            void catchUpLatestLogs();
            void fetchActive();

            if (shouldForceReconnect) {
                // 切回前台后立刻重建连接，并补拉断线期间的日志
                reconnectAttemptRef.current = 0;
                clearReconnectTimer();
                needsReconnectRef.current = true;
                void connect({ catchUp: true, force: true });
                return;
            }
        };

        const handleVisibilityChange = () => {
            if (typeof document === 'undefined') return;

            if (document.visibilityState === 'hidden') {
                hiddenAtRef.current = Date.now();
                return;
            }

            resumeStream('visibilitychange');
            hiddenAtRef.current = null;
        };

        const handleFocusOrOnline = () => {
            resumeStream('focus/online');
            hiddenAtRef.current = null;
        };

        void connect();
        const activeSyncTimer = setInterval(() => {
            void fetchActive();
        }, 10_000);

        if (typeof document !== 'undefined') {
            if (document.visibilityState === 'hidden') {
                hiddenAtRef.current = Date.now();
            }
            document.addEventListener('visibilitychange', handleVisibilityChange);
        }
        if (typeof window !== 'undefined') {
            window.addEventListener('online', handleFocusOrOnline);
            window.addEventListener('focus', handleFocusOrOnline);
            window.addEventListener('pageshow', handleFocusOrOnline);
        }

        return () => {
            cancelled = true;
            connectGenerationRef.current += 1;
            clearReconnectTimer();
            closeEventSource();
            needsReconnectRef.current = false;
            hiddenAtRef.current = null;

            if (typeof document !== 'undefined') {
                document.removeEventListener('visibilitychange', handleVisibilityChange);
            }
            if (typeof window !== 'undefined') {
                window.removeEventListener('online', handleFocusOrOnline);
                window.removeEventListener('focus', handleFocusOrOnline);
                window.removeEventListener('pageshow', handleFocusOrOnline);
            }
            clearInterval(activeSyncTimer);
        };
    }, [applyActiveEvent, catchUpLatestLogs, fetchActive, pageSize, prependLogs]);

    const clear = useCallback(() => {
        queryClient.removeQueries({ queryKey: logsInfiniteQueryKey(pageSize) });
    }, [pageSize, queryClient]);

    return {
        logs,
        activeRequests: visibleActiveRequests,
        isConnected,
        isOverview: overviewUsable,
        error,
        listError: logsQuery.error,
        isError: logsQuery.isError,
        isFetching: logsQuery.isFetching,
        hasMore: !!logsQuery.hasNextPage,
        isLoading: logsQuery.isLoading,
        isLoadingMore: logsQuery.isFetchingNextPage,
        loadMore,
        refetch: logsQuery.refetch,
        clear,
    };
}
