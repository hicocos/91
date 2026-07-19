import { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { useAuth } from "./AuthContext";
import { safeReturnPath } from "./safeReturnPath";

// 登录守卫：未登录跳 /login，并把安全的站内目的地放到 state，登录后可回跳
export function RequireAuth({ children }: { children: ReactNode }) {
  const { status, refresh } = useAuth();
  const location = useLocation();

  if (status === "loading") {
    return null;
  }

  if (status === "unavailable") {
    return (
      <div className="admin-loading-screen" role="alert">
        <div className="admin-connection-error">
          <p>暂时无法连接服务器，请检查网络后重试。</p>
          <button className="admin-btn is-primary" type="button" onClick={refresh}>
            重试
          </button>
        </div>
      </div>
    );
  }

  if (status === "guest") {
    const from = safeReturnPath(
      location.pathname + location.search + location.hash,
      "/"
    );
    return <Navigate to="/login" replace state={{ from }} />;
  }

  return <>{children}</>;
}
