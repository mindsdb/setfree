
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
| `SETFREE_VISION_MODEL` | vision model id; setting this enables the vision bridge |
| `SETFREE_VISION_BASE_URL` | separate endpoint for the vision model (optional) |
| `SETFREE_VISION_API_KEY` | API key for the vision model (optional) |
| `SETFREE_VISION_OFF` | `1` hard-disables the bridge for a run |

Order of precedence: env vars, then saved config, then interactive setup (terminal only).

## Vision bridge: a text-only model with eyes

Some of the best models for long coding sessions are text-only. You pick them for the context window, then hit a wall the moment an image lands in the conversation — the model throws a "not multimodal" error and the turn wedges.

The vision bridge fixes that when your gateway also serves a multimodal model. With it on, SetFree starts a tiny local proxy that the CLI talks to instead of the gateway directly. The proxy forwards everything unchanged **except** image content: each image is sent to your vision model, described, and replaced with a text caption before the request reaches the main model. The text model never sees a raw image block, so it never errors. Captions are cached to disk, so an image is described once ever, not once per turn.

Enable it by naming a vision model (saved or via `SETFREE_VISION_MODEL`):

```toml
# in config.toml's [vision] table
[vision]
model = "qwen3.5"
# base_url and api_key are optional; they default to your main gateway
```

or for a single run:

```sh
SETFREE_VISION_MODEL=qwen3.5 setfree claude
```

This is the one, deliberate exception to SetFree's "step aside, never sit in the request path" rule. It's strictly opt-in — with no vision model set, no proxy is started and nothing about the launch differs from before. The proxy runs on localhost, lives only as long as the CLI does, and never touches authentication or licensing. The CLI binary itself is still the one you installed, unmodified.

Two things the bridge can't do, because they'd require modifying the binary rather than proxying it: keep the original image for an on-demand "look closer" re-query, and let the text model ask the vision model about a specific detail it didn't caption well. The text model gets the caption and works from there. That's the tradeoff for staying out of the binary.

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
