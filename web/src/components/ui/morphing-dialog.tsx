import React, {
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  motion,
  AnimatePresence,
  MotionConfig,
  Transition,
  Variant,
} from 'motion/react';
import { createPortal } from 'react-dom';
import { cn } from '@/lib/utils';
import { XIcon } from 'lucide-react';
import useClickOutside from '@/hooks/useClickOutside';

export type MorphingDialogContextType = {
  isOpen: boolean;
  setIsOpen: React.Dispatch<React.SetStateAction<boolean>>;
  uniqueId: string;
  triggerRef: React.RefObject<HTMLDivElement | null>;
};

const MorphingDialogContext =
  React.createContext<MorphingDialogContextType | null>(null);

function useMorphingDialog() {
  const context = useContext(MorphingDialogContext);
  if (!context) {
    throw new Error(
      'useMorphingDialog must be used within a MorphingDialogProvider'
    );
  }
  return context;
}

export type MorphingDialogProviderProps = {
  children: React.ReactNode;
  transition?: Transition;
};

function MorphingDialogProvider({
  children,
  transition,
}: MorphingDialogProviderProps) {
  const [isOpen, setIsOpen] = useState(false);
  const uniqueId = useId();
  const triggerRef = useRef<HTMLDivElement>(null!);

  const contextValue = useMemo(
    () => ({
      isOpen,
      setIsOpen,
      uniqueId,
      triggerRef,
    }),
    [isOpen, uniqueId]
  );

  return (
    <MorphingDialogContext.Provider value={contextValue}>
      <MotionConfig transition={transition}>{children}</MotionConfig>
    </MorphingDialogContext.Provider>
  );
}

export type MorphingDialogProps = {
  children: React.ReactNode;
  transition?: Transition;
};

function MorphingDialog({ children, transition }: MorphingDialogProps) {
  return (
    <MorphingDialogProvider>
      <MotionConfig transition={transition}>{children}</MotionConfig>
    </MorphingDialogProvider>
  );
}

export type MorphingDialogTriggerProps = {
  children: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
  triggerRef?: React.RefObject<HTMLDivElement>;
  ariaLabel?: string;
};

function MorphingDialogTrigger({
  children,
  className,
  style,
  triggerRef: triggerRefProp,
  ariaLabel,
}: MorphingDialogTriggerProps) {
  const { setIsOpen, isOpen, uniqueId, triggerRef } = useMorphingDialog();

  const handleClick = useCallback(() => {
    setIsOpen(!isOpen);
  }, [isOpen, setIsOpen]);

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        setIsOpen(!isOpen);
      }
    },
    [isOpen, setIsOpen]
  );

  // Important: when dialog is open, framer-motion shared-layout can temporarily
  // "flash" the trigger back into its original position during internal re-layouts.
  // To make this robust, we render a non-motion placeholder (still in layout flow)
  // instead of the motion trigger while open.
  if (isOpen) {
    return (
      <div
        ref={triggerRefProp ?? triggerRef}
        className={cn('relative', className)}
        style={{ ...style, visibility: 'hidden', pointerEvents: 'none' }}
        aria-hidden
      >
        {children}
      </div>
    );
  }

  return (
    <motion.div
      ref={triggerRefProp ?? triggerRef}
      layoutId={`dialog-${uniqueId}`}
      className={cn('relative cursor-pointer', className)}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      style={style}
      aria-haspopup='dialog'
      aria-expanded={isOpen}
      aria-controls={`motion-ui-morphing-dialog-content-${uniqueId}`}
      aria-label={ariaLabel}
      role='button'
      tabIndex={0}
    >
      {children}
    </motion.div>
  );
}

/**
 * 嵌套浮层原语：浮层渲染到 document.body，脱离祖先滚动/裁切容器，层级在页面
 * 与已打开对话框之上。宿主通过**明确的归属关联**（自己内部是否有浮层打开）
 * 让出关闭与焦点处理，不使用全局“任意浮层打开即忽略所有关闭”的判定。
 */
const FLOATING_LAYER_SELECTOR =
  '[data-slot="popover-content"], [data-slot="select-content"]';

/** Radix 浮层触发器打开时带 aria-expanded，用于不 portal 时判断弹层是否打开 */
const OPEN_FLOATING_TRIGGER_SELECTOR =
  '[data-slot="popover-trigger"][aria-expanded="true"], [data-slot="select-trigger"][aria-expanded="true"]';

const FOCUSABLE_SELECTOR =
  'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

