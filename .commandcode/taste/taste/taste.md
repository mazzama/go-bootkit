# Taste
- Delegates design and implementation decisions to the assistant after stating high-level requirements (repeatedly says "follow your recommendation" / "yes, agree with your recommendation"). Confidence: 0.9
- Prefers independent work items (e.g., database migrations, implementation tickets) to be executed in parallel rather than sequentially. Confidence: 0.8
- Prefers code reviews structured as categorized findings — critical issues, architectural violations, strengths, and refactoring suggestions with exact code snippets — and wants them direct, technical, and critical without unnecessary praise. Confidence: 0.9
- Prefers setup/CLI steps to be documented explicitly as prerequisites in project docs. Confidence: 0.6
- Frequently uses architecture-review and code-grilling workflows (e.g., /improve-codebase-architecture, /grill-with-docs) and values critical scrutiny of production readiness, edge cases, concurrency, and connection pooling. Confidence: 0.8
- Prefers architecture/design proposals to include concrete, lightweight implementation sketches (actual interfaces, structs, adapter code, and wiring), not just conceptual diagrams — explicitly asks to see what the implementation "looks like." Confidence: 0.5
