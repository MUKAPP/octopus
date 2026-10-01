const appBaseURL = new URL('./', document.baseURI);

export function getAppBasePath(): string {
    return appBaseURL.pathname;
}

export function resolveAppURL(path: string): string {
    return new URL(path.startsWith('/') ? `.${path}` : path, appBaseURL).href;
}