/** 由 owner 内部触发器 aria-controls 指向的弹层元素（如日期选择面板） */
function getOwnedPopups(owner: HTMLElement | null): HTMLElement[] {
  if (!owner) return [];
  const popups: HTMLElement[] = [];
  owner.querySelectorAll('[aria-controls]').forEach((trigger) => {
    const popupId = trigger.getAttribute('aria-controls');
    const popup = popupId ? document.getElementById(popupId) : null;
    if (popup?.matches(FLOATING_LAYER_SELECTOR)) popups.push(popup);
  });
  return popups;
}

/** owner 内部的可聚焦元素 */
function collectFocusables(owner: HTMLElement | null): HTMLElement[] {
  if (!owner) return [];
  return Array.from(owner.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
    .filter((element) => !element.matches(':disabled') && element.getClientRects().length > 0);
}


/**
 * owner 内部是否有自有弹层（日期面板等）处于打开状态。
 * 打开期间“外部点击”的真实目标在 owner 之外，需忽略以免误关上级浮层。
 */
function hasOwnedOpenPopup(owner: HTMLElement | null): boolean {
  if (!owner) return false;
  if (getOwnedPopups(owner).length > 0) return true;
  return Boolean(owner.querySelector(OPEN_FLOATING_TRIGGER_SELECTOR));
}

/**
 * 在首个/末个可聚焦元素间的 Tab 收尾处循环。
 * contain 为真时，焦点落在集合之外也拉回集合内（浮层用，避免焦点跑到宿主对话框）。
 */
function cycleFocus(
  event: KeyboardEvent,
  focusables: HTMLElement[],
  contain = false
): void {
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  if (!first || !last) return;
  const active = document.activeElement;
  if (contain && (!active || !focusables.includes(active as HTMLElement))) {
    event.preventDefault();
    (event.shiftKey ? last : first).focus();
    return;
  }
  if (event.shiftKey ? active === first : active === last) {
    event.preventDefault();
    (event.shiftKey ? last : first).focus();
  }
}

const noop = () => {};

/** 已打开浮层的后进先出顺序，仅最上层浮层响应 Escape/Tab */
const openLayerStack: string[] = [];

/** 宿主对话框与内部浮层共享的层级状态 */
type NestedLayerContextValue = {
  /** 是否处于对话框宿主内；页面内的浮层为 false */
  enabled: boolean;
  register: () => void;
  unregister: () => void;
};

const NestedLayerContext = React.createContext<NestedLayerContextValue>({
  enabled: false,
  register: noop,
  unregister: noop,
});

export type MorphingDialogContentProps = {
  children: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
  dismissOnClickOutside?: boolean;
};

/** 宿主对话框内部的嵌套浮层登记（对话框内的浮层用） */
function useNestedLayerContextValue(): {
  value: NestedLayerContextValue;
  isAnyOpen: () => boolean;
} {
  const openCountRef = useRef(0);

  const isAnyOpen = useCallback(() => openCountRef.current > 0, []);
  // 登记只改 ref，不触发宿主重渲染，避免浮层开关引起对话框子树刷新
  const value = useMemo<NestedLayerContextValue>(
    () => ({
      enabled: true,
      register: () => {
        openCountRef.current += 1;
      },
      unregister: () => {
        openCountRef.current -= 1;
      },
    }),
    []
  );

  return useMemo(() => ({ value, isAnyOpen }), [value, isAnyOpen]);
}

function MorphingDialogContent({
  children,
  className,
  style,
  dismissOnClickOutside = true,
}: MorphingDialogContentProps) {
  const { setIsOpen, isOpen, uniqueId, triggerRef } = useMorphingDialog();
  const containerRef = useRef<HTMLDivElement>(null!);
  const layerContext = useNestedLayerContextValue();

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      // 对话框内部有浮层打开时，Escape 关闭与 Tab 循环由该浮层负责
      if (event.defaultPrevented || layerContext.isAnyOpen()) return;
      if (event.key === 'Escape') {
        setIsOpen(false);
        return;
      }
      if (event.key === 'Tab') {
        cycleFocus(event, collectFocusables(containerRef.current));
      }
    };

    document.addEventListener('keydown', handleKeyDown);

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [setIsOpen, layerContext]);

  useEffect(() => {
    if (isOpen) {
      document.body.classList.add('overflow-hidden');
      collectFocusables(containerRef.current)[0]?.focus();
    } else {
      document.body.classList.remove('overflow-hidden');
      triggerRef.current?.focus();
    }
  }, [isOpen, triggerRef]);

  useClickOutside(
    containerRef,
    () => {
      if (isOpen && dismissOnClickOutside) {
        setIsOpen(false);
      }
    },
    () => {
      // 只让出给“自己内部”打开的浮层；不使用全局任意浮层的判定
      return (
        layerContext.isAnyOpen() || hasOwnedOpenPopup(containerRef.current)
      );
    }
  );

  return (
    <NestedLayerContext.Provider value={layerContext.value}>
      <motion.div
        ref={containerRef}
        id={`motion-ui-morphing-dialog-content-${uniqueId}`}
        layoutId={`dialog-${uniqueId}`}
        className={cn('overflow-hidden', className)}
        style={style}
        role='dialog'
        aria-modal='true'
        aria-labelledby={`motion-ui-morphing-dialog-title-${uniqueId}`}
        aria-describedby={`motion-ui-morphing-dialog-description-${uniqueId}`}
      >
        {children}
      </motion.div>
    </NestedLayerContext.Provider>
  );
}

