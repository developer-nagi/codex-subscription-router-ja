# Security policy

## Supported versions

Only the most recent tagged source release is supported. The official ChatGPT builds it
targets are recorded in `docs/COMPATIBILITY.md`.

## Reporting a vulnerability

Do not open a public issue for a suspected credential leak, an arbitrary code execution
path, or a control-server authentication weakness. Report it through **Security → Report
a vulnerability** on the repository. If that form is unavailable, draft a private security
advisory from the Security tab and invite the repository owner. Include:

- The project version and the exact commit
- The official ChatGPT version used as build input
- Reproduction steps and the expected impact
- Whether private data such as credentials may have been exposed

Do not include real access tokens, device codes, account identifiers, or private
conversation content. Revoke any affected credential before sharing reproduction steps.

## Scope

Report issues in unmodified OpenAI apps or services to OpenAI. The scope here is the
patcher, the multiplexer, the injected UI, and the isolated local state.
