---
name: codex-model-catalog
description: Shape and behavior of the Codex model_catalog_json file agentctl writes
---

From openai/codex rust-v0.167.0 (`protocol/src/openai_models.rs`, `model-provider/src/provider.rs`).

- `{"models":[ModelInfo]}`; replaces the bundled catalog entirely and disables /models fetching. A parse error or an
  empty list aborts Codex at startup, so never write an empty catalog.
- Required per entry: slug, display_name, supported_reasoning_levels[{effort,description}], shell_type, visibility
  (`list` shows in picker), supported_in_api, priority, support_verbosity, truncation_policy{mode,limit},
  experimental_supported_tools, base_instructions (or model_messages.instructions_template). Unknown fields ignored.
- Omit Claude entries rather than `visibility: hide`; hidden entries are still selectable from config.
- CPA serves a ready catalog at `GET /v1/models?client_version=<codex version>` (all models incl. Claude; unknown models
  cloned from its gpt-5.5 template). `codex debug models --bundled` prints the same shape and round-trips.
