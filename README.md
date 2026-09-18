
<img width="900"  alt="hell" src="https://github.com/user-attachments/assets/d0942a23-2326-431f-b347-e5d6413c8b71" />

# SetFree

**Use the coding agent you love for FREE!! or with any LLM.**

```
setfree claude
```

And you get your usual `claude`, but powered with MindsHub-Air, a free coding model + the best models (Muse Spark, GLM, Deepseek, Astra,.. etc).



## Why

Claude Code, Codex, VS-Code, Gemini CLI, Aider: good interfaces, all of them. But each one assumes exactly one provider, one auth flow, one set of models. You can set them free.

The CLI you like and the model you use should be two different decisions. SetFree makes them free to use and opens your CLI to the best models for coding.


## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mindsdb/setfree/main/install.sh | sh
```

Windows:

```powershell
irm https://raw.githubusercontent.com/mindsdb/setfree/main/install.ps1 | iex
```

One binary. No dependencies.

## Usage

```sh
setfree claude
setfree codex .
setfree vscode .
setfree hermes
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


## Coding CLIs

Claude Code, Codex, VSCode and Hermes Agent work today. 


## Gateway configuration

Want to use it with your own LLM-Gateway?

Manage both without opening either file:

```sh
setfree config       # interactive: view and edit base URL / API key
setfree config show
setfree config set --base-url https://gw.example.com --api-key sk-...
setfree config reset
setfree usage
setfree usage --lifetime
```

For the MindsHub gateway, `usage` shows token counts and billed cost grouped
by model. Add `--json` for machine-readable output. Usage checks are not
available for other gateways because their accounting APIs differ.

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


## Contributing

Adapters in, exec out. Small enough to contribute to in an evening.

Good first PRs: a new CLI adapter, packaging for your platform, or closing gaps in the config schema as multi-gateway support lands.

Open an issue before anything big. Everything else: fork, branch, PR.

## License

MIT. See [LICENSE](LICENSE).

---

**Set your coding agents free.**
Any coding CLI. Any gateway. Any model.
