import type { APIKey } from '@/api/apikey';
import type { Channel } from '@/api/channel';
import type { LLMInfo } from '@/api/model';
import type { StatsAPIKeyFormatted, StatsMetricsFormatted } from '@/api/stats';
import { formatCount, formatMoney } from '@/lib/utils';

export type OpsSeverity = 'error' | 'warning' | 'healthy';

export type OpsInspectorItem =
    | {
        id: string;
        kind: 'channel';
        severity: OpsSeverity;
        channel: Channel;
        reason: 'no_enabled_keys' | 'key_error' | 'key_rate_limit' | 'high_delay' | 'healthy';
        statusCode?: number;
        keyLabel?: string;
        bestDelayMs: number;
        enabledKeyCount: number;
        totalKeyCount: number;
    }
    | {
        id: string;
        kind: 'all_channels_disabled';
        severity: 'warning';
        totalChannelCount: number;
    }
    | {
        id: string;
        kind: 'apikey';
        severity: Exclude<OpsSeverity, 'healthy'>;
        apiKey: APIKey;
        reason: 'expired' | 'quota_exhausted' | 'expiring_soon' | 'quota_near';
        daysLeft?: number;
        usedCost: number;
        maxCost?: number;
        quotaPercent?: number;
    };

export interface GatewayOpsSummary {
    items: OpsInspectorItem[];
    totalChannels: number;
    enabledChannels: number;
    totalApiKeys: number;
    enabledApiKeys: number;
    errorCount: number;
    warningCount: number;
    issueCount: number;
}

export function isPricedModel(model: LLMInfo): boolean {
    return (
        model.input + model.output + model.cache_read + model.cache_write > 0 ||
        model.price_schedule?.rules.some(
            (rule) => rule.input + rule.output + rule.cache_read + rule.cache_write > 0,
        ) === true
    );
}

