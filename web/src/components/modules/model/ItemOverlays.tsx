import { useId, useState } from 'react';
import { Loader2, Trash2, X } from 'lucide-react';
import { motion } from 'motion/react';
import { useTranslations } from 'use-intl';
import { useUpdateModel, type LLMInfo } from '@/api/model';
import { Button } from '@/components/ui/button';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import {
    MorphingDialogClose,
    MorphingDialogDescription,
    MorphingDialogTitle,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import { toast } from '@/components/common/Toast';
import { SPRING } from '@/lib/animations/fluid-transitions';
import { PriceScheduleEditor } from './PriceScheduleEditor';
import { fromScheduleDraft, toScheduleDraft, type PriceScheduleDraft } from './price-schedule';

type ModelDeleteOverlayProps = {
    layoutId: string;
    isPending: boolean;
    onCancel: () => void;
    onConfirm: () => void;
};

export function ModelDeleteOverlay({
    layoutId,
    isPending,
    onCancel,
    onConfirm,
}: ModelDeleteOverlayProps) {
    const t = useTranslations('model.overlay');
    return (
        <motion.div
            layoutId={layoutId}
            className="absolute inset-0 flex items-center justify-center gap-3 bg-destructive p-4 rounded-3xl"
            transition={SPRING.morph}
        >
            <button
                type="button"
                onClick={onCancel}
                className="focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring h-9 px-4 flex items-center justify-center gap-1.5 rounded-xl bg-destructive-foreground/20 text-destructive-foreground text-sm font-medium transition-[color,background-color,transform] motion-reduce:transition-none hover:bg-destructive-foreground/30 active:scale-[0.98] motion-reduce:transform-none"
            >
                <X className="size-4" />
                {t('cancel')}
            </button>
            <button
                type="button"
                onClick={onConfirm}
                disabled={isPending}
                className="focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring h-9 px-4 flex items-center justify-center gap-1.5 rounded-xl bg-destructive-foreground text-destructive text-sm font-medium transition-[color,background-color,transform] motion-reduce:transition-none hover:bg-destructive-foreground/90 active:scale-[0.98] motion-reduce:transform-none disabled:opacity-50 disabled:cursor-not-allowed"
            >
                {isPending ? (
                    <Loader2 className="size-4 animate-spin motion-reduce:animate-none" />
                ) : (
                    <Trash2 className="size-4" />
                )}
                {isPending ? t('deleting') : t('confirmDelete')}
            </button>
        </motion.div>
    );
}

export function ModelEditDialogContent({ model }: { model: LLMInfo }) {
    const { setIsOpen } = useMorphingDialog();
    const t = useTranslations('model.overlay');
    const tCreate = useTranslations('model.create');
    const tToast = useTranslations('model.toast');
    const updateModel = useUpdateModel();
    const baseId = useId();

    const [editValues, setEditValues] = useState(() => ({
        input: model.input.toString(),
        output: model.output.toString(),
        cache_read: model.cache_read.toString(),
        cache_write: model.cache_write.toString(),
    }));
    const [schedule, setSchedule] = useState<PriceScheduleDraft | null>(() => toScheduleDraft(model.price_schedule));

    const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        updateModel.mutate({
            name: model.name,
            price_schedule: fromScheduleDraft(schedule),
            input: parseFloat(editValues.input) || 0,
            output: parseFloat(editValues.output) || 0,
            cache_read: parseFloat(editValues.cache_read) || 0,
            cache_write: parseFloat(editValues.cache_write) || 0,
        }, {
            onSuccess: () => {
                setIsOpen(false);
                toast.success(tToast('updated'));
            },
            onError: (error) => {
                toast.error(tToast('updateFailed'), { description: error.message });
            }
        });
    };

    return (
        <>
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-4 flex items-start justify-between gap-3">
                    <div className="min-w-0">
                        <h2 className="truncate text-2xl font-bold text-card-foreground">{model.name}</h2>
                        <p className="mt-0.5 text-xs text-muted-foreground">{t('subtitle')}</p>
                    </div>
                    <MorphingDialogClose
                        className="relative right-0 top-0 shrink-0"
                        variants={{
                            initial: { opacity: 0, scale: 0.8 },
                            animate: { opacity: 1, scale: 1 },
                            exit: { opacity: 0, scale: 0.8 },
                        }}
                    />
                </header>
            </MorphingDialogTitle>
            <form onSubmit={handleSubmit} className="flex min-h-0 min-w-0 flex-1 flex-col">
                <MorphingDialogDescription className="scrollbar -mx-1 flex-1 min-h-0 min-w-0 overflow-y-auto overscroll-contain px-1 py-1">
                    <FieldGroup className="gap-4">
                        <div className="grid grid-cols-1 gap-3 min-[360px]:grid-cols-2 sm:grid-cols-4">
                            <Field>
                                <FieldLabel htmlFor={`${baseId}-input`}>{t('input')}</FieldLabel>
                                <Input
                                    id={`${baseId}-input`}
                                    type="number"
                                    step="any"
                                    min="0"
                                    value={editValues.input}
                                    onChange={(e) => setEditValues({ ...editValues, input: e.target.value })}
                                    className="rounded-xl tabular-nums text-base sm:text-sm"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor={`${baseId}-output`}>{t('output')}</FieldLabel>
                                <Input
                                    id={`${baseId}-output`}
                                    type="number"
                                    step="any"
                                    min="0"
                                    value={editValues.output}
                                    onChange={(e) => setEditValues({ ...editValues, output: e.target.value })}
                                    className="rounded-xl tabular-nums text-base sm:text-sm"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor={`${baseId}-cache-read`}>{t('cacheRead')}</FieldLabel>
                                <Input
                                    id={`${baseId}-cache-read`}
                                    type="number"
                                    step="any"
                                    min="0"
                                    value={editValues.cache_read}
                                    onChange={(e) => setEditValues({ ...editValues, cache_read: e.target.value })}
                                    className="rounded-xl tabular-nums text-base sm:text-sm"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor={`${baseId}-cache-write`}>{t('cacheWrite')}</FieldLabel>
                                <Input
                                    id={`${baseId}-cache-write`}
                                    type="number"
                                    step="any"
                                    min="0"
                                    value={editValues.cache_write}
                                    onChange={(e) => setEditValues({ ...editValues, cache_write: e.target.value })}
                                    className="rounded-xl tabular-nums text-base sm:text-sm"
                                />
                            </Field>
                        </div>
                        <p className="text-xs text-muted-foreground">{tCreate('priceHint')}</p>
                        <PriceScheduleEditor
                            value={schedule}
                            onChange={setSchedule}
                            basePrice={{
                                input: Number(editValues.input) || 0,
                                output: Number(editValues.output) || 0,
                                cache_read: Number(editValues.cache_read) || 0,
                                cache_write: Number(editValues.cache_write) || 0,
                            }}
                        />
                    </FieldGroup>
                </MorphingDialogDescription>
                <div className="mt-3 flex shrink-0 gap-2 border-t border-border/50 pt-4">
                    <Button
                        type="button"
                        variant="secondary"
                        disabled={updateModel.isPending}
                        className="h-11 flex-1 rounded-xl"
                        onClick={() => setIsOpen(false)}
                    >
                        {t('cancel')}
                    </Button>
                    <Button
                        type="submit"
                        disabled={updateModel.isPending}
                        className="h-11 flex-1 rounded-xl"
                    >
                        {updateModel.isPending ? t('saving') : t('save')}
                    </Button>
                </div>
            </form>
        </>
    );
}
