const CODEX_MUX_API = "http://127.0.0.1:__CODEX_MUX_CONTROL_PORT__/v1";
const CODEX_MUX_TOKEN = "__CODEX_MUX_CONTROL_TOKEN__";
let codexMuxLoginActive = false;

// formatjs message descriptors, the same shape the official app uses. A locale
// bundle's translation wins when present; otherwise defaultMessage (English) is shown.
// The patcher inserts our translations into the official locale bundles.
const CODEX_MUX_MESSAGES = {
  requestFailed: {
    id: "codexMux.requestFailed",
    defaultMessage: "Request failed ({status})",
    description: "Error when a control API request fails",
  },
  subscription: {
    id: "codexMux.subscription",
    defaultMessage: "Subscription",
    description: "Label above the subscription picker in the usage sheet",
  },
  loadingSubscriptions: {
    id: "codexMux.loadingSubscriptions",
    defaultMessage: "Loading subscriptions…",
    description: "Placeholder while connected subscriptions are fetched",
  },
  resetsUnavailable: {
    id: "codexMux.resetsUnavailable",
    defaultMessage: "Resets unavailable",
    description: "Shown when a subscription's reset credits cannot be read",
  },
  resetsAvailable: {
    id: "codexMux.resetsAvailable",
    defaultMessage:
      "{count, plural, one {# reset available} other {# resets available}}",
    description: "Number of banked usage resets for a subscription",
  },
  subscriptionLabel: {
    id: "codexMux.subscriptionLabel",
    defaultMessage: "Subscription {index}",
    description: "Default label given to a newly added subscription",
  },
  authTooLarge: {
    id: "codexMux.authTooLarge",
    defaultMessage: "auth.json must be smaller than 64 KB.",
    description: "Rejection when an imported auth.json is too large",
  },
  authNotObject: {
    id: "codexMux.authNotObject",
    defaultMessage: "auth.json must contain one JSON object.",
    description: "Rejection when an imported auth.json is not a JSON object",
  },
  authInvalidJson: {
    id: "codexMux.authInvalidJson",
    defaultMessage: "auth.json is not valid JSON.",
    description: "Rejection when an imported auth.json cannot be parsed",
  },
  verificationOpenFailed: {
    id: "codexMux.verificationOpenFailed",
    defaultMessage: "The sign-in verification page could not be opened safely.",
    description: "Error when the device-code verification URL is not trusted",
  },
  codeCopyFailed: {
    id: "codexMux.codeCopyFailed",
    defaultMessage: "The sign-in code could not be copied.",
    description: "Error when the device code cannot be copied to the clipboard",
  },
  connectingSubscriptions: {
    id: "codexMux.connectingSubscriptions",
    defaultMessage: "Connecting subscriptions…",
    description: "Shown while the account pool is still loading",
  },
  connectedCount: {
    id: "codexMux.connectedCount",
    defaultMessage:
      "{count, plural, one {# connected subscription} other {# connected subscriptions}}",
    description: "How many subscriptions are connected",
  },
  usageRemaining: {
    id: "codexMux.usageRemaining",
    defaultMessage: "Usage remaining",
    description: "Pooled weekly allowance row in the profile menu",
  },
  signInUnfinished: {
    id: "codexMux.signInUnfinished",
    defaultMessage: "Sign-in not finished",
    description: "Shown for an account whose sign-in never completed",
  },
  chatgptSubscription: {
    id: "codexMux.chatgptSubscription",
    defaultMessage: "ChatGPT subscription",
    description: "Fallback description for a connected account",
  },
  primary: {
    id: "codexMux.primary",
    defaultMessage: "Primary",
    description:
      "Name of the subscription that cannot be removed, and the badge marking it",
  },
  remove: {
    id: "codexMux.remove",
    defaultMessage: "Remove",
    description: "Action shown on an account row while managing subscriptions",
  },
  removeHint: {
    id: "codexMux.removeHint",
    defaultMessage:
      "Its chats leave this app immediately; the account data is kept in a local backup",
    description: "Explains what removing a subscription does",
  },
  removing: {
    id: "codexMux.removing",
    defaultMessage: "Removing…",
    description: "Shown while a subscription is being removed",
  },
  removeConfirm: {
    id: "codexMux.removeConfirm",
    defaultMessage: "Remove {label} from this PC",
    description: "Confirmation row for removing a subscription",
  },
  cancel: {
    id: "codexMux.cancel",
    defaultMessage: "Cancel",
    description: "Dismisses the current choice",
  },
  removeFailed: {
    id: "codexMux.removeFailed",
    defaultMessage: "Could not remove subscription",
    description: "Shown when removing a subscription fails",
  },
  codeCopied: {
    id: "codexMux.codeCopied",
    defaultMessage: "Code {code} copied",
    description: "Shown after the device code is copied",
  },
  codeClickToCopy: {
    id: "codexMux.codeClickToCopy",
    defaultMessage: "Code {code} · Click to copy",
    description: "Prompts the user to copy the device code",
  },
  finishSignIn: {
    id: "codexMux.finishSignIn",
    defaultMessage: "Finish signing in with ChatGPT",
    description: "Shown while a device-code sign-in is pending",
  },
  continueSignIn: {
    id: "codexMux.continueSignIn",
    defaultMessage: "Continue sign-in",
    description: "Opens the verification page for a pending sign-in",
  },
  poolUnavailable: {
    id: "codexMux.poolUnavailable",
    defaultMessage: "Subscription pool unavailable",
    description: "Shown when the control API cannot be reached",
  },
  deviceCodeHint: {
    id: "codexMux.deviceCodeHint",
    defaultMessage: "Sign in with a one-time device code",
    description: "Describes the device-code way to add a subscription",
  },
  working: {
    id: "codexMux.working",
    defaultMessage: "Working…",
    description: "Shown while adding a subscription is in progress",
  },
  continueWithChatGPT: {
    id: "codexMux.continueWithChatGPT",
    defaultMessage: "Continue with ChatGPT",
    description: "Starts a device-code sign-in",
  },
  importHint: {
    id: "codexMux.importHint",
    defaultMessage: "Use an existing Codex login file",
    description: "Describes the auth.json way to add a subscription",
  },
  importAuth: {
    id: "codexMux.importAuth",
    defaultMessage: "Import auth.json",
    description: "Starts an auth.json import",
  },
  addSubscription: {
    id: "codexMux.addSubscription",
    defaultMessage: "Add another subscription",
    description: "Opens the choice of how to add a subscription",
  },
  done: {
    id: "codexMux.done",
    defaultMessage: "Done",
    description: "Leaves the subscription management mode",
  },
  manageSubscriptions: {
    id: "codexMux.manageSubscriptions",
    defaultMessage: "Manage subscriptions",
    description: "Enters the mode where subscriptions can be removed",
  },
  showCombinedStats: {
    id: "codexMux.showCombinedStats",
    defaultMessage: "Show combined profile stats",
    description: "Accessible label for the combined profile avatars",
  },
  showAccountStats: {
    id: "codexMux.showAccountStats",
    defaultMessage: "Show {label} profile stats",
    description: "Accessible label for one subscription's profile avatar",
  },
  selectedProfile: {
    id: "codexMux.selectedProfile",
    defaultMessage: "Selected subscription profile",
    description: "Accessible label when one subscription's profile is shown",
  },
  combinedProfile: {
    id: "codexMux.combinedProfile",
    defaultMessage: "Combined profile",
    description: "Name shown when profile stats cover every subscription",
  },
  pluginConnections: {
    id: "codexMux.pluginConnections",
    defaultMessage: "Plugin connections",
    description: "Heading of the subscription picker on the Plugins page",
  },
  pluginScopeFor: {
    id: "codexMux.pluginScopeFor",
    defaultMessage:
      "Installs are shared. Connection access below is for {label}.",
    description: "Explains which subscription plugin connections apply to",
  },
  pluginScopeNone: {
    id: "codexMux.pluginScopeNone",
    defaultMessage:
      "Installs are shared. Choose a subscription for connection access.",
    description: "Prompts the user to pick a subscription for plugin access",
  },
  usageUnavailable: {
    id: "codexMux.usageUnavailable",
    defaultMessage: "Usage unavailable",
    description: "Shown when a thread's subscription usage cannot be read",
  },
  depleted: {
    id: "codexMux.depleted",
    defaultMessage: "Depleted",
    description: "Shown when a subscription has no weekly allowance left",
  },
  percentRemaining: {
    id: "codexMux.percentRemaining",
    defaultMessage: "{percent}% remaining",
    description: "Weekly allowance left on the thread's subscription",
  },
};


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
  if (!response.ok) {
    throw new Error(body.error || `Request failed (${response.status})`);
  }
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
  const intl = __CODEX_MUX_INTL__();
  const cachedAccounts = (globalThis.__codexMuxConnectedAccounts || []).filter(
    (account) => account.connected && account.enabled,
  );
  const [accounts, setAccounts] = __CODEX_MUX_REACT__.useState(cachedAccounts);
  const [selectedId, setSelectedId] = __CODEX_MUX_REACT__.useState("primary");
  const [resetCounts, setResetCounts] = __CODEX_MUX_REACT__.useState({});
  const [loading, setLoading] = __CODEX_MUX_REACT__.useState(cachedAccounts.length === 0);

  const loadAccounts = __CODEX_MUX_REACT__.useCallback(async () => {
    const result = await codexMuxRequest("/accounts");
    codexMuxRememberAccountOrder(result.accounts || []);
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
      intl,
      loading,
      resetCounts,
      selectedId: activeId,
      onSelect: setSelectedId,
    },
  );

}

