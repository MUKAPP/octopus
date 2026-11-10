import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import { apiRequest } from '../client';
import { formatCount, formatMoney, formatTime } from '@/lib/utils';

/**
 * 数据分析
 */

export type AnalyticsPeriod = 'today' | '7d' | '30d' | 'all' | 'custom';

export interface AnalyticsFilters {
    period: AnalyticsPeriod;
    /** YYYY-MM-DD */
    customStartDate: string | null;
    /** YYYY-MM-DD */
    customEndDate: string | null;
    /** null=all，''=unknown */
    model: string | null;
    /** null=all，''=unknown */
    actualModel: string | null;
    apiKey: { id: number; name: string } | null;
    channel: { id: number; name: string } | null;
    status: 'all' | 'success' | 'failed';
    stream: 'all' | 'true' | 'false';
    reasoningEffort: string | null;
    taskType: string | null;
    clientProtocol: string | null;
    errorCategory: string | null;
}

interface AnalyticsMetrics {
    input_token: number;
    output_token: number;
    input_cost: number;
    output_cost: number;
    wait_time: number;
    request_success: number;
    request_failed: number;
    cached_token: number;
    observed_cached_token: number | null;
    cache_read_sample_count: number;
    cache_ratio_sample_count: number;
    cache_read_ratio: number | null;
}

export interface AnalyticsDistribution {
    sample_count: number;
    p50: number | null;
    p95: number | null;
}

export interface AnalyticsFeatureRow {
    name: string;
    sample_count: number;
    input_token: number;
    output_token: number;
    input_cost: number;
    output_cost: number;
    request_success: number;
    request_failed: number;
    cache_write_token: number | null;
    ttft_ms: AnalyticsDistribution;
    total_duration_ms: AnalyticsDistribution;
    stream_output_tokens_per_second: AnalyticsDistribution & { value: number | null };
}

export interface AnalyticsDetails {
    coverage: {
        available_from: string;
        available_from_ms: number | null;
        sample_count: number;
        legacy_request_count: number;
        summary_source: 'daily_aggregates' | 'request_metadata';
    };
    performance: {
        ttft_ms: AnalyticsDistribution;
        total_duration_ms: AnalyticsDistribution;
        stream_output_tokens_per_second: AnalyticsDistribution & { value: number | null };
    };
    tokens: {
        usage_sample_count: number;
        cache_read_sample_count: number;
        cache_write_sample_count: number;
        reasoning_sample_count: number;
        cache_read_token: number | null;
        cache_write_token: number | null;
        reasoning_token: number | null;
    };
    reliability: {
        executed_attempts: number;
        retry_requests: number;
        retry_attempts: number;
        recovered_requests: number;
        circuit_break_skips: number;
        other_skips: number;
    };
    by_error_category: { name: string; count: number }[];
    by_status_code: { status_code: number; count: number }[];
    hourly: {
        hour: string;
        request_count: number;
        input_token: number;
        output_token: number;
        input_cost: number;
        output_cost: number;
    }[];
    peaks: { rpm: number | null; rpm_at: string; tpm: number | null; tpm_at: string };
    by_stream: AnalyticsFeatureRow[];
    by_reasoning_effort: AnalyticsFeatureRow[];
    by_client_protocol: AnalyticsFeatureRow[];
    by_task_type: AnalyticsFeatureRow[];
}

export interface AnalyticsResponse {
    available_from: string;
    resolved_start_date: string;
    resolved_end_date: string;
    summary: AnalyticsMetrics;
    details: AnalyticsDetails;
    trend: ({ date: string } & AnalyticsMetrics)[];
    by_model: ({ name: string } & AnalyticsMetrics)[];
    by_actual_model: ({ name: string } & AnalyticsMetrics)[];
    by_api_key: ({ id: number; name: string } & AnalyticsMetrics)[];
    by_channel: ({ id: number; name: string } & AnalyticsMetrics)[];
}