export function inspectGatewayOps(params: {
    channels: Array<{ raw: Channel; formatted: StatsMetricsFormatted }> | undefined;
    apiKeys: APIKey[] | undefined;
    apiKeyStats: StatsAPIKeyFormatted[] | undefined;
    nowSec: number;
}): GatewayOpsSummary {
    const channelList = params.channels ?? [];
    const apiKeyList = params.apiKeys ?? [];
    const statsByKeyId = new Map((params.apiKeyStats ?? []).map((item) => [item.api_key_id, item]));

    const items: OpsInspectorItem[] = [];
    const enabledChannelRows: Channel[] = [];

    for (const entry of channelList) {
        const channel = entry.raw;
        if (!channel.enabled) continue;
        enabledChannelRows.push(channel);

        const totalKeyCount = channel.keys.length;
        const enabledKeys = channel.keys.filter((k) => k.enabled);
        const measuredDelays = channel.base_urls.map((u) => u.delay).filter((d) => d > 0);
        const bestDelayMs = measuredDelays.length > 0 ? Math.min(...measuredDelays) : 0;

        if (enabledKeys.length === 0) {
            items.push({
                id: `channel-${channel.id}`,
                kind: 'channel',
                severity: 'error',
                channel,
                reason: 'no_enabled_keys',
                bestDelayMs,
                enabledKeyCount: 0,
                totalKeyCount,
            });
            continue;
        }

        const keySlotMap = new Map<number, number>();
        channel.keys.forEach((k, idx) => {
            keySlotMap.set(k.id, idx + 1);
        });

        const criticalKeyIndex = enabledKeys.findIndex(
            (k) => k.status_code === 401 || k.status_code === 403 || k.status_code >= 500,
        );
        if (criticalKeyIndex !== -1) {
            const k = enabledKeys[criticalKeyIndex];
            items.push({
                id: `channel-${channel.id}`,
                kind: 'channel',
                severity: 'error',
                channel,
                reason: 'key_error',
                statusCode: k.status_code,
                keyLabel: k.remark?.trim() || `#${keySlotMap.get(k.id) ?? criticalKeyIndex + 1}`,
                bestDelayMs,
                enabledKeyCount: enabledKeys.length,
                totalKeyCount,
            });
            continue;
        }

        const warningKeyIndex = enabledKeys.findIndex(
            (k) => k.status_code !== 0 && k.status_code !== 200,
        );
        if (warningKeyIndex !== -1) {
            const k = enabledKeys[warningKeyIndex];
            items.push({
                id: `channel-${channel.id}`,
                kind: 'channel',
                severity: 'warning',
                channel,
                reason: 'key_rate_limit',
                statusCode: k.status_code,
                keyLabel: k.remark?.trim() || `#${keySlotMap.get(k.id) ?? warningKeyIndex + 1}`,
                bestDelayMs,
                enabledKeyCount: enabledKeys.length,
                totalKeyCount,
            });
            continue;
        }

        if (bestDelayMs >= 1000) {
            items.push({
                id: `channel-${channel.id}`,
                kind: 'channel',
                severity: 'warning',
                channel,
                reason: 'high_delay',
                bestDelayMs,
                enabledKeyCount: enabledKeys.length,
                totalKeyCount,
            });
            continue;
        }

        items.push({
            id: `channel-${channel.id}`,
            kind: 'channel',
            severity: 'healthy',
            channel,
            reason: 'healthy',
            bestDelayMs,
            enabledKeyCount: enabledKeys.length,
            totalKeyCount,
        });
    }

    if (channelList.length > 0 && enabledChannelRows.length === 0) {
        items.push({
            id: 'all-channels-disabled',
            kind: 'all_channels_disabled',
            severity: 'warning',
            totalChannelCount: channelList.length,
        });
    }

    let enabledApiKeys = 0;
    for (const apiKey of apiKeyList) {
        if (!apiKey.enabled) continue;
        enabledApiKeys += 1;

        const usedCost = statsByKeyId.get(apiKey.id)?.total_cost.raw ?? 0;
        const maxCost = apiKey.max_cost;
        const hasQuota = maxCost !== undefined && maxCost > 0;
        const ratio = hasQuota ? usedCost / maxCost : 0;
        const quotaPercent = hasQuota ? Math.round(ratio * 100) : undefined;

        const hasExpiry = apiKey.expire_at !== undefined && apiKey.expire_at > 0;
        const remainingSec = hasExpiry ? (apiKey.expire_at as number) - params.nowSec : Infinity;
        const isExpired = hasExpiry && remainingSec <= 0;
        const isExpiringSoon = hasExpiry && !isExpired && remainingSec <= 7 * 86400;
        const daysLeft = hasExpiry && !isExpired ? Math.floor(remainingSec / 86400) : undefined;

        if (isExpired) {
            items.push({
                id: `apikey-${apiKey.id}`,
                kind: 'apikey',
                severity: 'error',
                apiKey,
                reason: 'expired',
                usedCost,
                maxCost,
                quotaPercent,
            });
            continue;
        }

        if (hasQuota && ratio >= 1) {
            items.push({
                id: `apikey-${apiKey.id}`,
                kind: 'apikey',
                severity: 'error',
                apiKey,
                reason: 'quota_exhausted',
                usedCost,
                maxCost,
                quotaPercent: Math.max(100, quotaPercent ?? 100),
            });
            continue;
        }

        if (hasQuota && ratio >= 0.8) {
            items.push({
                id: `apikey-${apiKey.id}`,
                kind: 'apikey',
                severity: 'warning',
                apiKey,
                reason: 'quota_near',
                usedCost,
                maxCost,
                quotaPercent,
            });
            continue;
        }

        if (isExpiringSoon) {
            items.push({
                id: `apikey-${apiKey.id}`,
                kind: 'apikey',
                severity: 'warning',
                apiKey,
                reason: 'expiring_soon',
                daysLeft,
                usedCost,
                maxCost,
                quotaPercent,
            });
        }
    }

    const severityOrder: Record<OpsSeverity, number> = {
        error: 0,
        warning: 1,
        healthy: 2,
    };

    items.sort((a, b) => severityOrder[a.severity] - severityOrder[b.severity]);

    const errorCount = items.filter((item) => item.severity === 'error').length;
    const warningCount = items.filter((item) => item.severity === 'warning').length;

    return {
        items,
        totalChannels: channelList.length,
        enabledChannels: enabledChannelRows.length,
        totalApiKeys: apiKeyList.length,
        enabledApiKeys,
        errorCount,
        warningCount,
        issueCount: errorCount + warningCount,
    };
}

export function formatExactOrCompactCount(value: number, locale: string): string {
    if (!Number.isFinite(value) || value <= 0) return '0';
    if (value < 100_000) return Math.round(value).toLocaleString(locale);
    const formatted = formatCount(value).formatted;
    return `${formatted.value}${formatted.unit}`;
}

export interface HomeMoneyDisplay {
    text: string;
    animValue?: string;
    prefixDollar: boolean;
    unitSuffix: string;
}

export function formatHomeMoney(amount: number | undefined): HomeMoneyDisplay {
    const val = amount !== undefined && Number.isFinite(amount) ? Math.max(0, amount) : 0;
    if (val === 0) {
        return {
            text: '$0.00',
            animValue: '0.00',
            prefixDollar: true,
            unitSuffix: '',
        };
    }
    if (val < 0.0001) {
        return {
            text: '<$0.0001',
            prefixDollar: false,
            unitSuffix: '',
        };
    }
    if (val < 0.01) {
        const trimmed = Number(val.toPrecision(2)).toString();
        return {
            text: `$${trimmed}`,
            prefixDollar: false,
            unitSuffix: '',
        };
    }
    const { formatted } = formatMoney(val);
    if (formatted.unit === '$') {
        return {
            text: `$${formatted.value}`,
            animValue: formatted.value,
            prefixDollar: true,
            unitSuffix: '',
        };
    }
    return {
        text: `$${formatted.value}${formatted.unit.replace('$', '')}`,
        animValue: formatted.value,
        prefixDollar: true,
        unitSuffix: formatted.unit.replace('$', ''),
    };
}
