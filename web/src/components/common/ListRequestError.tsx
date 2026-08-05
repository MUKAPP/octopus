import { AlertCircle, RefreshCw } from 'lucide-react';
import { Button } from '@/components/ui/button';

type ListRequestErrorProps = {
    description: string;
    retryLabel: string;
    onRetry: () => void | Promise<unknown>;
    isRetrying?: boolean;
    variant?: 'empty' | 'banner';
};

export function ListRequestError({
    description,
    retryLabel,
    onRetry,
    isRetrying = false,
    variant = 'empty',
}: ListRequestErrorProps) {
    const content = (
        <>
            <AlertCircle className="size-5 shrink-0 text-destructive" aria-hidden="true" />
            <p className="text-sm text-muted-foreground">{description}</p>
            <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void onRetry()}
                disabled={isRetrying}
                aria-busy={isRetrying}
            >
                <RefreshCw className="size-4" aria-hidden="true" />
                {retryLabel}
            </Button>
        </>
    );

    if (variant === 'banner') {
        return (
            <div
                className="pointer-events-auto absolute right-4 top-4 z-10 flex max-w-[calc(100%-2rem)] items-center gap-3 rounded-xl border border-destructive/30 bg-background/95 px-3 py-2 shadow-sm backdrop-blur-sm"
                role="alert"
            >
                {content}
            </div>
        );
    }

    return (
        <div className="flex max-w-sm flex-col items-center gap-3 text-center" role="alert">
            {content}
        </div>
    );
}
