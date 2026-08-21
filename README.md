
<img width="900"  alt="hell" src="https://github.com/user-attachments/assets/d0942a23-2326-431f-b347-e5d6413c8b71" />

# SetFree

**Use the coding agent you love with any LLM.**

```
setfree claude
```

And you get your usual `claude`, but with whatever model you want.



## Why

Claude Code, Codex, Gemini CLI, Aider: good interfaces, all of them. But each one assumes exactly one provider, one auth flow, one set of models. Want to point Claude Code at a company gateway or a local proxy instead? Enjoy hand-rolling environment variables, then redoing it every time something changes.

The CLI you like and the model you use should be two different decisions. SetFree just makes that true.


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
setfree codex .
```

Everything after the CLI name goes straight through, untouched:

```sh
setfree claude --dangerously-skip-permissions
setfree codex . --full-auto
```

First run, with no gateway configured, SetFree asks once:

```

Welcome to SetFree.

No LLM gateway is configured yet. Let's connect one.

✓ Settings saved

Saved as your default gateway.

Launching Claude Code...
```

Every run after that is silent: straight to `claude`, nothing printed, nothing asked. A wrapper you notice every time is a wrapper that's in your way.

Setup only happens on a real terminal. In scripts or CI, set `SETFREE_BASE_URL` and `SETFREE_API_KEY` instead. SetFree fails fast with a clear message rather than hanging on a prompt nobody's there to answer.

## Coding CLIs

Claude Code and Codex work today. Gemini CLI and Aider get detected if they're installed and show up on the landing screen, but they politely decline to launch until someone builds an adapter for them. No pretending.

## Gateway configuration



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
