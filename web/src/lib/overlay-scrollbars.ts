type Axis = 'x' | 'y';
type RtlScrollType = 'negative' | 'reverse' | 'default';

type Rail = {
  element: HTMLDivElement;
  thumb: HTMLDivElement;
  axis: Axis;
  range: number;
  length: number;
  thumbLength: number;
  events: AbortController;
};

type ParentHost = {
  element: HTMLElement;
  host: HTMLDivElement;
  users: number;
  position: string;
  priority: string;
  ownsPosition: boolean;
};

type Viewport = {
  element: HTMLElement;
  parent: ParentHost;
  x: Rail;
  y: Rail;
  rtl: boolean;
  observed: Set<Element>;
  restoreValue?: () => void;
};

type Drag = {
  viewport: Viewport;
  rail: Rail;
  pointerId: number;
  grab: number;
};

const owners = new WeakMap<Node, HTMLElement>();
const HIT_SIZE = 8;
const GAP = 2;

export function getOverlayScrollbarViewport(target: EventTarget | null): HTMLElement | null {
  if (!(target instanceof Node)) return null;
  for (let node: Node | null = target; node; node = node.parentNode) {
    const owner = owners.get(node);
    if (owner) return owner;
  }
  return null;
}

function hasMarker(element: Element): element is HTMLElement {
  return element instanceof HTMLElement && Array.from(element.classList).some(
    (token) => token === 'scrollbar' || token.endsWith(':scrollbar')
  );
}

function boxParent(element: HTMLElement): HTMLElement | null {
  const popup = element.closest<HTMLElement>('[data-slot="popover-content"], [data-slot="select-content"], [role="menu"]');
  if (popup === element) return element;
  const rect = element.getBoundingClientRect();
  let nearest: HTMLElement | null = null;
  for (let parent = element.parentElement; parent; parent = parent.parentElement) {
    const style = getComputedStyle(parent);
    if (style.display === 'contents') continue;
    nearest ??= parent;
    const parentRect = parent.getBoundingClientRect();
    const scaleX = parent.offsetWidth ? parentRect.width / parent.offsetWidth : 1;
    const scaleY = parent.offsetHeight ? parentRect.height / parent.offsetHeight : 1;
    const rightSpace = scaleX > 0 ? (parentRect.right - rect.right) / scaleX - parseFloat(style.borderRightWidth) : 0;
    const bottomSpace = scaleY > 0 ? (parentRect.bottom - rect.bottom) / scaleY - parseFloat(style.borderBottomWidth) : 0;
    if (
      (parseFloat(style.paddingRight) >= HIT_SIZE + GAP && rightSpace >= HIT_SIZE + GAP) ||
      (parseFloat(style.paddingBottom) >= HIT_SIZE + GAP && bottomSpace >= HIT_SIZE + GAP)
    ) return parent;
    // Keep nested rails within the original scroll/clip boundary.
    if (parent === popup || style.overflowX !== 'visible' || style.overflowY !== 'visible') break;
  }
  return nearest;
}

function clamp(value: number, max: number): number {
  return Math.max(0, Math.min(value, max));
}

function detectRtlScrollType(): RtlScrollType {
  const probe = document.createElement('div');
  const content = document.createElement('div');
  probe.dir = 'rtl';
  probe.style.cssText = 'position:fixed;left:0;top:0;width:4px;height:4px;overflow:scroll;visibility:hidden;contain:strict;';
  content.style.width = '8px';
  probe.append(content);
  document.body.append(probe);
  let type: RtlScrollType = 'default';
  if (probe.scrollLeft === 0) {
    probe.scrollLeft = 1;
    type = probe.scrollLeft === 0 ? 'negative' : 'reverse';
  }
  probe.remove();
  return type;
}

