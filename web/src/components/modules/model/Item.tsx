import { memo, useId, useMemo, useState } from 'react';
import { Pencil, Trash2, ArrowDownToLine, ArrowUpFromLine, Clock } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import { useLocale, useTranslations } from 'use-intl';
import { useDeleteModel, type LLMInfo } from '@/api/model';
import { getModelIcon } from '@/lib/model-icons';
import { toast } from '@/components/common/Toast';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';
import {
    MorphingDialog,
    MorphingDialogContainer,
    MorphingDialogContent,
    MorphingDialogTrigger,
} from '@/components/ui/morphing-dialog';
import { ModelDeleteOverlay, ModelEditDialogContent } from './ItemOverlays';
import { cn } from '@/lib/utils';

interface ModelItemProps {
    model: LLMInfo;
    layout?: 'grid' | 'list';
}

function formatModelPrice(price: number, locale: string): string {
    if (!Number.isFinite(price)) return '—';
    const abs = Math.abs(price);
    const maximumFractionDigits = abs > 0 && abs < 0.01 ? 6 : 4;
    return price.toLocaleString(locale, {
        minimumFractionDigits: 2,
        maximumFractionDigits,
    });
}

export const ModelItem = memo(function ModelItem({ model, layout = 'grid' }: ModelItemProps) {
    const t = useTranslations('model');
    const locale = useLocale();
    const isListLayout = layout === 'list';
    const [confirmDelete, setConfirmDelete] = useState(false);
    const instanceId = useId();
    const deleteLayoutId = `delete-btn-${model.name}-${instanceId}`;
    const deleteModel = useDeleteModel();

    const { Icon: ModelIcon, className: iconClassName, color: brandColor } = useMemo(() => getModelIcon(model.name), [model.name]);
    const ruleCount = model.price_schedule?.rules.length ?? 0;

    const handleDeleteClick = () => {
        setConfirmDelete(true);
    };
    const handleCancelDelete = () => setConfirmDelete(false);
    const handleConfirmDelete = () => {
        deleteModel.mutate(model.name, {
            onSuccess: () => {
                setConfirmDelete(false);
                toast.success(t('toast.deleted'));
            },
            onError: (error) => {
                setConfirmDelete(false);
                toast.error(t('toast.deleteFailed'), { description: error.message });
            }
        });
    };

    return (
        <MorphingDialog>
            <article
                className={cn(
                    'group relative flex h-full items-center gap-3 rounded-3xl border border-border bg-card p-4 transition-shadow',
                    confirmDelete && 'z-50'
                )}
            >
                <ModelIcon aria-hidden="true" className={cn('shrink-0', iconClassName)} width={52} height={52} />

                <div className="flex-1 min-w-0 flex flex-col justify-center gap-1.5">
                    <Tooltip side="top" sideOffset={10} align="start">
                        <TooltipTrigger asChild>
                            <h3 className="flex min-w-0 items-center gap-2 text-base font-semibold leading-tight text-card-foreground">
                                <span className="truncate">{model.name}</span>
                                {isListLayout && ruleCount > 0 && model.price_schedule && (
                                    <span className="inline-flex shrink-0 items-center gap-1 rounded-full border border-primary/20 bg-primary/10 px-2 py-0.5 text-[11px] font-medium leading-none text-primary">
                                        <Clock className="size-3 shrink-0" aria-hidden="true" />
                                        {t('schedule.badge', { count: ruleCount })}
                                    </span>
                                )}
                            </h3>
                        </TooltipTrigger>
                        <TooltipContent key={model.name}>
                            {model.name}
                            {ruleCount > 0 && model.price_schedule
                                ? ` · ${t('schedule.summary', { count: ruleCount })} (${model.price_schedule.timezone})`
                                : null}
                        </TooltipContent>
                    </Tooltip>

                    {!isListLayout && ruleCount > 0 && model.price_schedule && (
                        <div className="flex min-w-0">
                            <span
                                title={model.price_schedule.timezone}
                                className="inline-flex max-w-full items-center gap-1 truncate rounded-full border border-primary/20 bg-primary/10 px-2 py-0.5 text-[11px] font-medium leading-none text-primary"
                            >
                                <Clock className="size-3 shrink-0" aria-hidden="true" />
                                <span className="truncate">{t('schedule.summary', { count: ruleCount })}</span>
                            </span>
                        </div>
                    )}

                    {isListLayout ? (
                        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px] leading-snug text-muted-foreground">
                            <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
                                <ArrowDownToLine className="size-3.5 shrink-0" style={{ color: brandColor }} aria-hidden="true" />
                                <span>{t('card.inputCache')}</span>
                                <span className="tabular-nums">${formatModelPrice(model.input, locale)} / ${formatModelPrice(model.cache_read, locale)}</span>
                            </span>
                            <span className="hidden sm:inline text-muted-foreground/60" aria-hidden="true">|</span>
                            <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
                                <ArrowUpFromLine className="size-3.5 shrink-0" style={{ color: brandColor }} aria-hidden="true" />
                                <span>{t('card.outputCache')}</span>
                                <span className="tabular-nums">${formatModelPrice(model.output, locale)} / ${formatModelPrice(model.cache_write, locale)}</span>
                            </span>
                        </div>
                    ) : (
                        <div className="space-y-1 text-[13px] leading-snug text-muted-foreground">
                            <p className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5">
                                <span className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap">
                                    <ArrowDownToLine className="size-3.5 shrink-0" style={{ color: brandColor }} aria-hidden="true" />
                                    <span>{t('card.inputCache')}</span>
                                </span>
                                <span className="inline-flex flex-wrap items-baseline gap-x-1 tabular-nums">
                                    <span className="whitespace-nowrap">${formatModelPrice(model.input, locale)}</span>
                                    <span className="text-muted-foreground/60">/</span>
                                    <span className="whitespace-nowrap">${formatModelPrice(model.cache_read, locale)}</span>
                                </span>
                            </p>

                            <p className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5">
                                <span className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap">
                                    <ArrowUpFromLine className="size-3.5 shrink-0" style={{ color: brandColor }} aria-hidden="true" />
                                    <span>{t('card.outputCache')}</span>
                                </span>
                                <span className="inline-flex flex-wrap items-baseline gap-x-1 tabular-nums">
                                    <span className="whitespace-nowrap">${formatModelPrice(model.output, locale)}</span>
                                    <span className="text-muted-foreground/60">/</span>
                                    <span className="whitespace-nowrap">${formatModelPrice(model.cache_write, locale)}</span>
                                </span>
                            </p>
                        </div>
                    )}
                </div>

                <div
                    className={cn(
                        isListLayout
                            ? 'shrink-0 flex items-center gap-2 self-center'
                            : 'shrink-0 flex flex-col justify-between gap-2 self-stretch',
                        confirmDelete && 'invisible pointer-events-none'
                    )}
                >
                    <Tooltip side="top" sideOffset={8} align="center">
                        <TooltipTrigger>
                            <MorphingDialogTrigger
                                ariaLabel={t('card.edit')}
                                className="h-9 w-9 flex items-center justify-center rounded-lg bg-muted/60 text-muted-foreground transition-colors motion-reduce:transition-none hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                            >
                                <Pencil className="size-4" aria-hidden="true" />
                            </MorphingDialogTrigger>
                        </TooltipTrigger>
                        <TooltipContent>{t('card.edit')}</TooltipContent>
                    </Tooltip>

                    <Tooltip side="top" sideOffset={8} align="center">
                        <TooltipTrigger asChild>
                            <motion.button
                                layoutId={deleteLayoutId}
                                type="button"
                                onClick={handleDeleteClick}
                                disabled={confirmDelete}
                                className="h-9 w-9 flex items-center justify-center rounded-lg bg-destructive/10 text-destructive transition-colors motion-reduce:transition-none hover:bg-destructive hover:text-destructive-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-50"
                                aria-label={t('card.delete')}
                            >
                                <Trash2 className="size-4" aria-hidden="true" />
                            </motion.button>
                        </TooltipTrigger>
                        <TooltipContent>{t('card.delete')}</TooltipContent>
                    </Tooltip>
                </div>

                <AnimatePresence>
                    {confirmDelete && (
                        <ModelDeleteOverlay
                            layoutId={deleteLayoutId}
                            isPending={deleteModel.isPending}
                            onCancel={handleCancelDelete}
                            onConfirm={handleConfirmDelete}
                        />
                    )}
                </AnimatePresence>
            </article>

            <MorphingDialogContainer>
                <MorphingDialogContent
                    dismissOnClickOutside={false}
                    className="flex min-h-0 min-w-0 w-(--overlay-width) max-w-full max-h-(--overlay-height) flex-col overflow-hidden rounded-3xl bg-card px-4 py-5 text-card-foreground sm:px-6 md:max-w-[624px]"
                >
                    <ModelEditDialogContent model={model} />
                </MorphingDialogContent>
            </MorphingDialogContainer>
        </MorphingDialog>
    );
});
