import type { AlvaxDesktopApi } from '../shared/contracts/api';

declare global {
  interface Window {
    alvax: AlvaxDesktopApi;
  }
}

export {};