export type MorphingDialogOverlayLayerProps = {
  children: React.ReactNode;
  /** 与触发元素共享的 layoutId，保持展开/折叠 morph 动画 */
  layoutId: string;
  /** morph 动画参数，须与触发元素侧一致 */
  transition?: Transition;
  /** 最上层浮层按下 Escape 时调用；表单调用方保留提交期间的关闭约束。 */
  onClose: () => void;
  /** 浮层面板类名（宽度、内边距等；圆角/边框/背景/滚动已由原语提供） */
  panelClassName?: string;
};

/**
 * 视口居中的共享浮层面板：渲染到 document.body，脱离祖先滚动/裁切容器，
 * 面板高度受视口约束、自身可滚动并带细滚动条（.scrollbar）。
 * 仅最上层浮层响应 Escape/Tab；浮层内部打开的日期面板等自有弹层优先处理键盘与指针事件。
 * 打开时聚焦面板内首个可聚焦元素，关闭时把焦点归还给打开前的元素。
 */
function MorphingDialogOverlayLayer({
  children,
  layoutId,
  transition,
  onClose,
  panelClassName,
}: MorphingDialogOverlayLayerProps) {
  const host = useContext(NestedLayerContext);
  const panelRef = useRef<HTMLDivElement>(null!);
  const layerId = useId();
  // 在首次渲染时捕获触发器；同一次提交可能会禁用该按钮并移走焦点。
  const [restoreFocusTarget] = useState(() =>
    document.activeElement instanceof HTMLElement ? document.activeElement : null
  );

  // 处于对话框内时登记，宿主据此让出 Escape/Tab/外部点击处理
  useEffect(() => {
    if (!host.enabled) return;
    host.register();
    return host.unregister;
  }, [host]);

  // 入栈顺序决定谁是最上层；卸载时把焦点归还触发元素
  useEffect(() => {
    const target = restoreFocusTarget;
    openLayerStack.push(layerId);
    return () => {
      const index = openLayerStack.indexOf(layerId);
      if (index >= 0) openLayerStack.splice(index, 1);
      if (!target || !document.contains(target)) return;
      // 若用户已经主动转移焦点，不抢回触发器。
      if (document.activeElement && document.activeElement !== document.body) return;
      target.focus();
    };
  }, [layerId, restoreFocusTarget]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || openLayerStack[openLayerStack.length - 1] !== layerId) return;
      // 自有弹层（如日期选择）打开时，Escape/Tab 交给它自己处理
      if (hasOwnedOpenPopup(panelRef.current)) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key === 'Tab') {
        cycleFocus(event, collectFocusables(panelRef.current), true);
      }
    };

    document.addEventListener('keydown', handleKeyDown);

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [onClose, layerId]);

  // 打开后把焦点移入浮层
  useEffect(() => {
    collectFocusables(panelRef.current)[0]?.focus();
  }, []);


  return createPortal(
    <div className='pointer-events-none fixed inset-0 z-[60]'>
      <div className='flex h-full items-center justify-center pt-(--overlay-top) pr-(--overlay-right) pb-(--overlay-bottom) pl-(--overlay-left)'>
        <motion.div
          ref={panelRef}
          layoutId={layoutId}
          transition={transition}
          className={cn(
            'pointer-events-auto max-h-full overflow-y-auto scrollbar rounded-3xl border border-border bg-card outline-none',
            panelClassName
          )}
        >
          {children}
        </motion.div>
      </div>
    </div>,
    document.body
  );
}

export type MorphingDialogContainerProps = {
  children: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
};