export interface AnalyticsDimensions {
    models: string[];
    actual_models: string[];
    api_keys: { id: number; name: string }[];
    channels: { id: number; name: string }[];
    streams: string[];
    reasoning_efforts: string[];
    task_types: string[];
    client_protocols: string[];
    error_categories: string[];
}

/** 与后端历史统计一致：同 ID 的不同名称属于不同实体快照。 */
export function analyticsEntityKey(entity: { id: number; name: string }): string {
    return JSON.stringify([entity.id, entity.name]);
}

export type FormattedValue = { raw: number; formatted: { value: string; unit: string } };

export interface AnalyticsMetricsFormatted {
    input_token: FormattedValue;
    output_token: FormattedValue;
    input_cost: FormattedValue;
    output_cost: FormattedValue;
    wait_time: FormattedValue;
    request_success: FormattedValue;
    request_failed: FormattedValue;
    cached_token: FormattedValue;
    observed_cached_token: FormattedValue | null;
    cache_read_sample_count: number;
    cache_ratio_sample_count: number;
    request_count: FormattedValue;
    total_token: FormattedValue;
    total_cost: FormattedValue;
    /** 0~100，分母为 0 时 raw 为 0 */
    success_rate: { raw: number; formatted: string };
    average_wait_time: FormattedValue;
    average_cost: FormattedValue;
    cache_read_ratio: { raw: number | null; formatted: string };
}

export interface AnalyticsResponseFormatted {
    available_from: string;
    resolved_start_date: string;
    resolved_end_date: string;
    summary: AnalyticsMetricsFormatted;
    details: AnalyticsDetails;
    trend: { date: string; metrics: AnalyticsMetricsFormatted }[];
    by_model: { name: string; metrics: AnalyticsMetricsFormatted }[];
    by_actual_model: { name: string; metrics: AnalyticsMetricsFormatted }[];
    by_api_key: { id: number; name: string; metrics: AnalyticsMetricsFormatted }[];
    by_channel: { id: number; name: string; metrics: AnalyticsMetricsFormatted }[];
}

/**
 * 构造规范化查询参数。
 * 键序固定：period, start_date, end_date, model, actual_model,
 * api_key_id, api_key_name, channel_id, channel_name, status。
 * custom 范围无效（缺日期/无法解析/开始晚于结束）时返回 null，调用方停用查询。
 */
function buildAnalyticsParams(filters: AnalyticsFilters): Record<string, string> | null {
    const params: Record<string, string> = { period: filters.period };

    if (filters.period === 'custom') {
        // 持久化值为 YYYY-MM-DD，API 要求严格 YYYYMMDD（服务器本地日历日）
        const startMatch = /^(\d{4})-(\d{2})-(\d{2})$/.exec(filters.customStartDate ?? '');
        const endMatch = /^(\d{4})-(\d{2})-(\d{2})$/.exec(filters.customEndDate ?? '');
        const start = startMatch ? `${startMatch[1]}${startMatch[2]}${startMatch[3]}` : null;
        const end = endMatch ? `${endMatch[1]}${endMatch[2]}${endMatch[3]}` : null;
        if (!start || !end || start > end) {
            return null;
        }
        params.start_date = start;
        params.end_date = end;
    }

    if (filters.model !== null) {
        params.model = filters.model;
    }
    if (filters.actualModel !== null) {
        params.actual_model = filters.actualModel;
    }
    if (filters.apiKey !== null) {
        params.api_key_id = String(filters.apiKey.id);
        params.api_key_name = filters.apiKey.name;
    }
    if (filters.channel !== null) {
        params.channel_id = String(filters.channel.id);
        params.channel_name = filters.channel.name;
    }
    if (filters.status !== 'all') {
        params.status = filters.status;
    }

    if (filters.stream !== 'all') params.stream = filters.stream;
    if (filters.reasoningEffort !== null) params.reasoning_effort = filters.reasoningEffort;
    if (filters.taskType !== null) params.task_type = filters.taskType;
    if (filters.clientProtocol !== null) params.client_protocol = filters.clientProtocol;
    if (filters.errorCategory !== null) params.error_category = filters.errorCategory;

    return params;
}

