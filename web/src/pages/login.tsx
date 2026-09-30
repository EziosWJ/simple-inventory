import { useEffect, useState, type FormEvent } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { isApiError } from "@/lib/api-error";
import { useAuthStore } from "@/store/auth-store";
import type { LoginErrors } from "@/types";
import "./login.css";

const REMEMBERED_USERNAME_KEY = "simple-inventory-remembered-username";

const features = [
  { icon: "purchase", title: "采购管理", description: "高效订货 智能对账" },
  { icon: "sales", title: "销售管理", description: "快速开单 提升业绩" },
  { icon: "inventory", title: "库存管理", description: "实时库存 精准掌控" },
  { icon: "reports", title: "数据报表", description: "多维分析 助力决策" },
];

function Brand() {
  return (
    <div className="login-brand">
      <img src="/login/svg/logo-mark.svg" alt="" width="48" height="48" />
      <span>简单进销存</span>
    </div>
  );
}

export function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const login = useAuthStore((state) => state.login);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [rememberMe, setRememberMe] = useState(false);
  const [errors, setErrors] = useState<LoginErrors>({});
  const [submitting, setSubmitting] = useState(false);
  const [helpMessage, setHelpMessage] = useState("");

  const stateFrom =
    (location.state as { from?: { pathname?: string } } | null)?.from
      ?.pathname;
  const queryRedirect = new URLSearchParams(location.search).get("redirect");
  let from = stateFrom ?? queryRedirect ?? "/dashboard";

  if (from.includes("://") || from.startsWith("//")) {
    from = "/dashboard";
  }

  useEffect(() => {
    try {
      const rememberedUsername = localStorage.getItem(REMEMBERED_USERNAME_KEY);
      if (rememberedUsername) {
        setUsername(rememberedUsername);
        setRememberMe(true);
      }
    } catch {
      // 浏览器禁用本地存储时不影响正常登录。
    }
  }, []);

  if (isAuthenticated) {
    return <Navigate to={from} replace />;
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const nextErrors: LoginErrors = {};

    if (!username.trim()) {
      nextErrors.username = "请输入用户名";
    }
    if (!password) {
      nextErrors.password = "请输入密码";
    }

    if (Object.keys(nextErrors).length > 0) {
      setErrors(nextErrors);
      return;
    }

    try {
      setSubmitting(true);
      await login(username.trim(), password);
      try {
        if (rememberMe) {
          localStorage.setItem(REMEMBERED_USERNAME_KEY, username.trim());
        } else {
          localStorage.removeItem(REMEMBERED_USERNAME_KEY);
        }
      } catch {
        // 本地存储不可用时仍以登录结果为准。
      }
      navigate(from, { replace: true });
    } catch (error) {
      setErrors({
        account: isApiError(error)
          ? error.message
          : "登录失败，请稍后重试",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="login-page">
      <header className="login-header">
        <div className="login-header-brand">
          <Brand />
          <span className="login-tagline">让生意管理更简单</span>
        </div>
        <p className="login-values">简单 · 高效 · 专业 · 成长</p>
      </header>

      <div className="login-content">
        <section className="login-hero" aria-label="产品介绍">
          <div className="login-hero-copy">
            <h1>简单<span>进销存</span></h1>
            <p className="login-hero-subtitle">让企业的采购、销售与库存管理更简单</p>
            <ul className="login-features">
              {features.map((feature) => (
                <li key={feature.icon}>
                  <img src={`/login/svg/feature/${feature.icon}.svg`} alt="" width="52" height="52" />
                  <div>
                    <p>{feature.title}</p>
                    <span>{feature.description}</span>
                  </div>
                </li>
              ))}
            </ul>
          </div>
          <img
            className="login-warehouse"
            src="/login/png/hero-warehouse.png"
            alt="仓储管理场景：仓库、货物、叉车、配送车辆与库存看板"
            width="1448"
            height="1086"
            fetchPriority="high"
          />
          <p className="login-hero-caption">
            <span className="login-caption-dots" aria-hidden="true"><i /><i /><i /></span>
            <span>从混乱到有序，让每一件货物创造更大价值</span>
          </p>
        </section>

        <section className="login-panel" aria-labelledby="login-heading">
          <div className="login-card">
            <Brand />
            <div className="login-card-heading">
              <h2 id="login-heading">欢迎登录</h2>
              <p>高效管理采购、销售与库存</p>
            </div>

            <form className="login-form" onSubmit={handleSubmit} noValidate>
              <div className="login-field">
                <div className="login-input-wrap">
                  <img className="login-input-icon" src="/login/svg/auth/user.svg" alt="" width="26" height="26" />
                  <label htmlFor="username">用户名</label>
                  <Input
                    id="username"
                    name="username"
                    value={username}
                    placeholder="请输入用户名"
                    onChange={(event) => {
                      setUsername(event.target.value);
                      setErrors((current) => ({ ...current, username: undefined, account: undefined }));
                    }}
                    className="login-input"
                    autoComplete="username"
                    required
                    aria-invalid={Boolean(errors.username)}
                    aria-describedby={errors.username ? "username-error" : undefined}
                  />
                </div>
                {errors.username && <p className="login-field-error" id="username-error" role="alert">{errors.username}</p>}
              </div>

              <div className="login-field">
                <div className="login-input-wrap">
                  <img className="login-input-icon" src="/login/svg/auth/lock.svg" alt="" width="26" height="26" />
                  <label htmlFor="password">密码</label>
                  <Input
                    id="password"
                    name="password"
                    type={showPassword ? "text" : "password"}
                    value={password}
                    placeholder="请输入密码"
                    onChange={(event) => {
                      setPassword(event.target.value);
                      setErrors((current) => ({ ...current, password: undefined, account: undefined }));
                    }}
                    className="login-input login-password-input"
                    autoComplete="current-password"
                    required
                    aria-invalid={Boolean(errors.password)}
                    aria-describedby={errors.password ? "password-error" : undefined}
                  />
                  <button
                    type="button"
                    className="login-password-toggle"
                    aria-label={showPassword ? "隐藏密码" : "显示密码"}
                    aria-pressed={showPassword}
                    onClick={() => setShowPassword((value) => !value)}
                  >
                    <img src={`/login/svg/auth/${showPassword ? "eye" : "eye-off"}.svg`} alt="" width="24" height="24" />
                  </button>
                </div>
                {errors.password && <p className="login-field-error" id="password-error" role="alert">{errors.password}</p>}
              </div>

              <div className="login-form-options">
                <label htmlFor="remember-me" className="login-remember">
                  <Checkbox id="remember-me" checked={rememberMe} onChange={(event) => setRememberMe(event.target.checked)} />
                  记住我
                </label>
                <button type="button" className="login-text-button" onClick={() => setHelpMessage("如需重置密码，请联系为你开通账号的系统管理员。")}>忘记密码？</button>
              </div>

              {errors.account && (
                <p className="login-account-error" role="alert">{errors.account}</p>
              )}

              <Button
                type="submit"
                variant="primary"
                size="lg"
                className="login-submit"
                disabled={submitting}
                aria-busy={submitting}
              >
                {submitting ? (
                  <>
                    <span className="h-5 w-5 animate-spin rounded-full border-2 border-white/40 border-t-white motion-reduce:animate-none" aria-hidden />
                    登录中…
                  </>
                ) : (
                  <><span>登 录</span><img src="/login/svg/auth/arrow-right.svg" alt="" width="30" height="30" /></>
                )}
              </Button>
            </form>

            <div className="login-support">
              <p className="login-support-divider"><span>需要帮助？</span></p>
              <button type="button" className="login-text-button login-support-button" onClick={() => setHelpMessage("请联系为你开通账号的系统管理员，获取登录帮助或账号支持。") }>
                <img src="/login/svg/auth/support.svg" alt="" width="26" height="26" />
                联系管理员
              </button>
              {helpMessage && <p className="login-help-message" role="status">{helpMessage}</p>}
            </div>
          </div>
        </section>
      </div>
      <footer className="login-footer">简单进销存 · 专注中小企业的数字化管理工具</footer>
    </main>
  );
}
