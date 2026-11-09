import { Button } from '@/components/ui/button';

type InlineRetryProps = {
    /** 重试按钮文案，调用方自行取翻译。 */
    label: string;
    onRetry: () => void | Promise<unknown>;
    /** 重试进行中时禁用，避免重复触发。 */
    isRetrying?: boolean;
    className?: string;
};

/**
 * 卡片内联重试按钮。
 *
 * 改前同一串 `rounded-xl border border-border px-3 py-1.5 font-medium ...` 类名
 * 在首页与分析页 6 个文件里逐字复制了 14 次，这里统一为单一实现，
 * 并复用现有 Button 变体保证 focus / disabled 状态与全站一致。
 */
export function InlineRetry({ label, onRetry, isRetrying = false, className }: InlineRetryProps) {
    return (
        <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void onRetry()}
            disabled={isRetrying}
            aria-busy={isRetrying}
            className={className}
        >
            {label}
        </Button>
    );
}
