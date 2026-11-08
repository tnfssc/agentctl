---
name: model-classification
description: How agentctl decides a proxy model is Anthropic, and why unknown models count as Anthropic
---

Decided 2026-11-09 (user: soft lock is enough; Claude models must not appear in non-Claude-Code pickers; a user who
types a model by hand is on their own).

- Fail safe: a model is non-Anthropic only with positive evidence. Hiding a Gemini model costs one allow line; showing
  a Claude model breaks the policy. Per-proxy `codex_allow` can clear unknown models but never overrides a positive
  Claude signal.
- Normalize first: strip `prefix/` (CPA `force-model-prefix`), `(…)` and `[1m]` suffixes.
- Claude if: `owned_by == anthropic`; or id/display name matches `\b(claude|anthropic|opus|sonnet|haiku|fable|mythos)\b`.
- Non-Claude only if `owned_by` is a single-vendor owner (openai, google, xai, …). `antigravity` serves `claude-*`
  under its own owner, OpenAI-compat providers carry the user's provider name, aliases copy the original's owner but
  can have any id. Codenames churn (e.g. daybreak-blue/red are OpenAI's cyber program aliases).
- With a management key (not done yet): models served by a `claude` credential (`auth-files/models?name=`) and the
  alias map (`oauth-model-alias`) close the Antigravity and alias holes.
- CPA's model registry (router-for-me/models, refreshed every 3h) is grouped by credential type with no vendor field.
  models.dev `canonical_model_id` (`anthropic/…`) is a usable extra positive signal; not wired in.
