import { useState } from 'react';
import { useTranslations } from 'use-intl';
import { ScrollText, Trash2 } from 'lucide-react';
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
    AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { useClearLogs } from '@/api/endpoints/log';
import { toast } from '@/components/common/Toast';

export function SettingLog() {
    const t = useTranslations('setting');
    const tCommon = useTranslations('common');
    const clearLogs = useClearLogs();

    const [isClearing, setIsClearing] = useState(false);

    const handleClearLogs = () => {
        setIsClearing(true);
        clearLogs.mutate(undefined, {
            onSuccess: () => {
                toast.success(t('log.clearSuccess'));
                setIsClearing(false);
            },
            onError: (error) => {
                toast.error(t('log.clearFailed'), {
                    description: error instanceof Error ? error.message : String(error),
                });
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

            {/* 清空历史日志 */}
            <div className="flex flex-wrap items-center justify-between gap-3 sm:gap-4">
                <div className="flex items-center gap-3">
                    <Trash2 className="h-5 w-5 shrink-0 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('log.clear.label')}</span>
                </div>
                <AlertDialog>
                    <AlertDialogTrigger asChild>
                        <Button
                            variant="destructive"
                            size="sm"
                            disabled={isClearing}
                            className="rounded-xl"
                        >
                            {isClearing ? t('log.clear.clearing') : t('log.clear.button')}
                        </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                        <AlertDialogHeader>
                            <AlertDialogTitle>{t('log.clear.confirm.title')}</AlertDialogTitle>
                            <AlertDialogDescription>{t('log.clear.confirm.description')}</AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                            <AlertDialogCancel>{tCommon('cancel')}</AlertDialogCancel>
                            <AlertDialogAction variant="destructive" onClick={handleClearLogs}>
                                {t('log.clear.confirm.confirm')}
                            </AlertDialogAction>
                        </AlertDialogFooter>
                    </AlertDialogContent>
                </AlertDialog>
            </div>
        </div>
    );
}
