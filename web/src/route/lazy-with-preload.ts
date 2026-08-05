import { lazy, type ComponentType, type LazyExoticComponent } from 'react';

type PreloadableLazyComponent<T extends ComponentType<Record<string, never>>> =
  LazyExoticComponent<T> & {
    preload: () => Promise<{ default: T }>;
    retry: () => LazyExoticComponent<T>;
  };

export function lazyWithPreload<T extends ComponentType<Record<string, never>>>(
  factory: () => Promise<{ default: T }>,
): PreloadableLazyComponent<T> {
  const createLazyComponent = () => lazy(factory);
  const LazyComponent = createLazyComponent();

  return Object.assign(LazyComponent, {
    preload: factory,
    retry: createLazyComponent,
  });
}
