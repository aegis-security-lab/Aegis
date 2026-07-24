import { Component, StrictMode, type ErrorInfo, type ReactNode } from 'react';
import { createRoot } from 'react-dom/client';
import { CircleAlert, RotateCcw } from 'lucide-react';
import App from './App';
import { Button } from './components/ui/button';
import './styles.css';

document.documentElement.dataset.platform = navigator.userAgent.includes('Mac OS X')
  ? 'darwin'
  : 'other';

interface ErrorBoundaryState {
  error?: Error;
}

class RendererErrorBoundary extends Component<{ children: ReactNode }, ErrorBoundaryState> {
  state: ErrorBoundaryState = {};

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error('[Renderer]', error, info.componentStack);
    void window.alvax.system.logError({
      scope: 'Renderer error boundary',
      message: error.message,
      ...(error.stack ? { stack: `${error.stack}\n${info.componentStack ?? ''}` } : {}),
    });
  }

  render(): ReactNode {
    if (!this.state.error) return this.props.children;
    return (
      <main className="grid h-full place-items-center bg-background p-8 text-foreground">
        <section className="flex max-w-md flex-col items-center gap-4 text-center">
          <div className="grid size-12 place-items-center rounded-xl bg-destructive/10 text-destructive">
            <CircleAlert />
          </div>
          <div className="flex flex-col gap-2">
            <h1 className="text-lg font-semibold">页面加载遇到问题</h1>
            <p className="text-sm leading-6 text-muted-foreground">{this.state.error.message}</p>
          </div>
          <Button onClick={() => window.location.reload()}>
            <RotateCcw data-icon="inline-start" />
            重新加载
          </Button>
        </section>
      </main>
    );
  }
}

window.addEventListener('error', (event) => {
  void window.alvax.system.logError({
    scope: 'Renderer window.error',
    message: event.message,
    ...(event.error instanceof Error && event.error.stack ? { stack: event.error.stack } : {}),
  });
});

window.addEventListener('unhandledrejection', (event) => {
  const error = event.reason instanceof Error ? event.reason : new Error(String(event.reason));
  void window.alvax.system.logError({
    scope: 'Renderer unhandledrejection',
    message: error.message,
    ...(error.stack ? { stack: error.stack } : {}),
  });
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RendererErrorBoundary>
      <App />
    </RendererErrorBoundary>
  </StrictMode>,
);
