# SetFree

**Use the coding agent you love with any LLM.**

```
setfree claude
```

That's it. That's `claude`, the real Claude Code binary, running exactly like it always has — except its endpoint, credentials, and model are now whatever *you* configured, not whatever ships in the box.

SetFree doesn't touch your terminal experience, your keybindings, or your `.claude` config. It just launches the real CLI with the right environment already wired up.

Claude Code stays Claude Code. Codex stays Codex. You just stop being locked to one backend to use them.

---

## Why

Coding CLIs and LLM backends got bundled together for no good reason.

Claude Code, Codex, Gemini CLI, Aider — they're genuinely good interfaces. Good keybindings, good context handling, good agentic loops. But each one assumes you're talking to exactly one provider, through exactly one auth flow, to exactly one set of models. Want to point Claude Code at a company gateway, an OpenRouter model, or a local proxy instead? You're hand-rolling environment variables, hunting through docs, and re-doing it every time something changes.

**The interface and the model should be two separate decisions.**

You pick the CLI because you like how it works. You pick the model and infra because of cost, latency, compliance, or just what's good this month. Neither choice should hold the other hostage.

SetFree is the thin, boring layer that makes that true — nothing more.

## How it works

```
Claude Code ─┐
Codex ───────┤
Gemini CLI ──┤──  SetFree  ──  gateway of your choice  ──  model of your choice
Aider ───────┤
Other CLIs ──┘
```

When you run `setfree claude`, here's the whole trick:

