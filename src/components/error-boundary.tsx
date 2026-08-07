import * as React from "react"
import { CircleAlert } from "lucide-react"

import { Button } from "@/components/ui/button"

export class ErrorBoundary extends React.Component<
  { children: React.ReactNode; resetKey?: string },
  { error: Error | null }
> {
  state: { error: Error | null } = { error: null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidUpdate(previous: { resetKey?: string }) {
    if (this.props.resetKey !== previous.resetKey && this.state.error) {
      this.setState({ error: null })
    }
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex min-h-64 flex-col items-center justify-center gap-3 p-6 text-center">
          <CircleAlert className="size-6 text-destructive" />
          <p className="text-sm font-medium">页面加载失败</p>
          <p className="max-w-md text-xs leading-5 text-muted-foreground">
            {this.state.error.message}
          </p>
          <Button
            size="sm"
            variant="outline"
            onClick={() => window.location.reload()}
          >
            重新加载
          </Button>
        </div>
      )
    }
    return this.props.children
  }
}
