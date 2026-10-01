import { useEffect } from 'react';
import { installOverlayScrollbars } from '@/lib/overlay-scrollbars';

export function OverlayScrollbars() {
  useEffect(() => installOverlayScrollbars(), []);
  return null;
}