function MorphingDialogContainer({ children, className, style }: MorphingDialogContainerProps) {
  const { isOpen, uniqueId } = useMorphingDialog();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    // Schedule state update for next tick to avoid synchronous update warning
    const timer = setTimeout(() => setMounted(true), 0);
    return () => {
      clearTimeout(timer);
      setMounted(false);
    };
  }, []);

  if (!mounted) return null;

  return createPortal(
    <AnimatePresence initial={false} mode='sync'>
      {isOpen && (
        <>
          <motion.div
            key={`backdrop-${uniqueId}`}
            className='fixed inset-0 bg-white/40 backdrop-blur-xs dark:bg-black/40 z-50'
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
          />
          <div
            className={cn(
              'fixed inset-0 z-50 flex min-h-0 items-center justify-center overflow-clip',
              'pt-(--overlay-top) pr-(--overlay-right) pb-(--overlay-bottom) pl-(--overlay-left)',
              className
            )}
            style={style}
          >
            {children}
          </div>
        </>
      )}
    </AnimatePresence>,
    document.body
  );
}

export type MorphingDialogTitleProps = {
  children: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
};

function MorphingDialogTitle({
  children,
  className,
  style,
}: MorphingDialogTitleProps) {
  const { uniqueId } = useMorphingDialog();

  return (
    <motion.div
      id={`motion-ui-morphing-dialog-title-${uniqueId}`}
      layoutId={`dialog-title-container-${uniqueId}`}
      className={className}
      style={style}
      layout
    >
      {children}
    </motion.div>
  );
}

export type MorphingDialogSubtitleProps = {
  children: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
};

function MorphingDialogSubtitle({
  children,
  className,
  style,
}: MorphingDialogSubtitleProps) {
  const { uniqueId } = useMorphingDialog();

  return (
    <motion.div
      layoutId={`dialog-subtitle-container-${uniqueId}`}
      className={className}
      style={style}
    >
      {children}
    </motion.div>
  );
}

export type MorphingDialogDescriptionProps = {
  children: React.ReactNode;
  className?: string;
  disableLayoutAnimation?: boolean;
  variants?: {
    initial: Variant;
    animate: Variant;
    exit: Variant;
  };
};

function MorphingDialogDescription({
  children,
  className,
  variants,
  disableLayoutAnimation,
}: MorphingDialogDescriptionProps) {
  const { uniqueId } = useMorphingDialog();

  return (
    <motion.div
      key={`dialog-description-${uniqueId}`}
      layoutId={
        disableLayoutAnimation
          ? undefined
          : `dialog-description-content-${uniqueId}`
      }
      variants={variants}
      className={className}
      initial='initial'
      animate='animate'
      exit='exit'
      id={`dialog-description-${uniqueId}`}
    >
      {children}
    </motion.div>
  );
}

export type MorphingDialogImageProps = {
  src: string;
  alt: string;
  className?: string;
  style?: React.CSSProperties;
};

function MorphingDialogImage({
  src,
  alt,
  className,
  style,
}: MorphingDialogImageProps) {
  const { uniqueId } = useMorphingDialog();

  return (
    <motion.img
      src={src}
      alt={alt}
      className={cn(className)}
      layoutId={`dialog-img-${uniqueId}`}
      style={style}
    />
  );
}

export type MorphingDialogCloseProps = {
  children?: React.ReactNode;
  className?: string;
  variants?: {
    initial: Variant;
    animate: Variant;
    exit: Variant;
  };
};

function MorphingDialogClose({
  children,
  className,
  variants,
}: MorphingDialogCloseProps) {
  const { setIsOpen, uniqueId } = useMorphingDialog();

  const handleClose = useCallback(() => {
    setIsOpen(false);
  }, [setIsOpen]);

  return (
    <motion.button
      onClick={handleClose}
      type='button'
      aria-label='Close dialog'
      key={`dialog-close-${uniqueId}`}
      className={cn('absolute top-6 right-6', className)}
      initial='initial'
      animate='animate'
      exit='exit'
      variants={variants}
    >
      {children || <XIcon size={24} />}
    </motion.button>
  );
}

export {
  MorphingDialog,
  MorphingDialogTrigger,
  MorphingDialogContainer,
  MorphingDialogContent,
  MorphingDialogOverlayLayer,
  MorphingDialogClose,
  MorphingDialogTitle,
  MorphingDialogSubtitle,
  MorphingDialogDescription,
  MorphingDialogImage,
  useMorphingDialog,
};
