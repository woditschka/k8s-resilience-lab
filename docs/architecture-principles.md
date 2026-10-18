<!-- harness: 2026-09-30 -->
# Architecture Principles

This document carries the tactical pattern catalog this project builds with. It specializes the strategic properties the harness works from — one canonical vocabulary, bounded modules, an isolated unit-testable domain core, and the state-vs-history document split.

**This brief is the single surface for adapting the architecture style**, and it has two layers. The **closed properties** under *Domain Core* are the kernel every brief realizes but does not rewrite. The **open pattern catalog** below them ships an opinionated default the project is free to adapt. The `design-validation` skill reads this file and enforces it as written — it holds no competing copy, so changing a pattern here changes what is enforced.

## Design Principles

The high-level laws designs are evaluated against. The `design-validation` skill enforces them at triage. Rewrite the list to fit this project; keep it short and enforceable.

1. **Security and reliability are emergent** — must be designed in, not retrofitted.
2. **Consistency over novelty** — match existing patterns unless there is a compelling reason. A security law is always that reason: a neighbor's weakness is never a pattern to match.
3. **Explicit dependencies** — every integration point documented.
4. **Layer respect** — features belong in appropriate architectural layers.
5. **Minimal surface** — prefer module-internal visibility.
6. **Understandable systems** — if it cannot be reasoned about, it cannot be secured.
7. **Fail secure** — errors leave the system in a safe state.

## Module Boundaries

The unit of architecture is the module: a bounded context with a public API and hidden internals.

| Rule | Rationale |
|------|-----------|
| Modules depend only on other modules' public APIs | Internal implementation changes don't cascade |
| No circular dependencies between modules | Keeps the dependency graph a hierarchy; cycles make modules un-reasonable in isolation |
| Each module owns its configuration | Prevents god-config; the module stays independently understandable |
| Cross-module orchestration lives in a dedicated service or the entry point | Modules stay independent of each other's execution order |

**Default shape: modulith.** A single deployable unit with enforced internal module boundaries. It provides the separation of microservices without network boundaries, deployment pipelines, or distributed consistency problems — costs that outweigh the benefits at single-team scale.

**Enforcement.** Module boundaries are verified at test time: a modularity test fails the build when a module reaches into another's internals or a dependency cycle appears. Boundaries are not advisory.

**Module = bounded context.** Each module typically maps to one bounded context. If two modules share domain types, decide explicitly: they are one context, or they are two contexts needing a translation layer between them.

## Domain Core

Business logic lives in the domain core, isolated from infrastructure and testable without it. Orchestration stays thin: services coordinate sequencing and error handling; the core decides. When logic leaks into orchestration, it stops being unit-testable in isolation — that is the failure signal.

These properties are **closed**: adapt the *how* in the Pattern Catalog, not these.

| Property | Rationale |
|----------|-----------|
| Domain objects are immutable; collections use defensive copies; no setters | Eliminates shared-mutable-state bugs; safe to pass between pipeline steps without synchronization |
| Invariants are enforced when a domain object is first constructed; rebuilding one from stored state restores an already-valid instance | The domain is valid by construction, and only valid states are ever persisted |
| The domain core holds no infrastructure logic — no I/O, queries, transactions, or DI wiring | The same model runs under any infrastructure; swap the store and the domain is unchanged |
| Aggregates are the consistency boundary: outside code enters only through the root, and aggregates reference each other by identity | Invariants enforced in one place; boundaries stay boundaries |
| Anti-corruption guards every boundary the project does not control; infrastructure mechanics never dictate domain shape | The domain is the fixed point; infrastructure is a swappable boundary |
| Configuration is typed and immutable, validated at startup | Fail fast on invalid configuration; no hidden defaults buried in code |
| Errors flow outward; each layer wraps with context; log only at boundaries | Callers decide handling; no double-logging; per-item failure never aborts a batch |

## Pattern Catalog

The tactical patterns in force in this project — the harness's **opinionated default**, and **open** to adapt. Replace a pattern only with one that still realizes the closed properties above.

