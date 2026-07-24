import path from 'node:path';
import { net, protocol } from 'electron';
import { pathToFileURL } from 'node:url';

export function registerAppProtocol(): void {
  const rendererRoot = path.resolve(__dirname, '../renderer');
  protocol.handle('alvax', (request) => {
    const url = new URL(request.url);
    const relativePath = decodeURIComponent(url.pathname).replace(/^\/+/, '');
    const requestedPath = path.resolve(rendererRoot, relativePath || 'main_window/index.html');
    const insideRoot =
      requestedPath === rendererRoot || requestedPath.startsWith(`${rendererRoot}${path.sep}`);
    if (!insideRoot) return new Response('Forbidden', { status: 403 });
    return net.fetch(pathToFileURL(requestedPath).toString());
  });
}
