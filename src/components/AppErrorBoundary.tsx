import { Component, type ErrorInfo, type ReactNode } from "react";

type Props = { children: ReactNode };
type State = { failed: boolean };

export class AppErrorBoundary extends Component<Props, State> {
  state: State = { failed: false };

  static getDerivedStateFromError(): State {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("application render failed", error, info);
  }

  render() {
    if (this.state.failed) {
      return (
        <main className="admin-loading-screen" role="alert">
          <p>页面加载失败，请刷新后重试。</p>
          <button type="button" className="admin-btn" onClick={() => window.location.reload()}>
            刷新页面
          </button>
        </main>
      );
    }
    return this.props.children;
  }
}
