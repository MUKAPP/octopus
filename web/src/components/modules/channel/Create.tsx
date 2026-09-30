import { useState } from 'react';
import {
    MorphingDialogClose,
    MorphingDialogTitle,
    MorphingDialogDescription,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import { useCreateChannel, ChannelType, AutoGroupType } from '@/api/channel';
import { useTranslations } from 'use-intl';
import { ChannelForm, type ChannelFormData } from './Form';
import { toast } from '@/components/common/Toast';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import { CHANNEL_PRESETS, type ChannelPreset } from '@/lib/channel-presets';
import { cn } from '@/lib/utils';

export function CreateDialogContent() {
    const { setIsOpen } = useMorphingDialog();
    const createChannel = useCreateChannel();
    const [formData, setFormData] = useState<ChannelFormData>({
        name: '',
        type: ChannelType.OpenAIChat,
        base_urls: [{ url: '', delay: 0 }],
        custom_header: [],
        channel_proxy: '',
        param_override: '',
        param_append: '',
        keys: [{ enabled: true, channel_key: '', remark: '' }],
        model: '',
        custom_model: '',
        auto_sync: false,
        rate_multiplier: 1,
        priority: 0,
        auto_group: AutoGroupType.None,
        enabled: true,
        proxy: false,
        match_regex: '',
    });
    const t = useTranslations('channel.create');

    // 只填充当前草稿: 不改名称(留空时用当前语言的模板名)、不动密钥等其他字段,
    // 且仅当模板提供地址时整组替换地址列表, 空地址保留用户已填的多地址与延迟。
    const applyPreset = (preset: ChannelPreset) => {
        setFormData((current) => ({
            ...current,
            name: current.name.trim() ? current.name : t(`presets.${preset.id}.label`),
            type: preset.type,
            base_urls: preset.baseUrl ? [{ url: preset.baseUrl, delay: 0 }] : current.base_urls,
        }));
    };

    const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        const normalizedBaseUrls = (formData.base_urls ?? []).filter((u) => u.url.trim()).map((u) => ({
            url: u.url.trim(),
            delay: Number(u.delay || 0),
        }));
        const normalizedKeys = formData.keys
            .filter((k) => k.channel_key.trim())
            .map((k) => ({ enabled: k.enabled, channel_key: k.channel_key, remark: k.remark ?? '' }));
        const normalizedHeaders = (formData.custom_header ?? [])
            .map((h) => ({ header_key: h.header_key.trim(), header_value: h.header_value }))
            .filter((h) => h.header_key && h.header_value !== '');

        const channelProxy = formData.channel_proxy.trim();
        const paramOverride = formData.param_override.trim();
        const paramAppend = formData.param_append.trim();
        createChannel.mutate(
            {
                name: formData.name,
                type: formData.type,
                enabled: formData.enabled,
                base_urls: normalizedBaseUrls,
                keys: normalizedKeys,
                model: formData.model,
                custom_model: formData.custom_model,
                proxy: formData.proxy,
                auto_sync: formData.auto_sync,
                rate_multiplier: formData.rate_multiplier,
                priority: formData.priority,
                auto_group: formData.auto_group,
                custom_header: normalizedHeaders,
                channel_proxy: channelProxy,
                param_override: paramOverride,
                param_append: paramAppend,
                match_regex: formData.match_regex.trim(),
            },
            {
                onSuccess: () => {
                    setFormData({
                        name: '',
                        type: ChannelType.OpenAIChat,
                        base_urls: [{ url: '', delay: 0 }],
                        custom_header: [],
                        channel_proxy: '',
                        param_override: '',
                        param_append: '',
                        keys: [{ enabled: true, channel_key: '', remark: '' }],
                        model: '',
                        custom_model: '',
                        auto_sync: false,
                        rate_multiplier: 1,
                        priority: 0,
                        auto_group: AutoGroupType.None,
                        enabled: true,
                        proxy: false,
                        match_regex: '',
                    });
                    setIsOpen(false);
                },
                onError: (error) => {
                    const description = error instanceof Error ? error.message : String(error);
                    toast.error(t('toast.createFailed'), { description });
                }
            });
    };

    return (
        <div className="w-screen max-w-full md:max-w-xl h-full min-h-0 flex flex-col">
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-4 flex items-center justify-between">
                    <h2 className="text-2xl font-bold text-card-foreground">{t('dialogTitle')}</h2>
                    <MorphingDialogClose
                        className="relative right-0 top-0"
                        variants={{
                            initial: { opacity: 0, scale: 0.8 },
                            animate: { opacity: 1, scale: 1 },
                            exit: { opacity: 0, scale: 0.8 }
                        }}
                    />
                </header>
            </MorphingDialogTitle>
            <MorphingDialogDescription disableLayoutAnimation className="flex-1 min-h-0 overflow-auto">
                <Accordion type="single" collapsible className="mb-4 w-full border rounded-xl bg-card">
                    <AccordionItem value="presets" className="border-none">
                        <AccordionTrigger className="text-sm font-medium text-card-foreground py-3 px-4 hover:no-underline hover:bg-muted/30 rounded-xl transition-colors">
                            {t('presetTitle')}
                        </AccordionTrigger>
                        <AccordionContent className="pt-4 px-4 pb-4 space-y-3 border-t">
                            <p className="text-xs text-muted-foreground">{t('presetHint')}</p>
                            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                                {CHANNEL_PRESETS.map((preset) => (
                                    <Button
                                        key={preset.id}
                                        type="button"
                                        variant="outline"
                                        disabled={createChannel.isPending}
                                        onClick={() => applyPreset(preset)}
                                        className="h-auto w-full justify-start gap-3 px-3 py-2 text-left whitespace-normal"
                                    >
                                        <preset.Icon aria-hidden className={cn('size-5 shrink-0', preset.iconClassName)} />
                                        <span className="min-w-0">
                                            <span className="block text-sm font-medium">{t(`presets.${preset.id}.label`)}</span>
                                            <span className="block text-xs font-normal text-muted-foreground">{t(`presets.${preset.id}.description`)}</span>
                                        </span>
                                    </Button>
                                ))}
                            </div>
                        </AccordionContent>
                    </AccordionItem>
                </Accordion>
                <ChannelForm
                    formData={formData}
                    onFormDataChange={setFormData}
                    onSubmit={handleSubmit}
                    isPending={createChannel.isPending}
                    submitText={t('submit')}
                    pendingText={t('submitting')}
                    onCancel={() => setIsOpen(false)}
                    cancelText={t('cancel')}
                    idPrefix="new-channel"
                />
            </MorphingDialogDescription>
        </div>
    );
}