function formatMetrics(metrics: AnalyticsMetrics): AnalyticsMetricsFormatted {
    const requestCount = metrics.request_success + metrics.request_failed;
    const totalCost = metrics.input_cost + metrics.output_cost;
    const totalToken = metrics.input_token + metrics.output_token;
    const averageCost = requestCount > 0 ? totalCost / requestCount : 0;
    const formattedAverageCost = formatMoney(averageCost);
    if (averageCost > 0 && averageCost < 0.01) formattedAverageCost.formatted.value = averageCost.toPrecision(3);
    const successRate = requestCount > 0 ? (metrics.request_success / requestCount) * 100 : 0;
    const averageWaitTime = requestCount > 0 ? metrics.wait_time / requestCount : 0;

    return {
        input_token: formatCount(metrics.input_token),
        output_token: formatCount(metrics.output_token),
        input_cost: formatMoney(metrics.input_cost),
        output_cost: formatMoney(metrics.output_cost),
        wait_time: formatTime(metrics.wait_time),
        request_success: formatCount(metrics.request_success),
        request_failed: formatCount(metrics.request_failed),
        cached_token: formatCount(metrics.cached_token),
        observed_cached_token: metrics.observed_cached_token === null ? null : formatCount(metrics.observed_cached_token),
        cache_read_sample_count: metrics.cache_read_sample_count,
        cache_ratio_sample_count: metrics.cache_ratio_sample_count,
        request_count: formatCount(requestCount),
        total_token: formatCount(totalToken),
        total_cost: formatMoney(totalCost),
        success_rate: { raw: successRate, formatted: `${successRate.toFixed(1)}%` },
        average_wait_time: formatTime(averageWaitTime),
        average_cost: formattedAverageCost,
        cache_read_ratio: {
            raw: metrics.cache_read_ratio,
            formatted: metrics.cache_read_ratio === null ? '—' : `${metrics.cache_read_ratio.toFixed(1)}%`,
        },
    };
}

/**
 * 获取分析数据。custom 范围无效时查询自动停用（enabled=false）。
 */
export function useAnalytics(filters: AnalyticsFilters, enabled = true): UseQueryResult<AnalyticsResponseFormatted> {
    const params = buildAnalyticsParams(filters);

    return useQuery({
        queryKey: ['stats', 'analytics', params ?? { period: filters.period }],
        queryFn: async () => {
            return apiRequest<AnalyticsResponse>('/api/v1/stats/analytics', { params: params ?? undefined });
        },
        enabled: enabled && params !== null,
        select: (data): AnalyticsResponseFormatted => ({
            available_from: data.available_from,
            resolved_start_date: data.resolved_start_date,
            resolved_end_date: data.resolved_end_date,
            summary: formatMetrics(data.summary),
            details: data.details,
            trend: data.trend.map((item) => ({
                date: item.date,
                metrics: formatMetrics(item),
            })),
            by_model: data.by_model.map((item) => ({
                name: item.name,
                metrics: formatMetrics(item),
            })),
            by_actual_model: data.by_actual_model.map((item) => ({
                name: item.name,
                metrics: formatMetrics(item),
            })),
            by_api_key: data.by_api_key.map((item) => ({
                id: item.id,
                name: item.name,
                metrics: formatMetrics(item),
            })),
            by_channel: data.by_channel.map((item) => ({
                id: item.id,
                name: item.name,
                metrics: formatMetrics(item),
            })),
        }),
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}

/**
 * 获取分析筛选维度选项
 */
export function useAnalyticsDimensions(): UseQueryResult<AnalyticsDimensions> {
    return useQuery({
        queryKey: ['stats', 'analytics', 'dimensions'],
        queryFn: async () => {
            return apiRequest<AnalyticsDimensions>('/api/v1/stats/analytics/dimensions');
        },
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}