| Pattern | Rule | Realizes |
|---------|------|----------|
| **Value object** | Immutable data defined by its attributes; equality by value; no identity | Cheap real objects for tests; no shared mutable state |
| **Aggregate** | Root owns the consistency boundary for its children; outside code enters only through the root | Invariants enforced in one place |
| **Repository** | Persistence gateway, one per aggregate root; the default persistence boundary | The core never touches I/O directly |
| **Domain service** | Stateless; business logic that belongs to no single entity | Logic stays in the testable core |
| **Application service** | Thin; sequences the use case and owns the transaction boundary; no business logic | Orchestration never absorbs the core; one place opens and closes the transaction |
| **Anti-corruption mapper** | A single pure function taking the source values as arguments and returning the mapped object, or an error / fallback; imports no foreign type | A foreign-format change touches one mapper, never the domain |
| **Construction and update** | One entry point per type, taking every mandatory parameter; `with` copies for the attributes that vary, routed through it; rule-governed changes as named operations; no builder | Invariants at construction with one validation home; one API shared by production, mapping, and tests |

### Persistence and boundary mapping

Persistence is a spectrum; choose per project, and the domain core is identical across all of them:

1. **Event-sourced / in-memory** — the model object graph is materialized by folding an event stream (e.g. a log or broker), with no other persistence layer. A relational store is equally valid; neither is privileged.
2. **Repository with an anti-corruption mapper** — the default when the store's shape diverges from the model.
3. **Direct mapping** — when the project **owns both ends** and persistence **follows the model closely**, the model may carry persistence or serialization mapping metadata directly. This is the sanctioned substitute for a hand-written mapper at that controlled boundary — compliant, not a missing anti-corruption layer.

Direct mapping has two gates: the project owns both ends, **and** the stored shape tracks the model closely enough that a separate mapper would be pure boilerplate. Otherwise keep a separate persistence model behind a mapper. Anti-corruption is mandatory only at boundaries the project does **not** control — external APIs, foreign schemas, another system's events.

## Language Realization

How this project implements the catalog in Go.

### Building Blocks

| Pattern | Implementation |
|---------|----------------|
| Value object | Struct whose methods all take a value receiver. One with invariants hides its fields behind one constructor function, `NewOrder(customer, lines) (Order, error)`. One without exposes its fields and is built with a keyed composite literal |
| Aggregate | Struct with unexported fields and one constructor function; the root's methods are the only way in. Slices and maps are copied on the way in and on the way out |
| Repository | Struct holding the store's client, one per aggregate root; it stores and loads that aggregate and nothing else |
| Application service | Struct holding its collaborators; one method sequences one use case |
| Adapter | Struct holding one client; it adapts one external system or one kind of I/O |
| Domain service | Package-level pure function; no receiver, no state, no I/O |
| Anti-corruption mapper | Package-level pure function, `from{Source}()` / `to{Target}()`, exported only when another package maps through it; a complex mapping splits into unexported functions beside it |
| Configuration | Typed struct, built and validated once at startup by its constructor function, then passed by value |

Go has no container and no stereotype. The composition root, `main` or `cmd/<name>`, builds every component with its constructor function and hands each its collaborators as arguments. The shape names the role: a component with collaborators is a struct, and logic without them is a function. A package-level variable never carries a collaborator or a setting.

### Logic Placement

A component orchestrates; domain types, domain services, and mappers decide. A component struct holds four things: the sequence of steps, the I/O and external-service calls, the error and logging policy, and configuration reads. Three things leave it.

| What leaves a component | Where it goes |
|-------------------------|---------------|
| A decision about domain state: a merge rule, how an action applies, a predicate over candidates | A method on the domain type when the decision concerns one value; a domain service when it spans more than one |
| A change of representation: a view model, a prompt input, a filename composition | An anti-corruption mapper |
| Parsing and validation of an external format | An anti-corruption mapper or a domain service |

A method on a domain type takes no collaborator and no `context.Context`. A component's length tracks the number of steps it sequences; growth signals a missing use case, never room for logic. Placement is checked at code review; no build gate marks it.

### Module Layout

A module is one package under `internal/`, named for its subject. Its exported identifiers are its public API, and everything else stays unexported. A module that outgrows one package moves its internals to `internal/<module>/internal/`, which the compiler closes to every other module. Technical-layer packages (`models/`, `services/`, `handlers/`) are forbidden, and so is a second top-level module for the same concern.

The compiler is the modularity test: an import cycle fails the build, and an `internal/` package rejects an importer outside its parent. Foreign types stay in the package that owns the boundary. The domain packages import no client library, and only a mapper names both a domain type and its foreign counterpart.

An interface is declared in the package that consumes it, sized to the methods that consumer calls. A component returns its concrete struct.

