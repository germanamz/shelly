# Better Context: Token Efficiency & Attention for AI Agents

## Research: Best Practices for Token Efficiency & Attention in AI Agents

### Core Problem

LLMs have a **finite attention budget**. Every token introduced depletes it. Research on 18 leading models shows **context rot** — measurable, non-uniform performance degradation as input context expands. This is inherent to the transformer architecture where every token attends to every other (n² relationships). Additionally, the **"lost in the middle" effect** means LLMs recall information at the beginning and end of context far better than content buried in the middle.

### The Six Pillars of Token Efficiency

From research across Anthropic's engineering blog, Chroma's context rot study, Google's ADK architecture, and multiple practitioner writeups, the proven techniques cluster into six categories:

| Pillar | Technique | Impact |
|--------|-----------|--------|
| **1. Minimum Viable Context** | Only include high-signal tokens the agent actually needs | Foundational principle |
| **2. Tiered Storage** | Working context (ephemeral) / Session (durable log) / Memory (searchable long-term) / Artifacts (by reference) | Prevents context bloat |
| **3. Graduated Compaction** | Multi-level summarization with sliding windows, not just full-history-to-summary | Preserves recent detail |
| **4. Tool Result Offloading** | Store large outputs externally, keep lightweight references in context | 60-80% token reduction on tool-heavy workflows |
| **5. Prompt Caching Alignment** | Static content first (system instructions), dynamic content last | 45-80% cost reduction |
| **6. Agent Isolation** | Sub-agents with clean context windows return condensed summaries | Prevents context pollution |

### Counter-intuitive Finding

Chroma's research found that **logically coherent surrounding context hurts retrieval performance**. Shuffling the haystack consistently improves results. This suggests that structured/sectioned context (XML tags, clear headers) outperforms flowing prose for agent system prompts.

---

## Analysis: Shelly's Current State vs. Best Practices

### What Shelly Does Well

- **Pluggable effects system** — compaction is opt-in, configurable, extensible
- **On-demand skill loading** — skills with descriptions use `load_skill` tool instead of embedding
- **Project context assembly** — multi-source with caching (external + curated + generated)
- **Sub-agent delegation** — fresh agents per delegation with depth limiting
- **Post-hoc usage tracking** — token counts recorded per call
- **Clean architecture** — each layer has well-defined responsibilities

### Where Shelly Falls Short

#### 1. Unbounded Message Accumulation

**Current**: Messages accumulate forever until compaction triggers. No intermediate pruning.
**Best practice**: Graduated compaction — keep recent N messages in full detail, older messages in summarized form, with sliding window summarization at configurable thresholds.

#### 2. Single-Level Compaction

**Current**: `CompactEffect` does full-history → one summary block. Binary: everything or nothing.
**Best practice**: Multi-level compaction — recent turns detailed, medium-age turns condensed, old turns highly summarized. Google's ADK uses async sliding window summarization writing summaries back as session events.

#### 3. All Tools Sent Every Iteration

**Current**: Every available tool (with full JSON schema) is included in every LLM call. No filtering, no prioritization.
**Best practice**: Minimum viable toolset per call. Tools consume significant tokens (name + description + full JSON schema each). Anthropic recommends avoiding "bloated tool sets with overlapping functionality."

#### 4. No Tool Result Offloading

**Current**: Tool results are kept in full in the message history (truncated only during compaction rendering at 200/500 chars).
**Best practice**: Anthropic identifies clearing tool results as the "safe, lightest touch" form of compaction. Store full results externally, keep only summaries or references in context.

#### 5. Static System Prompt

**Current**: System prompt built once in `agent.Init()`, never updated. Contains everything: identity + instructions + full project context + all skill descriptions + agent directory.
**Best practice**: Context compilation via ordered processors. Separate stable prefix (identity, core instructions) from variable content (project context, skills, agent list). Only include what's relevant to current task phase.

#### 6. No Pre-Call Token Estimation

**Current**: `UsageTracker` records tokens after calls. CompactEffect checks previous call's `InputTokens` to decide if compaction needed.
**Best practice**: Predict token usage before sending to avoid wasted calls that exceed limits. Tokenizer-based estimation or heuristic character-to-token ratios.

#### 7. No Prompt Cache Alignment

**Current**: System prompt content ordering is not optimized for provider caching. Dynamic content (agent directory, skill list) mixed with static content (identity, instructions).
**Best practice**: Static content first, dynamic content last. Anthropic's prompt caching gives 75% cheaper processing for cached prefixes.

