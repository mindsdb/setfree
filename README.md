# SetFree

**Use the coding agent you love with any LLM.**

```
setfree claude
```

That's `claude`. The actual binary. Running with whatever endpoint, key, and model you configured instead of the one it shipped with.

SetFree doesn't touch your terminal, your keybindings, or your Claude config. It just hands the real CLI the environment it needs and gets out of the way.

## Why

Claude Code, Codex, Gemini CLI, Aider: good interfaces, all of them. But each one assumes exactly one provider, one auth flow, one set of models. Want to point Claude Code at a company gateway or a local proxy instead? Enjoy hand-rolling environment variables, then redoing it every time something changes.

The CLI you like and the model you use should be two different decisions. SetFree just makes that true.

## How it works

```
Claude Code ─┐
Codex ───────┤
Gemini CLI ──┤──  SetFree  ──  your gateway  ──  your model
Aider ───────┤
```

Run `setfree claude` and here's the whole trick:

1. SetFree finds your installed `claude` binary.
2. It turns your configured gateway into whatever that CLI actually expects: environment variables for Claude Code, config flags for Codex.
3. It becomes that binary. A real `exec`, not a wrapper sitting on top. Same stdin, same stdout, same exit code.

No proxy in the middle, no process babysitting your session. Once it launches, SetFree is gone and you're just running Claude Code.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mindsdb/setfree/main/install.sh | sh
```

Windows:

```powershell
irm https://raw.githubusercontent.com/mindsdb/setfree/main/install.ps1 | iex
```

One binary. No Python, no Node, no Go required to run it. Homebrew and friends are on the way, not here yet.

SetFree also keeps itself current on its own. Once a day it checks whether main has moved on without it and quietly installs the newer build. No `setfree update` to remember, no changelog to read. Set `SETFREE_NO_AUTOUPDATE=1` if you'd rather pin a version yourself.

## Usage

```sh
setfree claude
setfree claude .
setfree codex .
```

Everything after the CLI name goes straight through, untouched:

```sh
setfree claude --dangerously-skip-permissions
setfree codex . --full-auto
```

First run, with no gateway configured, SetFree asks once:

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

Every run after that is silent: straight to `claude`, nothing printed, nothing asked. A wrapper you notice every time is a wrapper that's in your way.

Setup only happens on a real terminal. In scripts or CI, set `SETFREE_BASE_URL` and `SETFREE_API_KEY` instead. SetFree fails fast with a clear message rather than hanging on a prompt nobody's there to answer.

## Coding CLIs

Claude Code and Codex work today. Gemini CLI and Aider get detected if they're installed and show up on the landing screen, but they politely decline to launch until someone builds an adapter for them. No pretending.

## Gateways

SetFree doesn't ship gateway-specific integrations. It needs one thing: a base URL and a key for an endpoint that speaks the protocol your CLI already expects (Anthropic-style for Claude Code, the OpenAI Responses API for Codex). That already covers most OpenRouter, LiteLLM, and self-hosted setups, plus a local proxy on `localhost`.

If a gateway needs something weirder, custom headers or a different auth scheme, that's what a gateway-specific adapter would handle later. See the roadmap.

## Configuration

SetFree keeps two small files in your normal per-user config directory: `~/Library/Application Support/setfree` on macOS, `~/.config/setfree` on Linux, `%AppData%\setfree` on Windows.

`config.toml` holds everything non-secret:

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

`credentials.toml` holds the API key, alone, at `0600`, so debugging your config with `cat` never leaks a secret onto your screen.

Manage both without opening either file:

```sh
setfree config       # interactive: view and edit base URL / API key
setfree config show
setfree config set --base-url https://gw.example.com --api-key sk-...
setfree config reset
```

Environment variables override saved config for a single run, handy in scripts and CI:

| Variable | Overrides |
|---|---|
| `SETFREE_BASE_URL` | gateway base URL |
| `SETFREE_API_KEY` | gateway API key |
| `SETFREE_MODEL` | model for this run |
| `SETFREE_GATEWAY` | which saved gateway to use |

Order of precedence: env vars, then saved config, then interactive setup (terminal only).

## Architecture

One abstraction: a CLI adapter.

```go
type Adapter interface {
	Name() string          // "claude"
	DisplayName() string   // "Claude Code"
	BinaryNames() []string // executables to look for on PATH
	Build(baseEnv []string, resolved gateway.Resolved) (Build, error)
}
```

`Build` takes a gateway (base URL, key, optional model) and returns whatever env vars or extra args that CLI needs. Everything vendor-specific stays inside one adapter. Nothing else in SetFree needs to know how Claude Code or Codex actually read their config.

```
internal/
  adapters/   adapter interface and registry, plus claude/ and codex/
  config/     SetFree's own settings, non-secret
  secrets/    API keys, kept separate on purpose
  gateway/    the normalized Gateway type and resolution order
  detect/     which known CLIs are installed
  launcher/   exec on Unix, spawn and wait on Windows
  terminal/   TTY and color detection, hidden password input
  ui/         the robot, the landing screen, setup and config prompts
  app/        command dispatch, the one place it all meets
```

No daemon, no background process, nothing persisted beyond the two config files above.

## Adding a CLI adapter

Read `internal/adapters/claude/claude.go` or `internal/adapters/codex/codex.go`. Each is under 60 lines. To add your own:

1. Create `internal/adapters/<name>/`.
2. Implement `Adapter`, register it in `init()` with `adapters.Register(...)`.
3. Blank-import the package from `internal/app/app.go`.
4. Add it to `internal/detect.List` with `Supported: true`.
5. Write a test for what `Build` produces given a known gateway.

Missing your favorite CLI? This is the fast path to fixing that yourself.

## Security

- API keys live in `credentials.toml`, apart from `config.toml`, `0600` on Unix. `setfree config show` tells you a key is configured. It never shows you the key.
- Codex's key never touches argv, where anyone on the machine could read it with `ps`. It travels through the child process's environment instead.
- SetFree builds an environment and steps aside. It doesn't proxy your traffic, phone home, or sit in the request path once the CLI is running.
- No modified binaries, ever. SetFree launches the CLI you installed, exactly as it is, and never rewrites its native config.
- Self-updates are checked against `checksums.txt` before anything gets installed. A mismatch aborts the update and leaves your current binary untouched.

This is a configuration tool, not a bypass. It doesn't touch authentication or licensing, and it doesn't spoof a provider. It just points a good interface at a backend you're already allowed to use.

## Roadmap

- Gemini CLI and Aider adapters
- Multiple named gateways (`setfree gateway add|list|use`); the config format already supports it
- `--gateway` / `--model` flags for one-off overrides
- Gateway-specific adapters where the generic path isn't enough
- Homebrew, Scoop, WinGet
- OS keychain storage as an alternative to `credentials.toml`

Not a promise, just the order things are likely to land. Open an issue if yours should jump the line.

## Contributing

Adapters in, exec out. Small enough to contribute to in an evening.

Good first PRs: a new CLI adapter, packaging for your platform, or closing gaps in the config schema as multi-gateway support lands.

Open an issue before anything big. Everything else: fork, branch, PR.

## License

MIT. See [LICENSE](LICENSE).

---

**Set your coding agents free.**
Any coding CLI. Any gateway. Any model.