### Go Idioms

| Principle | Rule | Rationale |
|-----------|------|-----------|
| **One construction path** | A constructor function takes every mandatory parameter, and its error return is the invariant check. Optional attributes are `With` methods on a value receiver, returning a copy routed through the constructor. A rule-governed change is a named method, never a `With`. | One validation home shared by production, mapping, and tests; a half-built value cannot exist. |
| **Functional options stay in infrastructure** | `NewServer(addr, WithTimeout(t))` configures an infrastructure component. A domain type never takes options. | Options hide which parameters are mandatory; a domain constructor states them. |
| **Immutability by convention** | Value receivers on domain types; no exported field on a type with invariants; `slices.Clone` and `maps.Clone` at every boundary a slice or map crosses. | Go has no immutable struct, so the type's API is what guarantees it. |
| **Errors as values** | Wrap with context, `fmt.Errorf("context: %w", err)`. A caller that must branch gets a sentinel or a typed error and inspects it with `errors.Is` or `errors.As`. | The error-flow rule in Go's own form; no string matching. |
| **Standard library first** | `slices`, `maps`, `cmp`, `errors`, `log/slog`, and `net/http` before a dependency or a hand-written equivalent. | Fewer dependencies; idioms every Go reader knows. |
| **Packages over helper types** | A stateless function set over a type the project does not own is a subject-named package (`slug`). No `Helper` type, and no `util`, `common`, or `misc` package. | The package is Go's native form of the utility class (§ Naming). |
| **Names read by subject** | An exported name sits in a package named for its subject and does not repeat it; no dot import. A value of several fields is written as a keyed composite literal whose field names are the external model's own. No function exists only to label an argument's position. | Go qualifies every identifier with its package name, so the qualifier reads as the noun (§ Naming). |

These principles apply equally to production code and test code. Tests are first-class code: they use the same constructors, `With` copies, and keyed literals as production.

## Naming

Names come from the project's canonical vocabulary (`ubiquitous-language.md`): if the PRD calls it a "feed item", the code says `FeedItem`, never `Entry` or `Record`.

| Concept | Rule |
|---------|------|
| Value objects, aggregates | Domain noun, no suffix |
| Repositories | Suffix `Repository`, one per aggregate root |
| Domain services | Verb or action name, stateless |
| Anti-corruption mappers | `from{Source}()` / `to{Target}()`, static, pure; source values in, mapped object or fallback out |
| Configuration | Suffix `Properties` or `Config`, immutable after construction |
| Optional attributes, named creators | `with{Attribute}(...)` returning a copy; `of(...)` or a domain verb (`place`, `reconstitute`) for a named creator |

**Prohibited suffixes:** `Manager`, `Utility`, `Handler`, `Processor`, `Base`, `Info`, `Data` (as a type suffix). These names are vague, attract unrelated responsibilities, and grow into god objects. Use specific domain nouns and verbs instead.

**`Helper` is allowed for one shape only:** Bloch's utility class from Effective Java, a non-instantiable class of pure static functions, narrowed here to one type the project does not own. Bloch names the class with the plural noun of its subject (`Strings`); `StringHelper` is the accepted alternative. `Helper` is the one suffix that marks the shape, so `Utility` and `Util` stay prohibited. The class is statically imported so the call reads as a verb. Behavior over a type the project owns lives on that type. A name that carries no subject (`Common`, `Misc`) is prohibited because it attracts everything. Where the language has a native form, an extension function or a subject-named package of functions, that form is used instead.

## Design Validation Checklist

Before approving a design, verify:

- [ ] Placement follows the module structure; no reach into another module's internals
- [ ] No circular dependencies introduced; the modularity test passes
- [ ] New types follow the naming rules; no prohibited suffixes
- [ ] Value objects immutable and equal by value; aggregates enforce invariants at construction, entered only through the root, referenced by identity
- [ ] One construction entry point per type taking every mandatory parameter; optional attributes as `with` copies routed through it; rule-governed changes as named operations; no builder and no test-only construction path
- [ ] Anti-corruption guards every boundary the project does not control; an owned, closely-tracked model may be mapped directly
- [ ] Persistence/serialization choices follow this brief's catalog; the domain core holds no infrastructure logic
- [ ] Domain logic testable without framework context; real objects usable in tests
- [ ] New dependencies justified against the dependency policy
- [ ] Terms match the canonical vocabulary