#### 8. No Structured External Memory

**Current**: No mechanism for agents to maintain persistent notes across compaction events.
**Best practice**: Structured note-taking (NOTES.md-style files) that survive context resets. Anthropic's Pokemon agent example shows agents developing strategic documentation that gets pulled back into context later.

#### 9. Conversation Translation During Delegation

**Current**: When delegating to sub-agents, the parent's context doesn't get reframed.
**Best practice**: Google's ADK reframes existing conversations so new agents don't misattribute prior actions — prior "Assistant" messages are recast as narrative context with attribution markers.

---

## Proposed Changes (Abstract, Framework-Level)

These follow Shelly's existing architecture patterns — effects, middleware, toolbox interfaces.

### Phase 1: Low-Hanging Fruit (High Impact, Low Risk)

1. **Tool Result Compaction Effect** — New effect that runs `PhaseAfterComplete`, replaces large tool results (above a configurable threshold) with truncated summaries while storing full results in an external reference. Uses existing `Effect` interface.

2. **System Prompt Sectioning** — Restructure `buildSystemPrompt()` to use clear section markers (XML tags or markdown headers) and order content for cache friendliness: `[identity] → [core instructions] → [project context] → [skills] → [agent directory]` with static content first.

3. **Tool Result Clearing on Compaction** — Before running the expensive summarization LLM call in `CompactEffect`, first try the "lightest touch": clear old tool results from messages. Only escalate to full summarization if still over threshold.

### Phase 2: Graduated Context Management

4. **Multi-Level Compaction** — Replace the binary full/compacted model with a graduated approach: recent messages (last N turns) kept in full, older messages summarized in progressively more condensed form. New effect or enhancement to existing `CompactEffect`.

5. **Token Budget Estimation** — Add a `TokenEstimator` interface to `modeladapter` that estimates token count for a chat + tools before sending. Use character-to-token heuristics (or provider-specific tokenizers). CompactEffect uses this for proactive rather than reactive compaction.

6. **Structured Note-Taking Tool** — New tool in `pkg/codingtoolbox/` (or a new package) that lets agents write/read persistent notes. Notes survive compaction and get re-injected into context when relevant. Uses existing `toolbox.Tool` interface.

### Phase 3: Advanced Optimization

7. **Dynamic Tool Filtering** — New middleware or effect that tracks tool usage patterns and filters the tool set sent to the LLM. Frequently-used tools always included; rarely-used tools available via a `list_tools`/`get_tool` pattern (similar to on-demand skills). Uses existing `ToolBox` interface.

8. **Context Compilation Pipeline** — Replace `buildSystemPrompt()` string concatenation with an ordered processor pipeline (inspired by Google ADK). Each processor is a function `(ctx, prompt) → prompt` that can add, remove, or transform sections. Enables per-iteration dynamic system prompts.

9. **Conversation Translation for Delegation** — When delegating to sub-agents via `delegate_to_agent`, reframe the parent's conversation history so the sub-agent gets a clean narrative context rather than raw message replay. Uses existing delegation infrastructure in `agent.go`.

10. **Proactive Memory Retrieval** — Before each LLM call, run similarity search against structured notes/memory and inject relevant snippets. Complements the structured note-taking tool from Phase 2.

---

## Sources

- [Effective context engineering for AI agents — Anthropic](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)
- [Context Rot — Chroma Research](https://research.trychroma.com/context-rot)
- [Architecting efficient context-aware multi-agent framework — Google](https://developers.googleblog.com/architecting-efficient-context-aware-multi-agent-framework-for-production/)
- [LLM Token Optimization — Redis](https://redis.io/blog/llm-token-optimization-speed-up-apps/)
- [Context Engineering: Token Optimization — FlowHunt](https://www.flowhunt.io/blog/context-engineering-ai-agents-token-optimization/)
- [Token Optimization Strategies for AI Agents — Elementor Engineers](https://medium.com/elementor-engineers/optimizing-token-usage-in-agent-based-assistants-ffd1822ece9c)
- [Top techniques to manage context lengths in LLMs — Agenta](https://agenta.ai/blog/top-6-techniques-to-manage-context-length-in-llms)
- [Optimizing Token Usage for AI Efficiency — SparkCo](https://sparkco.ai/blog/optimizing-token-usage-for-ai-efficiency-in-2025)
