const CODEX_MUX_API = "http://127.0.0.1:__CODEX_MUX_CONTROL_PORT__/v1";
const CODEX_MUX_TOKEN = "__CODEX_MUX_CONTROL_TOKEN__";
let codexMuxLoginActive = false;

function CodexMuxProfileMenuOpenChange(setOpen) {
  return (nextOpen) => {
    if (!nextOpen && codexMuxLoginActive) return;
    setOpen(nextOpen);
  };
}

async function codexMuxRequest(path, options = {}) {
  const response = await fetch(`${CODEX_MUX_API}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      "X-Codex-Mux-Token": CODEX_MUX_TOKEN,
      ...options.headers,
    },
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `リクエストに失敗しました (${response.status})`);
  return body;
}

const CODEX_MUX_ACCOUNT_SCOPED_PLUGIN_METHODS = new Set([
  "list-apps",
  "list-installed-apps",
  "read-apps",
  "list-mcp-server-status",
  "login-mcp-server",
]);

function codexMuxScopePluginRequest(method, params) {
  const accountId = globalThis.__codexMuxPluginAccountId;
  if (
    !accountId ||
    !CODEX_MUX_ACCOUNT_SCOPED_PLUGIN_METHODS.has(method) ||
    (params != null &&
      (typeof params !== "object" || Array.isArray(params)))
  ) {
    return params;
  }
  return { ...(params || {}), codexMuxAccountId: accountId };
}

async function codexMuxProfileData(accountId = null) {
  const query = accountId
    ? `?accountId=${encodeURIComponent(accountId)}`
    : "";
  const result = await codexMuxRequest(`/profile/combined${query}`);
  globalThis.__codexMuxCombinedProfileAccounts = result.accounts || [];
  return result.profile;
}

async function codexMuxRateLimitResets(accountId) {
  return codexMuxRequest(
    `/accounts/${encodeURIComponent(accountId)}/rate-limit-resets`,
  );
}

async function codexMuxConsumeRateLimitReset(accountId, input) {
  return codexMuxRequest(
    `/accounts/${encodeURIComponent(accountId)}/rate-limit-resets/consume`,
    {
      method: "POST",
      body: JSON.stringify({
        creditId: input.creditId ?? null,
        redeemRequestId: input.redeemRequestId,
      }),
    },
  );
}

function CodexMuxUseResetAccountState() {
  const cachedAccounts = (globalThis.__codexMuxConnectedAccounts || []).filter(
    (account) => account.connected && account.enabled,
  );
  const [accounts, setAccounts] = __CODEX_MUX_REACT__.useState(cachedAccounts);
  const [selectedId, setSelectedId] = __CODEX_MUX_REACT__.useState("primary");
  const [resetCounts, setResetCounts] = __CODEX_MUX_REACT__.useState({});
  const [loading, setLoading] = __CODEX_MUX_REACT__.useState(cachedAccounts.length === 0);

  const loadAccounts = __CODEX_MUX_REACT__.useCallback(async () => {
    const result = await codexMuxRequest("/accounts");
    const connected = (result.accounts || []).filter(
      (account) => account.connected && account.enabled,
    );
    setAccounts(connected);
    setSelectedId((current) =>
      connected.some((account) => account.id === current)
        ? current
        : connected[0]?.id || "primary",
    );
    setLoading(false);
    const entries = await Promise.all(
      connected.map(async (account) => {
        try {
          const resets = await codexMuxRateLimitResets(account.id);
          return [account.id, Math.max(0, resets.available_count || 0)];
        } catch {
          return [account.id, null];
        }
      }),
    );
    setResetCounts(Object.fromEntries(entries));
  }, []);

  __CODEX_MUX_REACT__.useEffect(() => {
    loadAccounts().catch(() => setLoading(false));
  }, [loadAccounts]);

  __CODEX_MUX_REACT__.useEffect(
    () => () => {
      delete window.__codexMuxResetAccountId;
      delete window.__codexMuxSelectedUsageWindows;
      delete window.__codexMuxResetAccountSelector;
    },
    [],
  );

  const selected =
    accounts.find((account) => account.id === selectedId) || accounts[0] || null;
  const activeId = selected?.id || selectedId;
  window.__codexMuxResetAccountId = activeId;
  window.__codexMuxSelectedUsageWindows = selected
    ? codexMuxUsageWindows(selected.rateLimits)
    : null;
  window.__codexMuxResetAccountSelector = (0, __CODEX_MUX_JSX__.jsx)(
    CodexMuxResetAccountSelector,
    {
      accounts,
      loading,
      resetCounts,
      selectedId: activeId,
      onSelect: setSelectedId,
    },
  );

}

function CodexMuxResetAccountSelector({
  accounts,
  loading,
  onSelect,
  resetCounts,
  selectedId,
}) {
  return (0, __CODEX_MUX_JSX__.jsxs)("div", {
    className: "pt-4",
    children: [
      (0, __CODEX_MUX_JSX__.jsx)("div", {
        className:
          "mb-2 px-1 text-xs font-medium text-token-text-secondary",
        children: "サブスクリプション",
      }),
      (0, __CODEX_MUX_JSX__.jsx)("div", {
        className:
          "flex flex-wrap gap-2 rounded-2xl border border-token-border p-2",
        children: loading
          ? (0, __CODEX_MUX_JSX__.jsx)("div", {
              className: "px-2 py-2 text-sm text-token-text-secondary",
              children: "サブスクリプションを読み込み中…",
            })
          : accounts.map((account) => {
              const selected = account.id === selectedId;
              const count = resetCounts[account.id];
              return (0, __CODEX_MUX_JSX__.jsxs)(
                "button",
                {
                  type: "button",
                  className: [
                    "flex min-w-fit items-center gap-2 rounded-xl px-3 py-2 text-left",
                    "transition-colors hover:bg-token-foreground/5",
                    selected
                      ? "bg-token-foreground/10 text-token-text-primary"
                      : "text-token-text-secondary",
                  ].join(" "),
                  "aria-pressed": selected,
                  onClick: () => onSelect(account.id),
                  children: [
                    (0, __CODEX_MUX_JSX__.jsx)(CodexMuxAccountAvatar, {
                      imageUrl: account.profileImageUrl,
                      label: account.label,
                      className: "size-7",
                    }),
                    (0, __CODEX_MUX_JSX__.jsxs)("span", {
                      className: "flex min-w-0 flex-col",
                      children: [
                        (0, __CODEX_MUX_JSX__.jsx)("span", {
                          className: "max-w-40 truncate text-sm font-medium",
                          children: account.planLabel
                            ? `${account.label} · ${account.planLabel}`
                            : account.label,
                        }),
                        (0, __CODEX_MUX_JSX__.jsx)("span", {
                          className: "text-xs text-token-text-tertiary",
                          children:
                            count == null
                              ? "リセット情報を取得できません"
                              : `利用可能なリセット: ${count}回`,
                        }),
                      ],
                    }),
                  ],
                },
                account.id,
              );
            }),
      }),
    ],
  });
}

function CodexMuxAccountMenu() {
  const [accounts, setAccounts] = __CODEX_MUX_REACT__.useState([]);
  const [loading, setLoading] = __CODEX_MUX_REACT__.useState(true);
  const [busy, setBusy] = __CODEX_MUX_REACT__.useState(false);
  const [error, setError] = __CODEX_MUX_REACT__.useState("");
  const [login, setLogin] = __CODEX_MUX_REACT__.useState(null);
  const [addMethodOpen, setAddMethodOpen] = __CODEX_MUX_REACT__.useState(false);
  const [codeCopied, setCodeCopied] = __CODEX_MUX_REACT__.useState(false);
  const [managing, setManaging] = __CODEX_MUX_REACT__.useState(false);
  const [pendingRemoval, setPendingRemoval] = __CODEX_MUX_REACT__.useState(null);
  const [removalError, setRemovalError] = __CODEX_MUX_REACT__.useState("");
  const loginAccountId = login?.accountId || null;

  const refresh = __CODEX_MUX_REACT__.useCallback(async () => {
    try {
      const result = await codexMuxRequest("/accounts");
      const nextAccounts = result.accounts || [];
      globalThis.__codexMuxConnectedAccounts = nextAccounts.filter(
        (account) => account.connected && account.enabled,
      );
      setAccounts(nextAccounts);
      setError("");
      if (nextAccounts.some((account) => account.connected)) setLoading(false);
    } catch (requestError) {
      setError(requestError.message);
      setLoading(false);
    }
  }, []);

  __CODEX_MUX_REACT__.useEffect(() => {
    refresh();
    const events = new EventSource(
      `${CODEX_MUX_API}/events?token=${encodeURIComponent(CODEX_MUX_TOKEN)}`,
    );
    events.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (
          payload.type === "account-updated" &&
          payload.accountId === loginAccountId
        ) {
          codexMuxLoginActive = false;
          setLogin(null);
        }
        if (payload.type === "account-updated") refresh();
        if (payload.type === "account-removed") refresh();
      } catch {}
    };
    const warmupTimer = setTimeout(refresh, 2_000);
    const loadingDeadline = setTimeout(() => {
      refresh().finally(() => setLoading(false));
    }, 6_000);
    const timer = setInterval(refresh, 30_000);
    return () => {
      clearTimeout(warmupTimer);
      clearTimeout(loadingDeadline);
      clearInterval(timer);
      events.close();
    };
  }, [refresh, loginAccountId]);

  __CODEX_MUX_REACT__.useEffect(() => {
    if (!login && !managing && !pendingRemoval && !addMethodOpen) return;
    const allowEscapeDismissal = (event) => {
      if (event.key !== "Escape") return;
      codexMuxLoginActive = false;
      setLogin(null);
      setPendingRemoval(null);
      setManaging(false);
      setAddMethodOpen(false);
    };
    window.addEventListener("keydown", allowEscapeDismissal, true);
    return () => window.removeEventListener("keydown", allowEscapeDismissal, true);
  }, [login, managing, pendingRemoval, addMethodOpen]);

  const connected = accounts.filter(
    (account) => account.connected && account.enabled,
  );
  // サインインに失敗したアカウントは接続済みにならず通常は表示されない。
  // 管理モードでは未接続のものも並べる。そうしないと消す手段が無くなる。
  const listedAccounts = managing
    ? accounts.filter((account) => account.enabled)
    : connected;
  const weeklyWindows = connected.map((account) =>
    codexMuxWeeklyWindow(account.rateLimits),
  );
  const hasCompleteUsage =
    connected.length > 0 && weeklyWindows.every((weekly) => weekly != null);
  const totalRemaining = weeklyWindows.reduce(
    (total, weekly) =>
      total + (weekly == null ? 0 : Math.max(0, 100 - weekly.usedPercent)),
    0,
  );

  async function addSubscription(event) {
    event.preventDefault();
    if (busy) return;
    codexMuxLoginActive = true;
    setAddMethodOpen(false);
    setBusy(true);
    setError("");
    try {
      const created = await codexMuxRequest("/accounts", {
        method: "POST",
        body: JSON.stringify({ label: `サブスクリプション ${connected.length + 1}` }),
      });
      const result = await codexMuxRequest(`/accounts/${created.account.id}/login`, {
        method: "POST",
        body: JSON.stringify({ mode: "chatgptDeviceCode" }),
      });
      const pendingLogin = result.login
        ? { ...result.login, accountId: created.account.id }
        : null;
      codexMuxLoginActive = pendingLogin != null;
      setCodeCopied(false);
      setLogin(pendingLogin);
      await refresh();
    } catch (requestError) {
      codexMuxLoginActive = false;
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  function chooseAddMethod(event) {
    event.preventDefault();
    if (busy) return;
    codexMuxLoginActive = true;
    setAddMethodOpen(true);
  }

  function cancelAddMethod(event) {
    event.preventDefault();
    if (busy) return;
    codexMuxLoginActive = false;
    setAddMethodOpen(false);
  }

  function importSubscription(event) {
    event.preventDefault();
    if (busy) return;
    codexMuxLoginActive = true;
    const input = document.createElement("input");
    input.type = "file";
    input.accept = "application/json,.json";
    input.hidden = true;
    document.body.append(input);
    const cleanup = () => {
      codexMuxLoginActive = false;
      input.remove();
    };
    input.addEventListener("cancel", cleanup, { once: true });
    input.addEventListener(
      "change",
      async () => {
        const file = input.files?.[0];
        if (!file) {
          cleanup();
          return;
        }
        setBusy(true);
        setError("");
        try {
          if (file.size > 64 * 1024) {
            throw new Error("auth.json は 64 KB 未満である必要があります。");
          }
          const auth = JSON.parse(await file.text());
          if (auth == null || Array.isArray(auth) || typeof auth !== "object") {
            throw new Error("auth.json は 1 つの JSON オブジェクトである必要があります。");
          }
          await codexMuxRequest("/accounts/import", {
            method: "POST",
            body: JSON.stringify({
              label: `サブスクリプション ${accounts.length + 1}`,
              auth,
            }),
          });
          await refresh();
        } catch (requestError) {
          const message =
            requestError instanceof SyntaxError
              ? "auth.json が正しい JSON ではありません。"
              : requestError.message;
          await refresh();
          setError(message);
        } finally {
          setBusy(false);
          setAddMethodOpen(false);
          cleanup();
        }
      },
      { once: true },
    );
    input.click();
  }

  async function copyCodeAndContinue(event) {
    event.preventDefault();
    const userCode = login?.userCode || "";
    const verificationUrl = login?.verificationUrl || login?.authUrl || "";
    const copy = userCode
      ? navigator.clipboard.writeText(userCode)
      : Promise.resolve();
    if (verificationUrl) {
      try {
        const destination = new URL(verificationUrl);
        const trustedHost =
          destination.hostname === "chatgpt.com" ||
          destination.hostname === "auth.openai.com";
        if (destination.protocol !== "https:" || !trustedHost) {
          throw new Error("untrusted verification URL");
        }
        window.open(destination.href, "_blank", "noopener,noreferrer");
      } catch {
        setError("サインイン確認ページを安全に開けませんでした。");
      }
    }
    try {
      await copy;
      setCodeCopied(userCode !== "");
    } catch {
      setError("サインインコードをコピーできませんでした。");
    }
  }

  function startManaging(event) {
    event.preventDefault();
    setPendingRemoval(null);
    setRemovalError("");
    setManaging(true);
  }

  function exitManaging(event) {
    event.preventDefault();
    setPendingRemoval(null);
    setRemovalError("");
    setManaging(false);
  }

  async function removeSubscription(event) {
    event.preventDefault();
    if (busy || !pendingRemoval) return;
    setBusy(true);
    setRemovalError("");
    try {
      await codexMuxRequest(
        `/accounts/${encodeURIComponent(pendingRemoval.id)}`,
        { method: "DELETE" },
      );
      setPendingRemoval(null);
      setManaging(false);
      await refresh();
    } catch (requestError) {
      setRemovalError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  const rows = [];
  rows.push(
    (0, __CODEX_MUX_JSX__.jsx)(
      __CODEX_MUX_MENU_ITEM__,
      {
        LeftIcon: CodexMuxUsageIcon,
        SubText: loading
          ? "サブスクリプションに接続中…"
          : `接続中のサブスクリプション: ${connected.length}件`,
        rightIcon: (0, __CODEX_MUX_JSX__.jsx)("span", {
          className: "text-token-description-foreground tabular-nums",
          children: loading
            ? "…"
            : hasCompleteUsage
              ? `${Math.round(totalRemaining)}%`
              : "–",
        }),
        children: "残り利用枠",
      },
      "codex-mux-total",
    ),
  );
  if (connected.length > 0) {
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(__CODEX_MUX_MENU__.Separator, {}, "codex-mux-accounts-separator"),
    );
  }

  for (const account of listedAccounts) {
    const weekly = codexMuxWeeklyWindow(account.rateLimits);
    const remaining = weekly == null ? null : Math.max(0, 100 - weekly.usedPercent);
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          LeftIcon: (iconProps) =>
            (0, __CODEX_MUX_JSX__.jsx)(CodexMuxAccountAvatar, {
              ...iconProps,
              imageUrl: account.profileImageUrl,
              label: account.label,
            }),
          SubText: !account.connected
            ? "サインイン未完了"
            : account.email
              ? (0, __CODEX_MUX_JSX__.jsx)(CodexMuxMaskedEmail, { email: account.email })
              : account.planType || "ChatGPTサブスクリプション",
          className: "group",
          onSelect:
            managing && !account.controller
              ? () => setPendingRemoval(account)
              : undefined,
          rightIcon: managing
            ? (0, __CODEX_MUX_JSX__.jsx)("span", {
                className: [
                  "text-xs font-medium",
                  account.controller
                    ? "text-token-description-foreground"
                    : "text-token-text-primary",
                ].join(" "),
                children: account.controller ? "プライマリ" : "削除",
              })
            : (0, __CODEX_MUX_JSX__.jsx)("span", {
                className: "text-token-description-foreground tabular-nums",
                children: remaining == null ? "–" : `${Math.round(remaining)}%`,
              }),
          children: account.planLabel
            ? `${account.label} · ${account.planLabel}`
            : account.label,
        },
        `codex-mux-account-${account.id}`,
      ),
    );
  }

  if (pendingRemoval) {
    const pendingLabel = pendingRemoval.planLabel
      ? `${pendingRemoval.label} · ${pendingRemoval.planLabel}`
      : pendingRemoval.label;
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          LeftIcon: CodexMuxAlertIcon,
          SubText:
            "チャットはただちにこのアプリから消えます。アカウントのデータはローカルのバックアップに残ります",
          tone: "danger",
          allowWrap: true,
          subTextAllowWrap: true,
          onSelect: removeSubscription,
          children: busy
            ? "削除中…"
            : `「${pendingLabel}」をこの PC から削除`,
        },
        "codex-mux-remove-confirm",
      ),
    );
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          onSelect: () => setPendingRemoval(null),
          children: "キャンセル",
        },
        "codex-mux-remove-cancel",
      ),
    );
  }

  if (removalError) {
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          LeftIcon: CodexMuxAlertIcon,
          SubText: removalError,
          tone: "danger",
          allowWrap: true,
          subTextAllowWrap: true,
          children: "サブスクリプションを削除できません",
        },
        "codex-mux-remove-error",
      ),
    );
  }

  if (login) {
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          LeftIcon: CodexMuxCopyIcon,
          SubText: login.userCode
            ? codeCopied
              ? `コード ${login.userCode} をコピーしました`
              : `コード ${login.userCode} · クリックでコピー`
            : "ChatGPTでのサインインを完了してください",
          onSelect: copyCodeAndContinue,
          children: "サインインを続行",
        },
        "codex-mux-login",
      ),
    );
  }

  if (error) {
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          LeftIcon: CodexMuxAlertIcon,
          SubText: error,
          tone: "danger",
          allowWrap: true,
          subTextAllowWrap: true,
          children: "サブスクリプションプールを利用できません",
        },
        "codex-mux-error",
      ),
    );
  }

  if (!loading) {
    if (addMethodOpen) {
      rows.push(
        (0, __CODEX_MUX_JSX__.jsx)(
          __CODEX_MUX_MENU_ITEM__,
          {
            LeftIcon: CodexMuxPlusIcon,
            SubText: "使い捨てのデバイスコードでサインインする",
            onSelect: addSubscription,
            children: busy ? "処理中…" : "ChatGPT でサインイン",
          },
          "codex-mux-add-device-code",
        ),
      );
      rows.push(
        (0, __CODEX_MUX_JSX__.jsx)(
          __CODEX_MUX_MENU_ITEM__,
          {
            LeftIcon: CodexMuxCopyIcon,
            SubText: "既存の Codex ログインファイルを使う",
            onSelect: importSubscription,
            children: busy ? "処理中…" : "auth.json をインポート",
          },
          "codex-mux-import-auth",
        ),
      );
      rows.push(
        (0, __CODEX_MUX_JSX__.jsx)(
          __CODEX_MUX_MENU_ITEM__,
          {
            onSelect: cancelAddMethod,
            children: "キャンセル",
          },
          "codex-mux-cancel-add",
        ),
      );
    } else {
      rows.push(
        (0, __CODEX_MUX_JSX__.jsx)(
          __CODEX_MUX_MENU_ITEM__,
          {
            LeftIcon: CodexMuxPlusIcon,
            onSelect: chooseAddMethod,
            children: "サブスクリプションを追加",
          },
          "codex-mux-add",
        ),
      );
    }
  }
  if (!loading && connected.length > 0) {
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          onSelect: managing ? exitManaging : startManaging,
          children: managing ? "完了" : "サブスクリプションを管理",
        },
        managing ? "codex-mux-manage-done" : "codex-mux-manage",
      ),
    );
  }
  rows.push((0, __CODEX_MUX_JSX__.jsx)(__CODEX_MUX_MENU__.Separator, {}, "codex-mux-separator"));
  return (0, __CODEX_MUX_JSX__.jsx)(__CODEX_MUX_JSX__.Fragment, { children: rows });
}

function codexMuxWeeklyWindow(rateLimits) {
  const windows = [rateLimits?.primary, rateLimits?.secondary].filter(Boolean);
  windows.sort(
    (left, right) =>
      (left.windowDurationMins || 0) - (right.windowDurationMins || 0),
  );
  return windows.at(-1) || null;
}

function codexMuxUsageWindows(rateLimits) {
  return [rateLimits?.primary, rateLimits?.secondary]
    .filter(Boolean)
    .map((window) => ({
      usedPercent: window.usedPercent,
      remainingPercent: Math.max(0, 100 - window.usedPercent),
      windowMinutes: window.windowDurationMins || 0,
      resetsAt: window.resetsAt ?? null,
    }));
}

function CodexMuxUsageIcon(props) {
  return (0, __CODEX_MUX_JSX__.jsx)("svg", {
    viewBox: "0 0 20 20",
    fill: "none",
    "aria-hidden": true,
    ...props,
    children: (0, __CODEX_MUX_JSX__.jsx)("path", {
      d: "M10 3.25a6.75 6.75 0 1 1 0 13.5 6.75 6.75 0 0 1 0-13.5Zm0 3v4l2.5 1.5",
      stroke: "currentColor",
      strokeWidth: 1.5,
      strokeLinecap: "round",
      strokeLinejoin: "round",
    }),
  });
}

function CodexMuxAlertIcon(props) {
  return (0, __CODEX_MUX_JSX__.jsx)("svg", {
    viewBox: "0 0 20 20",
    fill: "none",
    "aria-hidden": true,
    ...props,
    children: (0, __CODEX_MUX_JSX__.jsxs)(__CODEX_MUX_JSX__.Fragment, {
      children: [
        (0, __CODEX_MUX_JSX__.jsx)("circle", {
          cx: 10, cy: 10, r: 6.75,
          stroke: "currentColor", strokeWidth: 1.5,
        }),
        (0, __CODEX_MUX_JSX__.jsx)("path", {
          d: "M10 6.5v4M10 13.25h.01",
          stroke: "currentColor", strokeWidth: 1.5, strokeLinecap: "round",
        }),
      ],
    }),
  });
}

function CodexMuxPlusIcon(props) {
  return (0, __CODEX_MUX_JSX__.jsx)("svg", {
    viewBox: "0 0 20 20",
    fill: "none",
    "aria-hidden": true,
    ...props,
    children: (0, __CODEX_MUX_JSX__.jsx)("path", {
      d: "M10 4.25v11.5M4.25 10h11.5",
      stroke: "currentColor",
      strokeWidth: 1.5,
      strokeLinecap: "round",
    }),
  });
}

function CodexMuxCopyIcon(props) {
  return (0, __CODEX_MUX_JSX__.jsx)("svg", {
    viewBox: "0 0 20 20",
    fill: "none",
    "aria-hidden": true,
    ...props,
    children: (0, __CODEX_MUX_JSX__.jsxs)(__CODEX_MUX_JSX__.Fragment, {
      children: [
        (0, __CODEX_MUX_JSX__.jsx)("rect", {
          x: 6.25,
          y: 6.25,
          width: 9.5,
          height: 9.5,
          rx: 2,
          stroke: "currentColor",
          strokeWidth: 1.5,
        }),
        (0, __CODEX_MUX_JSX__.jsx)("path", {
          d: "M13.75 6.25V6A1.75 1.75 0 0 0 12 4.25H6A1.75 1.75 0 0 0 4.25 6v6c0 .97.78 1.75 1.75 1.75h.25",
          stroke: "currentColor",
          strokeWidth: 1.5,
          strokeLinecap: "round",
        }),
      ],
    }),
  });
}

function CodexMuxMaskedEmail({ email }) {
  return (0, __CODEX_MUX_JSX__.jsxs)(__CODEX_MUX_JSX__.Fragment, {
    children: [
      (0, __CODEX_MUX_JSX__.jsx)("span", {
        className: "group-hover:hidden",
        children: "••••••••",
      }),
      (0, __CODEX_MUX_JSX__.jsx)("span", {
        className: "hidden group-hover:inline",
        children: email,
      }),
    ],
  });
}

function CodexMuxAccountAvatar({ imageUrl, label, className }) {
  const [failed, setFailed] = __CODEX_MUX_REACT__.useState(false);
  const resolvedImageUrl = __CODEX_MUX_IMAGE_URL__(imageUrl || null);
  if (resolvedImageUrl && !failed) {
    return (0, __CODEX_MUX_JSX__.jsx)("img", {
      src: resolvedImageUrl,
      alt: "",
      className: `${className || "icon-sm"} rounded-full object-cover`,
      referrerPolicy: "no-referrer",
      onError: () => setFailed(true),
    });
  }
  const initials = label
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase())
    .join("");
  return (0, __CODEX_MUX_JSX__.jsx)("span", {
    className: `${className || "icon-sm"} flex items-center justify-center rounded-full bg-token-charts-purple/10 text-[9px] leading-none text-token-charts-purple`,
    "aria-hidden": true,
    children: initials || "?",
  });
}

function CodexMuxOverlappingAvatars({ accounts, size = "size-20" }) {
  const overlapClass = size === "size-20" ? "-ml-10" : "-ml-2";
  return (0, __CODEX_MUX_JSX__.jsx)("div", {
    className: "flex items-center justify-center",
    children: accounts.map((account, index) =>
      (0, __CODEX_MUX_JSX__.jsx)(
        "span",
        {
          className: `${index === 0 ? "" : overlapClass} rounded-full border-4 border-token-bg-primary`,
          title: account.planLabel
            ? `${account.label} · ${account.planLabel}`
            : account.label,
          children: (0, __CODEX_MUX_JSX__.jsx)(CodexMuxAccountAvatar, {
            imageUrl: account.profileImageUrl,
            label: account.label,
            className: size,
          }),
        },
        account.id,
      ),
    ),
  });
}

function CodexMuxUseConnectedAccounts() {
  const [accounts, setAccounts] = __CODEX_MUX_REACT__.useState(
    globalThis.__codexMuxCombinedProfileAccounts || [],
  );
  __CODEX_MUX_REACT__.useEffect(() => {
    let live = true;
    codexMuxRequest("/accounts")
      .then((result) => {
        if (!live) return;
        const connected = (result.accounts || []).filter(
          (account) => account.connected && account.enabled,
        );
        globalThis.__codexMuxCombinedProfileAccounts = connected;
        setAccounts(connected);
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, []);
  return accounts;
}

// プロフィール設定の統計は全サブスクリプションの合算値である。
// 単一アカウントの識別情報のままでは合算だと分からないため、接続中の
// アカウントを重ねて示す。接続が 1 件だけならネイティブ表示に任せる。
function CodexMuxProfileAvatarStack() {
  const accounts = CodexMuxUseConnectedAccounts();
  if (accounts.length < 2) return null;
  return (0, __CODEX_MUX_JSX__.jsx)("div", {
    className: "flex items-center justify-center",
    "aria-label": `接続中のサブスクリプション: ${accounts.length}件`,
    children: accounts.map((account, index) =>
      (0, __CODEX_MUX_JSX__.jsx)(
        "span",
        {
          className:
            "rounded-full border-4 border-token-bg-primary transition-transform hover:z-10 hover:scale-105",
          style: { marginLeft: index === 0 ? 0 : -20, zIndex: index },
          title: account.planLabel
            ? `${account.label} · ${account.planLabel}`
            : account.label,
          children: (0, __CODEX_MUX_JSX__.jsx)(CodexMuxAccountAvatar, {
            imageUrl: account.profileImageUrl,
            label: account.label,
            className: "size-20",
          }),
        },
        account.id,
      ),
    ),
  });
}

function CodexMuxProfileDisplayName() {
  const accounts = CodexMuxUseConnectedAccounts();
  if (accounts.length < 2) return null;
  return "合算プロフィール";
}

function CodexMuxProfileUsername() {
  const accounts = CodexMuxUseConnectedAccounts();
  if (accounts.length < 2) return null;
  return `接続中のサブスクリプション: ${accounts.length}件`;
}

function CodexMuxPluginScope() {
  const [accounts, setAccounts] = __CODEX_MUX_REACT__.useState([]);
  const [selectedId, setSelectedId] = __CODEX_MUX_REACT__.useState("primary");
  const [loading, setLoading] = __CODEX_MUX_REACT__.useState(true);
  const queryClient = lt();
  __CODEX_MUX_REACT__.useEffect(() => {
    let live = true;
    codexMuxRequest("/accounts")
      .then((result) => {
        if (!live) return;
        setAccounts(
          (result.accounts || []).filter(
            (account) => account.connected && account.enabled,
          ),
        );
      })
      .catch(() => {})
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, []);

  __CODEX_MUX_REACT__.useEffect(() => {
    globalThis.__codexMuxPluginAccountId = selectedId;
    return () => {
      delete globalThis.__codexMuxPluginAccountId;
    };
  }, [selectedId]);

  async function selectAccount(accountId) {
    if (accountId === selectedId) return;
    globalThis.__codexMuxPluginAccountId = accountId;
    setSelectedId(accountId);
    await queryClient.invalidateQueries({
      predicate: (query) => {
        const root = query.queryKey?.[0];
        return root === "apps" || root === "plugins" || root === "mcp";
      },
    });
  }

  const selected =
    accounts.find((account) => account.id === selectedId) || accounts[0] || null;

  return (0, __CODEX_MUX_JSX__.jsxs)("div", {
    className:
      "mb-5 rounded-2xl border border-token-border-light p-3",
    children: [
      (0, __CODEX_MUX_JSX__.jsxs)("div", {
        className: "px-1",
        children: [
          (0, __CODEX_MUX_JSX__.jsx)("div", {
            className: "text-sm font-medium text-token-text-primary",
            children: "プラグイン接続",
          }),
          (0, __CODEX_MUX_JSX__.jsx)("div", {
            className: "mt-0.5 text-xs text-token-text-secondary",
            children: selected
              ? `インストールは全サブスクリプションで共有されます。以下の接続アクセスは「${selected.label}」のものです。`
              : "インストールは全サブスクリプションで共有されます。接続アクセスに使うサブスクリプションを選択してください。",
          }),
        ],
      }),
      loading
        ? (0, __CODEX_MUX_JSX__.jsx)("div", {
            className: "mt-3 px-1 text-sm text-token-text-tertiary",
            children: "サブスクリプションを読み込み中…",
          })
        : (0, __CODEX_MUX_JSX__.jsx)("div", {
            className: "mt-3 flex flex-wrap gap-2",
            children: accounts.map((account) => {
              const active = account.id === selected?.id;
              return (0, __CODEX_MUX_JSX__.jsxs)(
                "button",
                {
                  type: "button",
                  className: [
                    "flex items-center gap-2 rounded-xl px-2.5 py-2 text-sm transition-colors",
                    active
                      ? "bg-token-foreground/10 text-token-text-primary"
                      : "text-token-text-secondary hover:bg-token-foreground/5",
                  ].join(" "),
                  "aria-pressed": active,
                  onClick: () => selectAccount(account.id),
                  children: [
                    (0, __CODEX_MUX_JSX__.jsx)(CodexMuxAccountAvatar, {
                      imageUrl: account.profileImageUrl,
                      label: account.label,
                      className: "size-7",
                    }),
                    (0, __CODEX_MUX_JSX__.jsx)("span", {
                      children: account.planLabel
                        ? `${account.label} · ${account.planLabel}`
                        : account.label,
                    }),
                  ],
                },
                account.id,
              );
            }),
          }),
    ],
  });
}

// The thread summary is emitted into a separate lazy-loaded renderer chunk.
// Export the same avatar component so both surfaces share image resolution,
// error handling, and the initials fallback.
globalThis.CodexMuxAccountAvatar = CodexMuxAccountAvatar;
globalThis.codexMuxProfileData = codexMuxProfileData;
globalThis.CodexMuxProfileAvatarStack = () =>
  (0, __CODEX_MUX_JSX__.jsx)(CodexMuxProfileAvatarStack, {});
globalThis.CodexMuxProfileDisplayName = () =>
  (0, __CODEX_MUX_JSX__.jsx)(CodexMuxProfileDisplayName, {});
globalThis.CodexMuxProfileUsername = () =>
  (0, __CODEX_MUX_JSX__.jsx)(CodexMuxProfileUsername, {});
globalThis.CodexMuxPluginScope = () =>
  (0, __CODEX_MUX_JSX__.jsx)(CodexMuxPluginScope, {});