function CodexMuxResetAccountSelector({
  accounts,
  intl,
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
        children: intl.formatMessage(CODEX_MUX_MESSAGES.subscription),
      }),
      (0, __CODEX_MUX_JSX__.jsx)("div", {
        className:
          "flex flex-wrap gap-2 rounded-2xl border border-token-border p-2",
        children: loading
          ? (0, __CODEX_MUX_JSX__.jsx)("div", {
              className: "px-2 py-2 text-sm text-token-text-secondary",
              children: intl.formatMessage(CODEX_MUX_MESSAGES.loadingSubscriptions),
            })
          : accounts.map((account, index) => {
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
                      label: codexMuxAccountLabel(intl, account, index),
                      className: "size-7",
                    }),
                    (0, __CODEX_MUX_JSX__.jsxs)("span", {
                      className: "flex min-w-0 flex-col",
                      children: [
                        (0, __CODEX_MUX_JSX__.jsx)("span", {
                          className: "max-w-40 truncate text-sm font-medium",
                          children: codexMuxAccountTitle(intl, account, index),
                        }),
                        (0, __CODEX_MUX_JSX__.jsx)("span", {
                          className: "text-xs text-token-text-tertiary",
                          children:
                            count == null
                              ? intl.formatMessage(CODEX_MUX_MESSAGES.resetsUnavailable)
                              : intl.formatMessage(
                                  CODEX_MUX_MESSAGES.resetsAvailable,
                                  { count },
                                ),
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
  const intl = __CODEX_MUX_INTL__();
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
  const [resetCounts, setResetCounts] = __CODEX_MUX_REACT__.useState({});
  const loginAccountId = login?.accountId || null;

  const refresh = __CODEX_MUX_REACT__.useCallback(async () => {
    try {
      const result = await codexMuxRequest("/accounts");
      const nextAccounts = result.accounts || [];
      codexMuxRememberAccountOrder(nextAccounts);
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
  // An account whose sign-in failed never becomes connected, so it is normally
  // hidden. List those while managing; otherwise there is no way to remove one.
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

  // Reset credits are fetched per account. Keying the effect on the connected ids
  // refreshes them when the menu opens or the pool changes, without adding a request
  // to every poll.
  const connectedIds = connected.map((account) => account.id).join(",");
  __CODEX_MUX_REACT__.useEffect(() => {
    if (!connectedIds) return;
    let live = true;
    Promise.all(
      connectedIds.split(",").map(async (id) => {
        try {
          const resets = await codexMuxRateLimitResets(id);
          return [id, Math.max(0, resets.available_count || 0)];
        } catch {
          return [id, null];
        }
      }),
    ).then((entries) => {
      if (live) setResetCounts(Object.fromEntries(entries));
    });
    return () => {
      live = false;
    };
  }, [connectedIds]);

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
        body: JSON.stringify({
          // The stored label is backend-facing only; the interface renders its own
          // localized name. Keep it language-neutral so state and events stay English.
          label: `Subscription ${connected.length + 1}`,
        }),
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
            throw new Error(intl.formatMessage(CODEX_MUX_MESSAGES.authTooLarge));
          }
          const auth = JSON.parse(await file.text());
          if (auth == null || Array.isArray(auth) || typeof auth !== "object") {
            throw new Error(intl.formatMessage(CODEX_MUX_MESSAGES.authNotObject));
          }
          await codexMuxRequest("/accounts/import", {
            method: "POST",
            body: JSON.stringify({
              // Backend-facing only; see the device-code path above.
              label: `Subscription ${accounts.length + 1}`,
              auth,
            }),
          });
          await refresh();
        } catch (requestError) {
          const message =
            requestError instanceof SyntaxError
              ? intl.formatMessage(CODEX_MUX_MESSAGES.authInvalidJson)
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
        setError(intl.formatMessage(CODEX_MUX_MESSAGES.verificationOpenFailed));
      }
    }
    try {
      await copy;
      setCodeCopied(userCode !== "");
    } catch {
      setError(intl.formatMessage(CODEX_MUX_MESSAGES.codeCopyFailed));
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
          ? intl.formatMessage(CODEX_MUX_MESSAGES.connectingSubscriptions)
          : intl.formatMessage(CODEX_MUX_MESSAGES.connectedCount, {
              count: connected.length,
            }),
        rightIcon: (0, __CODEX_MUX_JSX__.jsx)("span", {
          className: "text-token-description-foreground tabular-nums",
          children: loading
            ? "…"
            : hasCompleteUsage
              ? `${Math.round(totalRemaining)}%`
              : "–",
        }),
        children: intl.formatMessage(CODEX_MUX_MESSAGES.usageRemaining),
      },
      "codex-mux-total",
    ),
  );
  if (connected.length > 0) {
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(__CODEX_MUX_MENU__.Separator, {}, "codex-mux-accounts-separator"),
    );
  }

  for (const [index, account] of listedAccounts.entries()) {
    const weekly = codexMuxWeeklyWindow(account.rateLimits);
    const remaining = weekly == null ? null : Math.max(0, 100 - weekly.usedPercent);
    const resetCount = account.connected ? resetCounts[account.id] ?? null : null;
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          LeftIcon: (iconProps) =>
            (0, __CODEX_MUX_JSX__.jsx)(CodexMuxAccountAvatar, {
              ...iconProps,
              imageUrl: account.profileImageUrl,
              label: codexMuxAccountLabel(intl, account, index),
            }),
          SubText: (0, __CODEX_MUX_JSX__.jsxs)("span", {
            className: "flex min-w-0 flex-col",
            children: [
              (0, __CODEX_MUX_JSX__.jsx)("span", {
                className: "truncate",
                children: !account.connected
                  ? intl.formatMessage(CODEX_MUX_MESSAGES.signInUnfinished)
                  : account.email
                    ? (0, __CODEX_MUX_JSX__.jsx)(CodexMuxMaskedEmail, {
                        email: account.email,
                      })
                    : account.planType ||
                      intl.formatMessage(CODEX_MUX_MESSAGES.chatgptSubscription),
              }),
              (0, __CODEX_MUX_JSX__.jsx)(CodexMuxUsageBar, {
                remaining,
                title:
                  remaining == null
                    ? undefined
                    : intl.formatMessage(CODEX_MUX_MESSAGES.percentRemaining, {
                        percent: Math.round(remaining),
                      }),
              }),
            ],
          }),
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
                children: intl.formatMessage(
                  account.controller
                    ? CODEX_MUX_MESSAGES.primary
                    : CODEX_MUX_MESSAGES.remove,
                ),
              })
            : (0, __CODEX_MUX_JSX__.jsxs)("span", {
                className:
                  "flex items-center gap-1.5 text-token-description-foreground tabular-nums",
                children: [
                  (0, __CODEX_MUX_JSX__.jsx)("span", {
                    children: remaining == null ? "–" : `${Math.round(remaining)}%`,
                  }),
                  resetCount == null
                    ? null
                    : (0, __CODEX_MUX_JSX__.jsx)("span", {
                        className: "text-xs",
                        style: { color: "rgb(34 197 94)" },
                        title: intl.formatMessage(
                          CODEX_MUX_MESSAGES.resetsAvailable,
                          { count: resetCount },
                        ),
                        children: `♻${resetCount}`,
                      }),
                ],
              }),
          children: codexMuxAccountTitle(intl, account, index),
        },
        `codex-mux-account-${account.id}`,
      ),
    );
  }

  if (pendingRemoval) {
    const pendingLabel = codexMuxAccountTitle(intl, pendingRemoval, null);
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          LeftIcon: CodexMuxAlertIcon,
          SubText:
            intl.formatMessage(CODEX_MUX_MESSAGES.removeHint),
          tone: "danger",
          allowWrap: true,
          subTextAllowWrap: true,
          onSelect: removeSubscription,
          children: busy
            ? intl.formatMessage(CODEX_MUX_MESSAGES.removing)
            : intl.formatMessage(CODEX_MUX_MESSAGES.removeConfirm, {
                label: pendingLabel,
              }),
        },
        "codex-mux-remove-confirm",
      ),
    );
    rows.push(
      (0, __CODEX_MUX_JSX__.jsx)(
        __CODEX_MUX_MENU_ITEM__,
        {
          onSelect: () => setPendingRemoval(null),
          children: intl.formatMessage(CODEX_MUX_MESSAGES.cancel),
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
          children: intl.formatMessage(CODEX_MUX_MESSAGES.removeFailed),
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
              ? intl.formatMessage(CODEX_MUX_MESSAGES.codeCopied, {
                  code: login.userCode,
                })
              : intl.formatMessage(CODEX_MUX_MESSAGES.codeClickToCopy, {
                  code: login.userCode,
                })
            : intl.formatMessage(CODEX_MUX_MESSAGES.finishSignIn),
          onSelect: copyCodeAndContinue,
          children: intl.formatMessage(CODEX_MUX_MESSAGES.continueSignIn),
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
          children: intl.formatMessage(CODEX_MUX_MESSAGES.poolUnavailable),
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
            SubText: intl.formatMessage(CODEX_MUX_MESSAGES.deviceCodeHint),
            onSelect: addSubscription,
            children: intl.formatMessage(
              busy
                ? CODEX_MUX_MESSAGES.working
                : CODEX_MUX_MESSAGES.continueWithChatGPT,
            ),
          },
          "codex-mux-add-device-code",
        ),
      );
      rows.push(
        (0, __CODEX_MUX_JSX__.jsx)(
          __CODEX_MUX_MENU_ITEM__,
          {
            LeftIcon: CodexMuxCopyIcon,
            SubText: intl.formatMessage(CODEX_MUX_MESSAGES.importHint),
            onSelect: importSubscription,
            children: intl.formatMessage(
              busy ? CODEX_MUX_MESSAGES.working : CODEX_MUX_MESSAGES.importAuth,
            ),
          },
          "codex-mux-import-auth",
        ),
      );
      rows.push(
        (0, __CODEX_MUX_JSX__.jsx)(
          __CODEX_MUX_MENU_ITEM__,
          {
            onSelect: cancelAddMethod,
            children: intl.formatMessage(CODEX_MUX_MESSAGES.cancel),
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
            children: intl.formatMessage(CODEX_MUX_MESSAGES.addSubscription),
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
          children: intl.formatMessage(
            managing
              ? CODEX_MUX_MESSAGES.done
              : CODEX_MUX_MESSAGES.manageSubscriptions,
          ),
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

// The bar warns before the allowance runs out: blue while there is room, orange
// under 30 percent, red under 10.
function codexMuxUsageBarColor(remaining) {
  if (remaining <= 10) return "rgb(239 68 68)";
  if (remaining <= 30) return "rgb(249 115 22)";
  return "rgb(59 130 246)";
}

function CodexMuxUsageBar({ remaining, title }) {
  if (remaining == null) return null;
  const width = Math.max(0, Math.min(100, remaining));
  return (0, __CODEX_MUX_JSX__.jsx)("span", {
    className: "mt-1 block h-0.5 w-full overflow-hidden rounded-full",
    style: { backgroundColor: "rgb(107 114 128 / 0.35)" },
    title,
    "aria-hidden": true,
    children: (0, __CODEX_MUX_JSX__.jsx)("span", {
      className: "block h-full rounded-full",
      style: {
        width: `${width}%`,
        backgroundColor: codexMuxUsageBarColor(width),
      },
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

// A subscription's name is display-only. The label stored with the account is written
// once, when the account is added, so it would keep the language that was active then.
// Derive the name from the interface language instead, and remember each account's
// position in the full list so every view numbers a subscription the same way.
function codexMuxRememberAccountOrder(accounts) {
  const ordinals = {};
  accounts.forEach((account, index) => {
    ordinals[account.id] = index + 1;
  });
  globalThis.__codexMuxAccountOrdinals = ordinals;
}

function codexMuxAccountLabel(intl, account, fallbackIndex) {
  if (account.controller) return intl.formatMessage(CODEX_MUX_MESSAGES.primary);
  const ordinals = globalThis.__codexMuxAccountOrdinals || {};
  const index =
    ordinals[account.id] ?? (fallbackIndex == null ? null : fallbackIndex + 1);
  return index == null
    ? account.label
    : intl.formatMessage(CODEX_MUX_MESSAGES.subscriptionLabel, { index });
}

function codexMuxAccountTitle(intl, account, fallbackIndex) {
  const label = codexMuxAccountLabel(intl, account, fallbackIndex);
  return account.planLabel ? `${label} · ${account.planLabel}` : label;
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

function CodexMuxUseConnectedAccounts() {
  const [accounts, setAccounts] = __CODEX_MUX_REACT__.useState(
    globalThis.__codexMuxCombinedProfileAccounts || [],
  );
  __CODEX_MUX_REACT__.useEffect(() => {
    let live = true;
    codexMuxRequest("/accounts")
      .then((result) => {
        if (!live) return;
        codexMuxRememberAccountOrder(result.accounts || []);
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

// The Profile settings statistics are pooled across every subscription. One
// account's identity would not convey that, so stack the connected accounts instead.
// With a single connection, leave the native header alone.
function CodexMuxProfileAvatarStack() {
  const intl = __CODEX_MUX_INTL__();
  const accounts = CodexMuxUseConnectedAccounts();
  if (accounts.length < 2) return null;
  // The header reserves a single avatar-sized slot. A plain row of avatars is a flex
  // child of that slot, so it is compressed into ellipses. Keep the slot's own box and
  // centre the wider stack over it instead, and stop each avatar from shrinking.
  return (0, __CODEX_MUX_JSX__.jsx)("div", {
    className: "relative",
    style: { width: 80, height: 80, flexShrink: 0 },
    "aria-label": intl.formatMessage(CODEX_MUX_MESSAGES.connectedCount, {
      count: accounts.length,
    }),
    children: (0, __CODEX_MUX_JSX__.jsx)("div", {
      className: "flex items-center",
      style: {
        // Without an explicit width the absolute row shrinks to the slot and the
        // avatars overflow to one side, which offsets the centring translate.
        width: "max-content",
        position: "absolute",
        left: "50%",
        top: "50%",
        transform: "translate(-50%, -50%)",
      },
      children: accounts.map((account, index) =>
        (0, __CODEX_MUX_JSX__.jsx)(
          "span",
          {
            className:
              "rounded-full border-4 border-token-bg-primary transition-transform hover:z-10 hover:scale-105",
            style: {
              position: "relative",
              flexShrink: 0,
              marginLeft: index === 0 ? 0 : -20,
              zIndex: index,
            },
            title: codexMuxAccountTitle(intl, account, index),
            children: (0, __CODEX_MUX_JSX__.jsx)(CodexMuxAccountAvatar, {
              imageUrl: account.profileImageUrl,
              label: codexMuxAccountLabel(intl, account, index),
              className: "size-20 shrink-0",
            }),
          },
          account.id,
        ),
      ),
    }),
  });
}

function CodexMuxProfileDisplayName() {
  const intl = __CODEX_MUX_INTL__();
  const accounts = CodexMuxUseConnectedAccounts();
  if (accounts.length < 2) return null;
  return intl.formatMessage(CODEX_MUX_MESSAGES.combinedProfile);
}

function CodexMuxProfileUsername() {
  const intl = __CODEX_MUX_INTL__();
  const accounts = CodexMuxUseConnectedAccounts();
  if (accounts.length < 2) return null;
  return intl.formatMessage(CODEX_MUX_MESSAGES.connectedCount, {
    count: accounts.length,
  });
}

function CodexMuxPluginScope() {
  const intl = __CODEX_MUX_INTL__();
  const [accounts, setAccounts] = __CODEX_MUX_REACT__.useState([]);
  const [selectedId, setSelectedId] = __CODEX_MUX_REACT__.useState("primary");
  const [loading, setLoading] = __CODEX_MUX_REACT__.useState(true);
  const queryClient = lt();
  __CODEX_MUX_REACT__.useEffect(() => {
    let live = true;
    codexMuxRequest("/accounts")
      .then((result) => {
        if (!live) return;
        codexMuxRememberAccountOrder(result.accounts || []);
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
            children: intl.formatMessage(CODEX_MUX_MESSAGES.pluginConnections),
          }),
          (0, __CODEX_MUX_JSX__.jsx)("div", {
            className: "mt-0.5 text-xs text-token-text-secondary",
            children: selected
              ? intl.formatMessage(CODEX_MUX_MESSAGES.pluginScopeFor, {
                  label: codexMuxAccountLabel(intl, selected, null),
                })
              : intl.formatMessage(CODEX_MUX_MESSAGES.pluginScopeNone),
          }),
        ],
      }),
      loading
        ? (0, __CODEX_MUX_JSX__.jsx)("div", {
            className: "mt-3 px-1 text-sm text-token-text-tertiary",
            children: intl.formatMessage(CODEX_MUX_MESSAGES.loadingSubscriptions),
          })
        : (0, __CODEX_MUX_JSX__.jsx)("div", {
            className: "mt-3 flex flex-wrap gap-2",
            children: accounts.map((account, index) => {
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
                      label: codexMuxAccountLabel(intl, account, index),
                      className: "size-7",
                    }),
                    (0, __CODEX_MUX_JSX__.jsx)("span", {
                      children: codexMuxAccountTitle(intl, account, index),
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
globalThis.CodexMuxMessages = CODEX_MUX_MESSAGES;
globalThis.codexMuxProfileData = codexMuxProfileData;
globalThis.CodexMuxProfileAvatarStack = () =>
  (0, __CODEX_MUX_JSX__.jsx)(CodexMuxProfileAvatarStack, {});
globalThis.CodexMuxProfileDisplayName = () =>
  (0, __CODEX_MUX_JSX__.jsx)(CodexMuxProfileDisplayName, {});
globalThis.CodexMuxProfileUsername = () =>
  (0, __CODEX_MUX_JSX__.jsx)(CodexMuxProfileUsername, {});
globalThis.CodexMuxPluginScope = () =>
  (0, __CODEX_MUX_JSX__.jsx)(CodexMuxPluginScope, {});
