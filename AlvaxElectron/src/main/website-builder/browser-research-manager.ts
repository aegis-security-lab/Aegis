import { BrowserWindow } from 'electron';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

export interface BrowserInspection {
  requestedUrl: string;
  finalUrl: string;
  title: string;
  description: string;
  text: string;
  html: string;
  headings: Array<{ level: number; text: string }>;
  links: Array<{ text: string; href: string }>;
  screenshotPath: string;
}

export class BrowserResearchManager {
  private readonly windows = new Set<BrowserWindow>();

  async inspect(url: string, outputDirectory: string): Promise<BrowserInspection> {
    const parsed = new URL(url);
    if (!['http:', 'https:'].includes(parsed.protocol)) {
      throw new Error('浏览器工具仅支持 http:// 或 https:// 地址。');
    }

    const window = new BrowserWindow({
      show: false,
      width: 1440,
      height: 1000,
      webPreferences: {
        sandbox: true,
        contextIsolation: true,
        nodeIntegration: false,
        offscreen: true,
        javascript: true,
      },
    });
    this.windows.add(window);
    window.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
    window.webContents.on('will-navigate', (event, target) => {
      if (!['http:', 'https:'].includes(new URL(target).protocol)) event.preventDefault();
    });

    try {
      await window.loadURL(parsed.toString(), { userAgent: chromeUserAgent() });
      await waitForRenderedPage(window);
      const page = await window.webContents.executeJavaScript(`(() => {
        const clean = (value) => String(value || '').replace(/\\s+/g, ' ').trim();
        const visible = (element) => {
          const style = getComputedStyle(element);
          const rect = element.getBoundingClientRect();
          return style.display !== 'none' && style.visibility !== 'hidden' && rect.width > 0 && rect.height > 0;
        };
        return {
          title: document.title,
          description: document.querySelector('meta[name="description"]')?.content || '',
          text: clean(document.body?.innerText).slice(0, 50000),
          html: document.documentElement.outerHTML.slice(0, 100000),
          headings: Array.from(document.querySelectorAll('h1,h2,h3,h4,h5,h6'))
            .filter(visible).slice(0, 100).map((node) => ({ level: Number(node.tagName.slice(1)), text: clean(node.textContent) })),
          links: Array.from(document.querySelectorAll('a[href]')).filter(visible).slice(0, 200)
            .map((node) => ({ text: clean(node.textContent), href: node.href })),
          height: Math.min(Math.max(document.documentElement.scrollHeight, 1000), 12000),
        };
      })()` , true) as Omit<BrowserInspection, 'requestedUrl' | 'finalUrl' | 'screenshotPath'> & { height: number };

      window.setContentSize(1440, page.height);
      await delay(150);
      await mkdir(outputDirectory, { recursive: true });
      const screenshotPath = path.join(outputDirectory, `page-${Date.now()}.png`);
      const image = await window.webContents.capturePage();
      await writeFile(screenshotPath, image.toPNG());
      return {
        requestedUrl: parsed.toString(),
        finalUrl: window.webContents.getURL(),
        title: page.title,
        description: page.description,
        text: page.text,
        html: page.html,
        headings: page.headings,
        links: page.links,
        screenshotPath,
      };
    } finally {
      this.windows.delete(window);
      if (!window.isDestroyed()) window.destroy();
    }
  }

  shutdown(): void {
    for (const window of this.windows) if (!window.isDestroyed()) window.destroy();
    this.windows.clear();
  }
}

async function waitForRenderedPage(window: BrowserWindow): Promise<void> {
  await delay(900);
  let previous = -1;
  for (let attempt = 0; attempt < 8; attempt += 1) {
    const length = await window.webContents.executeJavaScript('document.body?.innerText?.length ?? 0', true) as number;
    if (length > 0 && length === previous) return;
    previous = length;
    await delay(350);
  }
}

function chromeUserAgent(): string {
  return 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36';
}

function delay(durationMs: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, durationMs));
}
