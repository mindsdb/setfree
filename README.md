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

You pick the CLI because you like how it works. You pick the model and infra because of cost, latency, compliance, or just what's good this month. Neither choice should hostage the other.

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

1. SetFree locates your installed `claude` binary.
2. It resolves your configured gateway and model into the specific environment variables, base URLs, and auth that CLI expects.
3. It `exec`s the real binary with that environment — passing through your working directory, stdin/stdout/stderr, exit code, and signals untouched.

No shims, no forked CLI, no man-in-the-middle process babysitting your session. Once it launches, SetFree is out of the picture — you're just running Claude Code.

## Install

```sh
curl -fsSL https://setfree.dev/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://setfree.dev/install.ps1 | iex
```

Single static binary. No Python, no Node, no Go toolchain required to run it. Homebrew, Scoop, and WinGet formulas are on the roadmap — not yet published, so don't go looking for `brew install setfree` today.

## Usage

```sh
setfree claude              # launch Claude Code through your default gateway
setfree codex .              # launch Codex in the current directory
setfree gemini --model gpt-5 # override the model for this run
```

Anything after the CLI name passes straight through to the underlying tool:

```sh
setfree claude --dangerously-skip-permissions
setfree codex . --full-auto
```

Explicit gateway and model overrides are on the near-term roadmap:

```sh
setfree claude --gateway openrouter
setfree claude --gateway https://llm.example.com
setfree codex --model claude-sonnet
```

*(Illustrative — see [Status](#status) for what actually ships today.)*

## Configuration

SetFree reads a single config file, checked in nowhere, full of secrets nowhere:

`~/.setfree/config.toml`

```toml
default_gateway = "my-gateway"

[gateways.my-gateway]
base_url = "https://llm.example.com"
api_key_env = "MY_LLM_API_KEY"

[cli.claude]
model = "kimi-k2.5"

[cli.codex]
model = "gpt-5"
```

Credentials are referenced by environment variable name (`api_key_env`), never written to disk in plaintext. Pull them from your shell profile, a secrets manager, or whatever credential store you already trust — SetFree just reads the pointer.

This schema is early and will change. Treat it as a sketch of the shape, not a stable contract yet.

## Status

SetFree is young. Here's what's real versus what's coming.

**Coding CLIs**

| CLI | Status |
|---|---|
| Claude Code | 🚧 In progress |
| Codex | 🚧 In progress |
| Gemini CLI | 📋 Planned |
| Aider | 📋 Planned |

**Gateways**

| Gateway | Status |
|---|---|
| Custom / OpenAI-compatible URL | 🚧 In progress |
| OpenRouter | 📋 Planned |
| LiteLLM | 📋 Planned |
| MindsHub | 📋 Planned |
| Enterprise / self-hosted gateways | 📋 Planned |

🚧 = actively being built, expect rough edges. 📋 = designed, not implemented. Nothing here is vaporware marketing — if it's not in one of these tables as shipping, it doesn't work yet.

## Architecture

SetFree is built around two small, swappable abstractions:

**CLI adapters** know how one specific coding tool wants to be configured — which environment variables it reads, what shape it expects its base URL in, how it expects auth to be presented. A `claude` adapter and a `codex` adapter don't need to agree on anything internally; they just need to speak SetFree's normalized config on one side and the CLI's native expectations on the other.

**Gateway adapters** know how to talk to one specific LLM endpoint style — building the right base URL, headers, and credential wiring for OpenRouter, a LiteLLM instance, or a bare OpenAI-compatible URL.

SetFree's job is entirely in the middle:

```
CLI adapter  →  normalized config  →  gateway adapter  →  environment for exec()
```

That's the whole system. No daemon, no background process, no persistent state beyond your config file.

## Adding a CLI adapter

A CLI adapter is a small Go package that answers one question: *given a normalized model + gateway config, what environment does this CLI need to see?*

Roughly:

1. Implement the adapter interface — binary discovery, env var mapping, arg passthrough rules.
2. Register it under `cli/<name>`.
3. Add a fixture/test asserting the expected environment for a known config.

If your favorite CLI isn't listed above, this is the fastest path to getting it supported — PRs adding adapters are exactly what this project wants.

## Adding a gateway adapter

Same idea, other side of the wire: given SetFree's normalized gateway config (`base_url`, `api_key_env`, provider-specific quirks), produce the concrete env vars and headers a CLI adapter can consume.

Contribution shape is the same as above — implement the interface, register under `gateway/<name>`, add a test fixture. Gateways with genuinely different auth or endpoint conventions are the most useful contributions right now.

## Security philosophy

SetFree doesn't ask you to trust it with less than you already trust your shell.

- Secrets are referenced by environment variable name, never stored or logged by SetFree in plaintext.
- SetFree does not phone home, does not proxy your traffic, and does not sit in the request path once your CLI has launched. It builds an environment and gets out of the way.
- No modified or repackaged coding CLI binaries — ever. SetFree launches the CLI you already installed, unmodified.

This is a configuration and interoperability tool. It doesn't circumvent authentication, bypass licensing, or spoof a provider — it just lets you point a great interface at the backend you're already authorized to use.

## Roadmap

- [ ] Stable `--gateway` / `--model` CLI flags
- [ ] Gemini CLI and Aider adapters
- [ ] OpenRouter, LiteLLM, and MindsHub gateway adapters
- [ ] Homebrew / Scoop / WinGet distribution
- [ ] Config validation and `setfree doctor`
- [ ] Per-project config overrides

Roadmap order isn't a promise — it's a reflection of what's likely to get picked up first. Open an issue if your use case should jump the queue.

## Contributing

SetFree is intentionally small in scope: adapters in, exec out. That makes it a good project to contribute to even in short bursts.

Good first contributions:

- A new CLI or gateway adapter (see above)
- Filling gaps in the config schema
- Platform-specific packaging (Scoop, WinGet, Homebrew formula)

Open an issue before large changes so we can agree on shape first. Everything else — fork, branch, PR.

## License

MIT — see [LICENSE](LICENSE).

---

**Set your coding agents free.**
Any coding CLI. Any gateway. Any model.