1. SetFree finds your installed `claude` binary on `PATH`.
2. It resolves your configured gateway (and model, if you've pinned one) into whatever that specific CLI expects — environment variables for Claude Code, `-c` config overrides for Codex.
3. It replaces itself with the real binary (`exec`, not a subprocess) — same stdin/stdout/stderr, same working directory, same exit code, same signal handling a direct invocation would have.

No shim, no proxy, no man-in-the-middle process babysitting your session. Once it launches, SetFree is out of the picture entirely — you're just running Claude Code.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mindsdb/setfree/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/mindsdb/setfree/main/install.ps1 | iex
```

Single native binary, built in Go. No Python, no Node, no Go toolchain required to run it. Homebrew, Scoop, and WinGet formulas are on the roadmap — not yet published, so don't go looking for `brew install setfree` today.

## Usage

```sh
setfree claude       # launch Claude Code through your configured gateway
setfree claude .     # arguments pass straight through
setfree codex .
```

Anything after the CLI name is forwarded to the underlying tool untouched:

```sh
setfree claude --dangerously-skip-permissions
setfree codex . --full-auto
```

The first time you run either, SetFree notices there's no gateway configured yet and asks, once:

```
 ,-.
(o.o)
|>-<|
/   \

Welcome to SetFree.

No LLM gateway is configured yet. Let's connect one.

Base URL
> https://llm.example.com

API key
>

✓ Settings saved

Saved as your default gateway.

Launching Claude Code...
```

Every run after that is silent — straight to `claude`, no prompts, no banner. That's deliberate: a wrapper you notice on every launch is a wrapper that's in your way.

Setup only runs when stdin is a real terminal. In scripts or CI, set `SETFREE_BASE_URL` / `SETFREE_API_KEY` instead (see [Configuration](#configuration)) — SetFree fails fast with a clear message rather than hanging on a prompt no one will answer.

## Supported CLIs

| CLI | Status |
|---|---|
| Claude Code (`claude`) | ✅ Implemented |
| Codex (`codex`) | ✅ Implemented |
| Gemini CLI (`gemini`) | 📋 Detected, not launchable yet |
| Aider (`aider`) | 📋 Detected, not launchable yet |

Gemini CLI and Aider show up in `setfree`'s landing screen if they're installed, but there's no adapter for them yet — running `setfree gemini` tells you that plainly instead of pretending to work.

## Gateways

SetFree doesn't ship gateway-specific integrations today — it needs one thing: a base URL and a key for an endpoint that speaks the protocol your coding CLI already expects (Anthropic-style for Claude Code, OpenAI Responses API for Codex). In practice that covers most OpenRouter, LiteLLM, and self-hosted setups, since they speak one of those dialects already. A local proxy on `http://localhost:...` works the same way.

If a specific gateway needs quirkier handling (custom headers, a different auth scheme), that's the extension point a future gateway-specific adapter would fill — see [Roadmap](#roadmap).

## Configuration

SetFree keeps two small files in your per-user config directory (`os.UserConfigDir()` — e.g. `~/Library/Application Support/setfree` on macOS, `~/.config/setfree` on Linux, `%AppData%\setfree` on Windows):

**`config.toml`** — everything non-secret:

```toml
version = 1
default_gateway = "default"

[gateways.default]
base_url = "https://llm.example.com"

[cli.claude]
model = "kimi-k2.5"

[cli.codex]
model = "gpt-5"
```

**`credentials.toml`** — API keys, kept in a separate file with `0600` permissions so a `cat config.toml` to debug your setup never dumps a secret to your terminal.

Manage it without hand-editing either file:

```sh
setfree config          # interactive: view and edit base URL / API key
setfree config show      # print current status (keys shown as "configured", never in full)
setfree config set --base-url https://gw.example.com --api-key sk-...
setfree config reset     # clear SetFree's own config — never touches Claude/Codex's
```

For scripts and CI, environment variables override saved config for a single run:

| Variable | Overrides |
|---|---|
| `SETFREE_BASE_URL` | gateway base URL |
| `SETFREE_API_KEY` | gateway API key |
| `SETFREE_MODEL` | model for this run |
| `SETFREE_GATEWAY` | which saved gateway to use, by name |

Precedence: SetFree env vars → saved config → interactive setup (which only triggers on a real terminal).

## Architecture

SetFree is built around one small abstraction: a **CLI adapter**.

```go
type Adapter interface {
	Name() string          // "claude" — what you type on the command line
	DisplayName() string   // "Claude Code"
	BinaryNames() []string // executables to look for on PATH
	Build(baseEnv []string, resolved gateway.Resolved) (Build, error)
}
```

`Build` turns a normalized `Gateway{BaseURL, APIKey}` plus an optional model into whatever that specific CLI needs — environment variables, extra argv, or both. Everything vendor-specific lives inside one adapter; nothing else in SetFree needs to know how Claude Code or Codex actually read their config.

```
internal/
  adapters/       adapter interface + registry, plus claude/ and codex/
  config/         SetFree's own non-secret settings (config.toml)
  secrets/        API key storage, isolated from config on purpose
  gateway/        the normalized Gateway type + resolution precedence
  detect/         which known coding CLIs are installed
  launcher/       process replacement (exec on Unix, spawn+wait on Windows)
  terminal/       TTY/color detection, hidden password input
  ui/             the robot, landing screen, setup and config prompts
  app/            command dispatch — the only place that ties it together
```

No daemon, no background process, no persistent state beyond the two config files above.

## Adding a CLI adapter

Look at `internal/adapters/claude/claude.go` or `internal/adapters/codex/codex.go` — each is under 60 lines. To add one:

1. Create `internal/adapters/<name>/`.
2. Implement `Adapter`, register it in an `init()` via `adapters.Register(...)`.
3. Blank-import the package from `internal/app/app.go`.
4. Add it to `internal/detect.List` with `Supported: true`.
5. Write a test asserting the environment/args `Build` produces for a known gateway — see the existing adapter tests for the pattern.

If your favorite CLI isn't listed above, this is the fastest path to getting it supported.

## Security philosophy

- API keys live in `credentials.toml`, separate from `config.toml`, created with `0600` permissions on Unix. `setfree config show` never prints one — only whether it's configured.
- Codex's API key is never placed on argv (visible to any user via `ps`); it's passed through the child's environment and referenced by name instead.
- SetFree builds a child process environment and gets out of the way — it doesn't proxy your traffic, phone home, or sit in the request path once the real CLI is running.
- No modified or repackaged coding CLI binaries, ever. SetFree launches the CLI you already installed, unmodified, and never rewrites its native config.

This is a configuration and interoperability tool. It doesn't circumvent authentication, bypass licensing, or spoof a provider — it just lets you point a great interface at a backend you're already authorized to use.

## Roadmap

- [ ] Gemini CLI and Aider adapters
- [ ] `setfree gateway add|list|use` for multiple named gateways (the config format already supports it)
- [ ] `--gateway` / `--model` flags for one-off overrides
- [ ] Gateway-specific adapters for providers that need non-generic handling
- [ ] Homebrew / Scoop / WinGet distribution
- [ ] OS keychain-backed credential storage as an alternative to `credentials.toml`

Roadmap order isn't a promise — it's a reflection of what's likely to get picked up first. Open an issue if your use case should jump the queue.

## Contributing

SetFree is intentionally small: adapters in, exec out. That makes it a good project to contribute to even in short bursts.

Good first contributions:

- A new CLI adapter (see above)
- Platform-specific packaging (Scoop, WinGet, Homebrew formula)
- Filling gaps in the config schema as multi-gateway support lands

Open an issue before large changes so we can agree on shape first. Everything else — fork, branch, PR.

## License

MIT — see [LICENSE](LICENSE).

---

**Set your coding agents free.**
Any coding CLI. Any gateway. Any model.