/** Enhances existing native viewports without moving their content, refs, or scroll roots. */
export function installOverlayScrollbars(): () => void {
  const rtlType = detectRtlScrollType();
  const viewports = new Map<HTMLElement, Viewport>();
  const parents = new Map<HTMLElement, ParentHost>();
  const resizeOwners = new Map<Element, Set<Viewport>>();
  const dirty = new Set<Viewport>();
  const animated = new Set<Animation>();
  const internal = new WeakSet<Node>();
  let frame = 0;
  let disposed = false;
  let drag: Drag | null = null;

  const schedule = () => {
    if (!disposed && !frame) frame = requestAnimationFrame(flush);
  };
  const markAll = () => {
    for (const viewport of viewports.values()) dirty.add(viewport);
    schedule();
  };

  const readX = (viewport: Viewport, range = viewport.x.range) => {
    const value = viewport.element.scrollLeft;
    if (!viewport.rtl) return clamp(value, range);
    if (rtlType === 'negative') return clamp(range + value, range);
    if (rtlType === 'reverse') return clamp(range - value, range);
    return clamp(value, range);
  };

  const scroll = (viewport: Viewport, x: number, y: number) => {
    x = clamp(x, viewport.x.range);
    if (viewport.rtl) {
      if (rtlType === 'negative') x -= viewport.x.range;
      else if (rtlType === 'reverse') x = viewport.x.range - x;
    }
    viewport.element.scrollTo({ left: x, top: clamp(y, viewport.y.range), behavior: 'instant' });
    dirty.add(viewport);
    schedule();
  };

  const stopEvent = (event: Event) => {
    event.preventDefault();
    event.stopPropagation();
  };

  const finishDrag = () => {
    const previous = drag;
    drag = null;
    if (!previous) return;
    previous.rail.element.removeAttribute('data-dragging');
    if (previous.rail.element.hasPointerCapture(previous.pointerId)) {
      previous.rail.element.releasePointerCapture(previous.pointerId);
    }
  };

  const moveDrag = (event: PointerEvent) => {
    if (!drag || event.pointerId !== drag.pointerId) return;
    if (event.type === 'pointermove') stopEvent(event);
    const { viewport, rail, grab } = drag;
    const rect = rail.element.getBoundingClientRect();
    const scale = (rail.axis === 'x' ? rect.width : rect.height) / rail.length;
    const travel = (rail.length - rail.thumbLength) * scale;
    if (travel <= 0) return;
    const coordinate = rail.axis === 'x' ? event.clientX - rect.left : event.clientY - rect.top;
    const value = clamp((coordinate - grab * rail.thumbLength * scale) / travel * rail.range, rail.range);
    scroll(viewport, rail.axis === 'x' ? value : readX(viewport), rail.axis === 'y' ? value : viewport.element.scrollTop);
  };

  const endDrag = (event: PointerEvent) => {
    if (!drag || event.pointerId !== drag.pointerId) return;
    event.preventDefault();
    finishDrag();
  };

  const createRail = (viewport: Viewport, axis: Axis): Rail => {
    const element = document.createElement('div');
    const thumb = document.createElement('div');
    element.className = 'overlay-scroll-rail';
    element.dataset.axis = axis;
    element.setAttribute('aria-hidden', 'true');
    thumb.className = 'overlay-scroll-thumb';
    element.append(thumb);
    internal.add(element);
    internal.add(thumb);
    owners.set(element, viewport.element);
    owners.set(thumb, viewport.element);
    const events = new AbortController();
    const rail: Rail = { element, thumb, axis, range: 0, length: 0, thumbLength: 0, events };

    element.addEventListener('pointerdown', (event) => {
      event.preventDefault();
      if (drag || !event.isPrimary || event.button !== 0 || rail.range <= 0) return;
      const rect = thumb.getBoundingClientRect();
      const onThumb = event.target === thumb;
      const size = axis === 'x' ? rect.width : rect.height;
      const coordinate = axis === 'x' ? event.clientX - rect.left : event.clientY - rect.top;
      drag = { viewport, rail, pointerId: event.pointerId, grab: onThumb && size > 0 ? clamp(coordinate / size, 1) : 0.5 };
      element.dataset.dragging = '';
      element.setPointerCapture(event.pointerId);
      if (!onThumb) moveDrag(event);
    }, { signal: events.signal });
    element.addEventListener('pointermove', moveDrag, { signal: events.signal });
    element.addEventListener('pointerup', endDrag, { signal: events.signal });
    element.addEventListener('pointercancel', endDrag, { signal: events.signal });
    element.addEventListener('lostpointercapture', endDrag, { signal: events.signal });
    element.addEventListener('click', (event) => event.preventDefault(), { signal: events.signal });
    element.addEventListener('dblclick', stopEvent, { signal: events.signal });
    element.addEventListener('contextmenu', stopEvent, { signal: events.signal });
    element.addEventListener('wheel', (event) => {
      if (event.ctrlKey) return;
      stopEvent(event);
      const style = getComputedStyle(viewport.element);
      const line = parseFloat(style.lineHeight) || parseFloat(style.fontSize) * 1.2;
      const xUnit = event.deltaMode === WheelEvent.DOM_DELTA_LINE ? line : event.deltaMode === WheelEvent.DOM_DELTA_PAGE ? viewport.element.clientWidth : 1;
      const yUnit = event.deltaMode === WheelEvent.DOM_DELTA_LINE ? line : event.deltaMode === WheelEvent.DOM_DELTA_PAGE ? viewport.element.clientHeight : 1;
      const dx = event.shiftKey && event.deltaX === 0 ? event.deltaY * xUnit : event.deltaX * xUnit;
      const dy = event.shiftKey && event.deltaX === 0 ? 0 : event.deltaY * yUnit;
      scroll(viewport, readX(viewport) + dx, viewport.element.scrollTop + dy);
    }, { passive: false, signal: events.signal });
    return rail;
  };

  const acquireParent = (element: HTMLElement): ParentHost => {
    const existing = parents.get(element);
    if (existing) {
      existing.users++;
      return existing;
    }
    const host = document.createElement('div');
    host.className = 'overlay-scroll-host';
    host.setAttribute('aria-hidden', 'true');
    internal.add(host);
    const parent: ParentHost = {
      element, host, users: 1,
      position: element.style.getPropertyValue('position'),
      priority: element.style.getPropertyPriority('position'),
      ownsPosition: getComputedStyle(element).position === 'static',
    };
    if (parent.ownsPosition) element.style.setProperty('position', 'relative');
    // Preserve the original last child's spacing selectors.
    element.prepend(host);
    parents.set(element, parent);
    return parent;
  };

  const releaseParent = (parent: ParentHost) => {
    if (--parent.users) return;
    parent.host.remove();
    if (parent.ownsPosition && parent.element.style.getPropertyValue('position') === 'relative') {
      if (parent.position) parent.element.style.setProperty('position', parent.position, parent.priority);
      else parent.element.style.removeProperty('position');
    }
    parents.delete(parent.element);
  };

  const resizeObserver = new ResizeObserver((entries) => {
    for (const entry of entries) {
      for (const viewport of resizeOwners.get(entry.target) ?? []) dirty.add(viewport);
    }
    schedule();
  });

  const syncObserved = (viewport: Viewport) => {
    const next = new Set<Element>([viewport.element, viewport.parent.element]);
    for (const child of viewport.element.children) if (!internal.has(child)) next.add(child);
    for (const element of viewport.observed) {
      if (next.has(element)) continue;
      const users = resizeOwners.get(element);
      users?.delete(viewport);
      if (!users?.size) {
        resizeObserver.unobserve(element);
        resizeOwners.delete(element);
      }
    }
    for (const element of next) {
      if (viewport.observed.has(element)) continue;
      let users = resizeOwners.get(element);
      if (!users) {
        users = new Set();
        resizeOwners.set(element, users);
        resizeObserver.observe(element);
      }
      users.add(viewport);
    }
    viewport.observed = next;
  };

  const add = (element: HTMLElement) => {
    if (viewports.has(element) || internal.has(element)) return;
    const parent = boxParent(element);
    if (!parent) return;
    const viewport = { element, parent: acquireParent(parent), rtl: false, observed: new Set<Element>() } as Viewport;
    viewport.x = createRail(viewport, 'x');
    viewport.y = createRail(viewport, 'y');
    viewport.parent.host.append(viewport.x.element, viewport.y.element);
    viewports.set(element, viewport);
    syncObserved(viewport);
    if (element instanceof HTMLTextAreaElement) {
      const ownDescriptor = Object.getOwnPropertyDescriptor(element, 'value');
      const descriptor = ownDescriptor ?? Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value');
      if (descriptor?.get && descriptor.set && descriptor.configurable !== false) {
        const nativeSet = descriptor.set;
        const setValue = function (this: HTMLTextAreaElement, value: string) {
          nativeSet.call(this, value);
          dirty.add(viewport);
          schedule();
        };
        // Delegate to React's existing value tracker; never rewrite value or selection.
        Object.defineProperty(element, 'value', { ...descriptor, set: setValue });
        viewport.restoreValue = () => {
          if (Object.getOwnPropertyDescriptor(element, 'value')?.set !== setValue) return;
          if (ownDescriptor) Object.defineProperty(element, 'value', ownDescriptor);
          else Reflect.deleteProperty(element, 'value');
        };
      }
    }
    dirty.add(viewport);
  };

  const remove = (viewport: Viewport) => {
    if (drag?.viewport === viewport) finishDrag();
    viewport.restoreValue?.();
    for (const rail of [viewport.x, viewport.y]) {
      rail.events.abort();
      owners.delete(rail.element);
      owners.delete(rail.thumb);
      rail.element.remove();
    }
    for (const element of viewport.observed) {
      const users = resizeOwners.get(element);
      users?.delete(viewport);
      if (!users?.size) {
        resizeObserver.unobserve(element);
        resizeOwners.delete(element);
      }
    }
    releaseParent(viewport.parent);
    dirty.delete(viewport);
    viewports.delete(viewport.element);
  };

  const discover = (node: Node) => {
    if (!(node instanceof Element) || internal.has(node)) return;
    if (hasMarker(node)) add(node);
    for (const element of node.querySelectorAll('[class*="scrollbar"]')) if (hasMarker(element)) add(element);
  };

  const collectAnimations = () => {
    for (const animation of document.getAnimations()) {
      const effect = animation.effect;
      const target = effect instanceof KeyframeEffect ? effect.target : null;
      if (!(target instanceof Element) || internal.has(target) || animation.playState !== 'running') continue;
      for (const viewport of viewports.values()) {
        if (target.contains(viewport.element)) {
          animated.add(animation);
          break;
        }
      }
    }
  };

  const measure = (viewport: Viewport) => {
    const { element, parent } = viewport;
    const style = getComputedStyle(element);
    const parentStyle = getComputedStyle(parent.element);
    const rect = element.getBoundingClientRect();
    const parentRect = parent.element.getBoundingClientRect();
    const scaleX = parent.element.offsetWidth ? parentRect.width / parent.element.offsetWidth : 1;
    const scaleY = parent.element.offsetHeight ? parentRect.height / parent.element.offsetHeight : 1;
    const targetScaleX = element.offsetWidth && scaleX ? rect.width / element.offsetWidth / scaleX : 1;
    const targetScaleY = element.offsetHeight && scaleY ? rect.height / element.offsetHeight / scaleY : 1;
    viewport.rtl = style.direction === 'rtl';
    const width = element.clientWidth * targetScaleX;
    const height = element.clientHeight * targetScaleY;
    const left = scaleX ? (rect.left - parentRect.left) / scaleX - parent.element.clientLeft + element.clientLeft * targetScaleX : 0;
    const top = scaleY ? (rect.top - parentRect.top) / scaleY - parent.element.clientTop + element.clientTop * targetScaleY : 0;
    const right = left + width;
    const bottom = top + height;
    const selfHosted = parent.element === element;
    let opacity = selfHosted ? 1 : Number(style.opacity);
    let visible = style.getPropertyValue('--overlay-scrollbar-enabled').trim() === '1' && style.visibility === 'visible' && rect.width > 0 && rect.height > 0 && scaleX > 0 && scaleY > 0;
    for (let ancestor = selfHosted ? null : element.parentElement; ancestor && ancestor !== parent.element; ancestor = ancestor.parentElement) {
      const ancestorStyle = getComputedStyle(ancestor);
      opacity *= Number(ancestorStyle.opacity);
      visible &&= ancestorStyle.visibility === 'visible';
    }
    visible &&= (selfHosted ? Number(style.opacity) : opacity) > 0 && element.getClientRects().length > 0;
    const radius = Math.max(...[style.borderTopLeftRadius, style.borderTopRightRadius, style.borderBottomLeftRadius, style.borderBottomRightRadius].map((value) => parseFloat(value) || 0));
    const inset = Math.max(GAP, Math.min(radius, 12, width / 4, height / 4));
    // Keep rails beside content in existing padding, clear of controls at the outer edge.
    const paddingRight = (parseFloat(style.paddingRight) || 0) * targetScaleX;
    const paddingBottom = (parseFloat(style.paddingBottom) || 0) * targetScaleY;
    const useRightPadding = paddingRight >= HIT_SIZE + GAP;
    const useBottomPadding = paddingBottom >= HIT_SIZE + GAP;
    const outsideX = !useRightPadding && Math.min(parent.element.clientWidth - right, parseFloat(parentStyle.paddingRight) || 0) >= HIT_SIZE + GAP;
    const outsideY = !useBottomPadding && Math.min(parent.element.clientHeight - bottom, parseFloat(parentStyle.paddingBottom) || 0) >= HIT_SIZE + GAP;
    const xRange = Math.max(0, element.scrollWidth - element.clientWidth);
    const yRange = Math.max(0, element.scrollHeight - element.clientHeight);
    const showX = visible && /^(auto|scroll)$/.test(style.overflowX) && xRange > 0;
    const showY = visible && /^(auto|scroll)$/.test(style.overflowY) && yRange > 0;
    const verticalLength = Math.max(0, height - 2 * inset - (showX && !outsideY ? Math.max(HIT_SIZE, paddingBottom - inset) : 0));
    const horizontalLength = Math.max(0, width - 2 * inset - (showY && !outsideX ? Math.max(HIT_SIZE, paddingRight - inset) : 0));
    const railLayout = (rail: Rail, show: boolean, length: number, range: number, client: number, extent: number, x: number, y: number, position: number) => {
      const thumbLength = Math.min(length - Math.min(GAP, length / 2), Math.max(18, length * client / Math.max(client, extent)));
      return { rail, show: show && length > 0, length, range, thumbLength, x, y, position: range ? clamp(position, range) / range * (length - thumbLength) : 0 };
    };
    return {
      viewport, opacity,
      color: style.getPropertyValue('--scrollbar-thumb'),
      hoverColor: style.getPropertyValue('--scrollbar-thumb-hover'),
      rails: [
        railLayout(viewport.x, showX, horizontalLength, xRange, element.clientWidth, element.scrollWidth, left + inset, useBottomPadding ? bottom - paddingBottom + GAP : outsideY ? bottom + GAP : bottom - HIT_SIZE, readX(viewport, xRange)),
        railLayout(viewport.y, showY, verticalLength, yRange, element.clientHeight, element.scrollHeight, useRightPadding ? right - paddingRight + GAP : outsideX ? right + GAP : right - HIT_SIZE, top + inset, element.scrollTop),
      ],
    };
  };

  function flush() {
    frame = 0;
    if (disposed) return;
    for (const animation of animated) {
      if (animation.playState !== 'running') animated.delete(animation);
    }
    if (animated.size) for (const viewport of viewports.values()) dirty.add(viewport);
    const measurements = Array.from(dirty, measure);
    dirty.clear();
    const hostLayouts = new Map<ParentHost, { left: number; top: number; width: number; height: number; radius: string }>();
    for (const { viewport } of measurements) {
      const parent = viewport.parent;
      if (hostLayouts.has(parent)) continue;
      hostLayouts.set(parent, {
        left: parent.element.scrollLeft, top: parent.element.scrollTop,
        width: parent.element.clientWidth, height: parent.element.clientHeight,
        radius: getComputedStyle(parent.element).borderRadius,
      });
    }
    // All native geometry is read before any overlay geometry is written.
    for (const [parent, layout] of hostLayouts) {
      Object.assign(parent.host.style, {
        left: `${layout.left}px`, top: `${layout.top}px`, width: `${layout.width}px`, height: `${layout.height}px`, borderRadius: layout.radius,
      });
    }
    for (const layout of measurements) {
      for (const { rail, show, length, range, thumbLength, x, y, position } of layout.rails) {
        rail.range = range;
        rail.length = length;
        rail.thumbLength = thumbLength;
        if (!show && drag?.rail === rail) finishDrag();
        Object.assign(rail.element.style, {
          display: show ? 'block' : 'none', left: `${x}px`, top: `${y}px`,
          width: `${rail.axis === 'x' ? length : HIT_SIZE}px`, height: `${rail.axis === 'y' ? length : HIT_SIZE}px`,
          opacity: `${layout.opacity}`,
        });
        rail.element.style.setProperty('--scrollbar-thumb', layout.color);
        rail.element.style.setProperty('--scrollbar-thumb-hover', layout.hoverColor);
        Object.assign(rail.thumb.style, {
          width: rail.axis === 'x' ? `${thumbLength}px` : '3px',
          height: rail.axis === 'y' ? `${thumbLength}px` : '3px',
          transform: rail.axis === 'x' ? `translateX(${position}px)` : `translateY(${position}px)`,
        });
      }
    }
    if (animated.size) schedule();
  }

  const mutationObserver = new MutationObserver((records) => {
    let changed = false;
    let structure = false;
    for (const record of records) {
      if (internal.has(record.target)) continue;
      if (record.type === 'childList') {
        const nodes = [...record.addedNodes, ...record.removedNodes];
        if (nodes.every((node) => internal.has(node))) {
          for (const parent of parents.values()) {
            if (parent.element.isConnected && !parent.host.isConnected) {
              parent.element.prepend(parent.host);
              changed = true;
            }
          }
          continue;
        }
        for (const node of record.addedNodes) discover(node);
        structure = true;
      } else if (record.type === 'attributes' && record.attributeName === 'class' && record.target instanceof Element) {
        if (hasMarker(record.target)) add(record.target);
      }
      changed = true;
    }
    if (!changed) return;
    for (const viewport of [...viewports.values()]) {
      if (!viewport.element.isConnected || !hasMarker(viewport.element)) {
        remove(viewport);
        continue;
      }
      if (boxParent(viewport.element) !== viewport.parent.element) {
        const element = viewport.element;
        remove(viewport);
        add(element);
        continue;
      }
      if (structure) syncObserved(viewport);
    }
    collectAnimations();
    markAll();
  });

  const onScroll = (event: Event) => {
    if (event.target instanceof HTMLElement) {
      const viewport = viewports.get(event.target);
      if (viewport) {
        dirty.add(viewport);
        for (const child of viewports.values()) if (viewport.element.contains(child.element)) dirty.add(child);
        schedule();
        return;
      }
    }
    markAll();
  };
  const onInput = (event: Event) => {
    if (!(event.target instanceof HTMLElement)) return;
    for (const viewport of viewports.values()) if (viewport.element.contains(event.target)) dirty.add(viewport);
    schedule();
  };
  const onAnimation = () => {
    collectAnimations();
    markAll();
  };
  const onBlur = () => finishDrag();
  const animationEvents = ['transitionrun', 'transitionend', 'transitioncancel', 'animationstart', 'animationend', 'animationcancel'];

  discover(document.body);
  mutationObserver.observe(document.documentElement, {
    subtree: true, childList: true, characterData: true, attributes: true,
    attributeFilter: ['class', 'style', 'dir', 'hidden', 'open', 'value', 'rows', 'cols', 'wrap'],
  });
  document.addEventListener('scroll', onScroll, true);
  document.addEventListener('input', onInput, true);
  document.addEventListener('change', onInput, true);
  document.addEventListener('load', markAll, true);
  for (const name of animationEvents) document.addEventListener(name, onAnimation, true);
  window.addEventListener('resize', markAll);
  window.addEventListener('blur', onBlur);
  window.visualViewport?.addEventListener('resize', markAll);
  window.visualViewport?.addEventListener('scroll', markAll);
  document.fonts.addEventListener('loadingdone', markAll);
  collectAnimations();
  schedule();

  return () => {
    disposed = true;
    mutationObserver.disconnect();
    resizeObserver.disconnect();
    if (frame) cancelAnimationFrame(frame);
    finishDrag();
    document.removeEventListener('scroll', onScroll, true);
    document.removeEventListener('input', onInput, true);
    document.removeEventListener('change', onInput, true);
    document.removeEventListener('load', markAll, true);
    for (const name of animationEvents) document.removeEventListener(name, onAnimation, true);
    window.removeEventListener('resize', markAll);
    window.removeEventListener('blur', onBlur);
    window.visualViewport?.removeEventListener('resize', markAll);
    window.visualViewport?.removeEventListener('scroll', markAll);
    document.fonts.removeEventListener('loadingdone', markAll);
    for (const viewport of [...viewports.values()]) remove(viewport);
    animated.clear();
    dirty.clear();
    resizeOwners.clear();
  };
}
