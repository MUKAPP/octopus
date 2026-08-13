import { useEffect, useState, useRef } from 'react';
import { useTranslations } from 'use-intl';
import { ScrollText, Calendar, Trash2, FileText } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { useSettingList, useSetSetting, SettingKey } from '@/api/endpoints/setting';
import { useClearLogs } from '@/api/endpoints/log';
import { toast } from '@/components/common/Toast';

const CONTENT_MAX_KIB_MIN = 1;
const CONTENT_MAX_KIB_MAX = 10240;
const CONTENT_MAX_KIB_DEFAULT = 256;

export function SettingLog() {
    const t = useTranslations('setting');
    const { data: settings } = useSettingList();
    const setSetting = useSetSetting();
    const clearLogs = useClearLogs();

    const [enabled, setEnabled] = useState(true);
    const [keepPeriod, setKeepPeriod] = useState('7');
    const [contentEnabled, setContentEnabled] = useState(true);
    const [contentMaxKiB, setContentMaxKiB] = useState(String(CONTENT_MAX_KIB_DEFAULT));
    const [isClearing, setIsClearing] = useState(false);

    const initialEnabled = useRef(true);
    const initialKeepPeriod = useRef('7');
    const initialContentEnabled = useRef(true);
    const initialContentMaxKiB = useRef(String(CONTENT_MAX_KIB_DEFAULT));

    useEffect(() => {
        if (settings) {
            const enabledSetting = settings.find(s => s.key === SettingKey.RelayLogKeepEnabled);
            const periodSetting = settings.find(s => s.key === SettingKey.RelayLogKeepPeriod);
            const contentEnabledSetting = settings.find(s => s.key === SettingKey.RelayLogContentEnabled);
            const contentMaxBytesSetting = settings.find(s => s.key === SettingKey.RelayLogContentMaxBytes);
            if (enabledSetting) {
                const isEnabled = enabledSetting.value === 'true';
                queueMicrotask(() => setEnabled(isEnabled));
                initialEnabled.current = isEnabled;
            }
            if (periodSetting) {
                queueMicrotask(() => setKeepPeriod(periodSetting.value));
                initialKeepPeriod.current = periodSetting.value;
            }
            if (contentEnabledSetting) {
                const isContentEnabled = contentEnabledSetting.value === 'true';
                queueMicrotask(() => setContentEnabled(isContentEnabled));
                initialContentEnabled.current = isContentEnabled;
            }
            if (contentMaxBytesSetting) {
                const bytes = Number(contentMaxBytesSetting.value);
                const kib = Number.isFinite(bytes) && bytes > 0 ? String(Math.round(bytes / 1024)) : String(CONTENT_MAX_KIB_DEFAULT);
                queueMicrotask(() => setContentMaxKiB(kib));
                initialContentMaxKiB.current = kib;
            }
        }
    }, [settings]);

    const handleEnabledChange = (checked: boolean) => {
        setEnabled(checked);
        setSetting.mutate(
            { key: SettingKey.RelayLogKeepEnabled, value: checked ? 'true' : 'false' },
            {
                onSuccess: () => {
                    toast.success(t('saved'));
                    initialEnabled.current = checked;
                }
            }
        );
    };

    const handleKeepPeriodSave = () => {
        if (keepPeriod === initialKeepPeriod.current) return;

        setSetting.mutate(
            { key: SettingKey.RelayLogKeepPeriod, value: keepPeriod },
            {
                onSuccess: () => {
                    toast.success(t('saved'));
                    initialKeepPeriod.current = keepPeriod;
                }
            }
        );
    };

    const handleContentEnabledChange = (checked: boolean) => {
        setContentEnabled(checked);
        setSetting.mutate(
            { key: SettingKey.RelayLogContentEnabled, value: checked ? 'true' : 'false' },
            {
                onSuccess: () => {
                    toast.success(t('saved'));
                    initialContentEnabled.current = checked;
                },
                onError: () => {
                    // mutation 失败时恢复最近一次服务器值
                    setContentEnabled(initialContentEnabled.current);
                    toast.error(t('saveFailed'));
                }
            }
        );
    };

    const handleContentMaxKiBSave = () => {
        const parsed = Number(contentMaxKiB);
        if (!Number.isInteger(parsed) || parsed < CONTENT_MAX_KIB_MIN || parsed > CONTENT_MAX_KIB_MAX) {
            setContentMaxKiB(initialContentMaxKiB.current);
            toast.error(t('contentMaxBytes.invalid'));
            return;
        }
        if (contentMaxKiB === initialContentMaxKiB.current) return;

        // 提交时精确换算成字节
        setSetting.mutate(
            { key: SettingKey.RelayLogContentMaxBytes, value: String(parsed * 1024) },
            {
                onSuccess: () => {
                    toast.success(t('saved'));
                    initialContentMaxKiB.current = contentMaxKiB;
                },
                onError: () => {
                    setContentMaxKiB(initialContentMaxKiB.current);
                    toast.error(t('saveFailed'));
                }
            }
        );
    };

    const handleClearLogs = () => {
        setIsClearing(true);
        clearLogs.mutate(undefined, {
            onSuccess: () => {
                toast.success(t('log.clearSuccess'));
                setIsClearing(false);
            },
            onError: () => {
                toast.error(t('log.clearFailed'));
                setIsClearing(false);
            }
        });
    };

    return (
        <div className="rounded-3xl border border-border bg-card p-6 space-y-5">
            <h2 className="text-lg font-bold text-card-foreground flex items-center gap-2">
                <ScrollText className="h-5 w-5" />
                {t('log.title')}
            </h2>

            {/* 是否启用历史日志 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <ScrollText className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('log.enabled.label')}</span>
                </div>
                <Switch
                    checked={enabled}
                    onCheckedChange={handleEnabledChange}
                />
            </div>

            {/* 历史日志保存范围 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <Calendar className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('log.keepPeriod.label')}</span>
                </div>
                <Input
                    type="number"
                    value={keepPeriod}
                    onChange={(e) => setKeepPeriod(e.target.value)}
                    onBlur={handleKeepPeriodSave}
                    placeholder={t('log.keepPeriod.placeholder')}
                    className="w-48 rounded-xl"
                    disabled={!enabled}
                />
            </div>

            {/* 是否保存请求/响应正文 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <FileText className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('log.contentEnabled.label')}</span>
                </div>
                <Switch
                    checked={contentEnabled}
                    onCheckedChange={handleContentEnabledChange}
                />
            </div>

            {/* 每侧正文上限（KiB） */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <FileText className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('log.contentMaxBytes.label')}</span>
                </div>
                <div className="flex items-center gap-2">
                    <Input
                        type="number"
                        value={contentMaxKiB}
                        onChange={(e) => setContentMaxKiB(e.target.value)}
                        onBlur={handleContentMaxKiBSave}
                        placeholder={t('log.contentMaxBytes.placeholder')}
                        min={CONTENT_MAX_KIB_MIN}
                        max={CONTENT_MAX_KIB_MAX}
                        className="w-48 rounded-xl"
                        disabled={!contentEnabled}
                    />
                    <span className="text-xs text-muted-foreground shrink-0">{t('log.contentMaxBytes.unit')}</span>
                </div>
            </div>

            {/* 清空历史日志 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <Trash2 className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('log.clear.label')}</span>
                </div>
                <Button
                    variant="destructive"
                    size="sm"
                    onClick={handleClearLogs}
                    disabled={isClearing}
                    className="rounded-xl"
                >
                    {isClearing ? t('log.clear.clearing') : t('log.clear.button')}
                </Button>
            </div>
        </div>
    );
}
