# Wuthering Waves Wiki Ingestion Compliance

This document records the compliance decisions that govern how iris-bot
reads lore from external sources. It is paired with the machine-readable
registry in `internal/lore/source`, which enforces these rules at runtime.

## Source of truth

The canonical lore source is the Wuthering Waves Wiki on Fandom:

- Host: `wutheringwaves.fandom.com`
- Registry ID: `fandom_wutheringwaves`

No other wiki, mirror, or fan site is registered today. Ingestion code
must reject requests to unregistered hosts via
`Registry.ValidateAccess`, which fails closed.

## License and attribution

Fandom wiki content is licensed under **Creative Commons Attribution,
ShareAlike 3.0** (CC BY-SA 3.0). Every Discord response that surfaces
information sourced from the wiki MUST include a citation link back to
the original page.

Citation format rendered in Discord:

```
Title, wutheringwaves.fandom.com/wiki/Title
```

The attribution URL template stored in the registry is:

```
https://wutheringwaves.fandom.com/wiki/{page}
```

where `{page}` is the URL-encoded page title.

Generated responses must not remove author credit, strip license
notices, or reframe wiki content as original iris-bot content.

## Fandom Terms of Use

Summary of the relevant clauses from
<https://www.fandom.com/terms-of-use>:

- Automated access is permitted only when it respects site stability.
- Aggressive crawling, scraping at high rates, or reproducing the full
  site is prohibited.
- Prefer the MediaWiki API and the official XML dumps over scraping.
- Bots must identify themselves with a truthful User-Agent and a
  contact method.

iris-bot encodes these clauses as the Fandom source policy.

## Allowed access methods

The registry defines four `AccessMethod` values. For Fandom, three are
allowed, one is explicitly forbidden.

Preference ordering, highest first:

1. `mediawiki_api` (preferred): structured, rate-friendly, stable.
2. `xml_dump`: offline bulk ingestion; zero runtime load on Fandom.
3. `browser`: headless Camoufox for pages that require JavaScript
   rendering. Used sparingly and only when the API cannot deliver the
   required content.
4. `html_scrape` (FORBIDDEN for Fandom): fetching rendered HTML
   directly bypasses both the API and the dump path and is the most
   load-inducing method. It is excluded from the Fandom policy by
   design. If a future source requires it, that source must register it
   explicitly and document why.

## User-Agent policy

Every outbound request to a registered source must send the exact
`UserAgent` field from that source's policy. The Fandom policy ships:

```
IrisBot/1.0 (+https://github.com/eko/iris-bot; contact: ops@example.invalid)
```

Contact email is a placeholder; operators must replace it with a real
mailbox before deploying to production.

## Rate limiting

Default rate limit for Fandom: **1 request per second**. Ingestion
workers must respect the `RateLimitRPS` field stored on the policy and
MUST NOT run concurrent workers against the same host that together
exceed this budget.

## No training or fine-tuning

Fandom content is used only to answer user questions in real time and
to build local retrieval indexes. It is never used to train or
fine-tune language models, regardless of model size or provider.

## Unknown sources fail closed

`Registry.ValidateAccess(host, method)` returns
`ErrSourceNotRegistered` for any host that is not explicitly
registered. Callers must treat this as a hard stop and never fall back
to raw HTTP fetching.

## Adding a new source

To add a source later, either:

1. Extend `DefaultRegistry` with another `Source` whose `Policy`
   passes `Policy.Validate`, or
2. Call `Registry.Register` at runtime from a higher-level bootstrap.

Both paths require license, attribution URL, user-agent, at least one
allowed access method, and a notes URL pointing at the source's ToU.
Registration is rejected if any field is missing.

## Escalation

If operators discover that Fandom policy changes, or that a page is no
longer under CC BY-SA 3.0, update the `Policy` in
`internal/lore/source/registry.go` and rebuild. Do not patch the
policy by editing data at a call site.
