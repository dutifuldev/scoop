import { useQueryClient } from "@tanstack/react-query";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import { getMe, login as loginRequest, logout as logoutRequest, updateMySettings } from "../api";
import type { AuthUser, LanguageOption, UserSettings } from "../types";

type AuthStatus = "loading" | "authenticated" | "unauthenticated";

interface AuthContextValue {
  status: AuthStatus;
  user: AuthUser | null;
  settings: UserSettings | null;
  languages: LanguageOption[];
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  updateSettings: (payload: Partial<UserSettings>) => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps): JSX.Element {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [user, setUser] = useState<AuthUser | null>(null);
  const [settings, setSettings] = useState<UserSettings | null>(null);
  const [languages, setLanguages] = useState<LanguageOption[]>([]);

  const applyAuthenticatedState = useCallback(
    (next: { user: AuthUser; settings: UserSettings; languages: LanguageOption[] }) => {
      setUser(next.user);
      setSettings(next.settings);
      setLanguages(next.languages);
      setStatus("authenticated");
    },
    [],
  );

  const applyUnauthenticatedState = useCallback(() => {
    setUser(null);
    setSettings(null);
    setLanguages([]);
    setStatus("unauthenticated");
  }, []);

  useEffect(() => {
    let cancelled = false;
    void getMe()
      .then((me) => {
        if (cancelled) {
          return;
        }
        applyAuthenticatedState(me);
      })
      .catch(() => {
        if (cancelled) {
          return;
        }
        applyUnauthenticatedState();
      });

    return () => {
      cancelled = true;
    };
  }, [applyAuthenticatedState, applyUnauthenticatedState]);

  const login = useCallback(
    async (username: string, password: string) => {
      const response = await loginRequest(username, password);
      applyAuthenticatedState(response);
      await queryClient.invalidateQueries();
    },
    [applyAuthenticatedState, queryClient],
  );

  const logout = useCallback(async () => {
    try {
      await logoutRequest();
    } catch {
      // Always clear local auth state even if the request fails.
    }
    applyUnauthenticatedState();
    queryClient.clear();
  }, [applyUnauthenticatedState, queryClient]);

  const updateSettings = useCallback(async (payload: Partial<UserSettings>) => {
    const response = await updateMySettings(payload);
    setSettings(response.settings);
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      settings,
      languages,
      login,
      logout,
      updateSettings,
    }),
    [status, user, settings, languages, login, logout, updateSettings],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext);
  if (!value) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return value;
}
