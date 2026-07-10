# Patches applied to the raw models.dev api.json before aibridgepricesgen
# consumes it. The Makefile pipes the fetched payload through this filter
# (jq -f scripts/aibridgepricesgen/overrides.jq) and both generated outputs
# (prices.json and knownModelsGenerated.json) read the patched snapshot.
#
# Every patch guards its assumption about upstream, so a stale override
# fails the pipeline loudly instead of silently patching nothing.

# claude-sonnet-4-5: upstream advertises a 1M-token context window, which
# implies tiered context_over_200k pricing. Coder persists flat pricing only,
# so pin the context limit to the flat-priced 200k tier.
if .anthropic.models | has("claude-sonnet-4-5") then
  .anthropic.models."claude-sonnet-4-5".limit.context = 200000
else
  error("overrides.jq: claude-sonnet-4-5 gone from upstream; drop or update its context pin")
end

# claude-mythos-5: not listed upstream. Anthropic documents it as sharing
# claude-fable-5's specs and pricing, so inject it as a copy with its own
# id and display name.
| if .anthropic.models | has("claude-fable-5") then
    .anthropic.models."claude-mythos-5" = (
      .anthropic.models."claude-fable-5"
      | .id = "claude-mythos-5"
      | .name = "Claude Mythos 5"
    )
  else
    error("overrides.jq: claude-fable-5 gone from upstream; the claude-mythos-5 copy has no source")
  end
