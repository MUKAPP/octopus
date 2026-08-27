import { useEffect, useState, useRef } from 'react';
import { useTranslations } from 'use-intl';
import { Zap, Hash, Timer, TimerOff, HelpCircle } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { useSettingList, useSetSetting, SettingKey } from '@/api/setting';
import { toast } from '@/components/common/Toast';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';

export function SettingCircuitBreaker() {
    const t = useTranslations('setting');
    const { data: settings } = useSettingList();
    const setSetting = useSetSetting();

    const [enabled, setEnabled] = useState(true);
    const [threshold, setThreshold] = useState('');
    const [cooldown, setCooldown] = useState('');
    const [maxCooldown, setMaxCooldown] = useState('');

    const initialThreshold = useRef('');
    const initialCooldown = useRef('');
    const initialMaxCooldown = useRef('');

    useEffect(() => {
        if (settings) {
            const enabledSetting = settings.find(s => s.key === SettingKey.CircuitBreakerEnabled);
            const th = settings.find(s => s.key === SettingKey.CircuitBreakerThreshold);
            const cd = settings.find(s => s.key === SettingKey.CircuitBreakerCooldown);
            const mcd = settings.find(s => s.key === SettingKey.CircuitBreakerMaxCooldown);
            if (enabledSetting) {
                const value = enabledSetting.value !== 'false';
                queueMicrotask(() => setEnabled(value));
            }
            if (th) {
                queueMicrotask(() => setThreshold(th.value));
                initialThreshold.current = th.value;
            }
            if (cd) {
                queueMicrotask(() => setCooldown(cd.value));
                initialCooldown.current = cd.value;
            }
            if (mcd) {
                queueMicrotask(() => setMaxCooldown(mcd.value));
                initialMaxCooldown.current = mcd.value;
            }
        }
    }, [settings]);

    const handleEnabledChange = (value: boolean) => {
        const previousValue = enabled;
        setEnabled(value);
        setSetting.mutate({ key: SettingKey.CircuitBreakerEnabled, value: String(value) }, {
            onSuccess: () => {
                toast.success(t('saved'));
            },
            onError: () => setEnabled(previousValue),
        });
    };

    const handleSave = (key: string, value: string, initialValue: string) => {
        if (value === initialValue) return;

        setSetting.mutate({ key, value }, {
            onSuccess: () => {
                toast.success(t('saved'));
                if (key === SettingKey.CircuitBreakerThreshold) {
                    initialThreshold.current = value;
                } else if (key === SettingKey.CircuitBreakerCooldown) {
                    initialCooldown.current = value;
                } else if (key === SettingKey.CircuitBreakerMaxCooldown) {
                    initialMaxCooldown.current = value;
                }
            }
        });
    };

    return (
        <div className="rounded-3xl border border-border bg-card p-6 space-y-5">
            <h2 className="text-lg font-bold text-card-foreground flex items-center gap-2">
                <Zap className="h-5 w-5" />
                {t('circuitBreaker.title')}
                <TooltipProvider>
                    <Tooltip>
                        <TooltipTrigger asChild>
                            <HelpCircle className="size-4 text-muted-foreground cursor-help" />
                        </TooltipTrigger>
                        <TooltipContent>
                            {t('circuitBreaker.hint')}
                        </TooltipContent>
                    </Tooltip>
                </TooltipProvider>
            </h2>


            <div className="flex items-center justify-between gap-4">
                <div className="text-sm font-medium">{t('circuitBreaker.enabled.label')}</div>
                <Switch checked={enabled} onCheckedChange={handleEnabledChange} disabled={setSetting.isPending} />
            </div>
            {/* 熔断触发阈值 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <Hash className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('circuitBreaker.threshold.label')}</span>
                </div>
                <Input
                    type="number"
                    value={threshold}
                    onChange={(e) => setThreshold(e.target.value)}
                    onBlur={() => handleSave(SettingKey.CircuitBreakerThreshold, threshold, initialThreshold.current)}
                    placeholder={t('circuitBreaker.threshold.placeholder')}
                    disabled={!enabled || setSetting.isPending}
                    className="w-48 rounded-xl"
                />
            </div>

            {/* 基础冷却时间 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <Timer className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('circuitBreaker.cooldown.label')}</span>
                </div>
                <Input
                    type="number"
                    value={cooldown}
                    onChange={(e) => setCooldown(e.target.value)}
                    onBlur={() => handleSave(SettingKey.CircuitBreakerCooldown, cooldown, initialCooldown.current)}
                    placeholder={t('circuitBreaker.cooldown.placeholder')}
                    disabled={!enabled || setSetting.isPending}
                    className="w-48 rounded-xl"
                />
            </div>

            {/* 最大冷却时间 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <TimerOff className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('circuitBreaker.maxCooldown.label')}</span>
                </div>
                <Input
                    type="number"
                    value={maxCooldown}
                    onChange={(e) => setMaxCooldown(e.target.value)}
                    onBlur={() => handleSave(SettingKey.CircuitBreakerMaxCooldown, maxCooldown, initialMaxCooldown.current)}
                    placeholder={t('circuitBreaker.maxCooldown.placeholder')}
                    disabled={!enabled || setSetting.isPending}
                    className="w-48 rounded-xl"
                />
            </div>
        </div>
    );
}
