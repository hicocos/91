import {
  ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import * as api from "./api";
import { subscribeUnauthorized } from "./authEvents";

type AuthStatus = "loading" | "authed" | "guest" | "unavailable";

type AuthCtx = {
  status: AuthStatus;
  role: string;
  isAdmin: boolean;
  login: (username: string, password: string) => Promise<string | undefined>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
};

const Ctx = createContext<AuthCtx | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [role, setRole] = useState<string>("");

  const refresh = useCallback(async () => {
    setStatus("loading");
    try {
      const r = await api.me();
      setStatus(r.authenticated ? "authed" : "guest");
      setRole(r.authenticated ? r.role ?? "" : "");
    } catch (error) {
      if (error instanceof api.UnauthorizedError) {
        setStatus("guest");
      } else {
        setStatus("unavailable");
      }
      setRole("");
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    return subscribeUnauthorized(() => {
      setStatus("guest");
      setRole("");
    });
  }, []);

  const login = useCallback(async (u: string, p: string) => {
    const result = await api.login(u, p);
    setStatus("authed");
    setRole(result.role ?? "");
    return result.role;
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } finally {
      setStatus("guest");
      setRole("");
    }
  }, []);

  const isAdmin = role === "admin";

  const value = useMemo(
    () => ({ status, role, isAdmin, login, logout, refresh }),
    [status, role, isAdmin, login, logout, refresh]
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth(): AuthCtx {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useAuth must be used inside <AuthProvider>");
  return ctx;
}
