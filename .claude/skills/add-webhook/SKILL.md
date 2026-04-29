# Add or extend a webhook

Use this skill when the user wants to add webhook support for a Kubernetes resource in this repository.

## Repository rules

- Only create a new webhook for a new GVK.
- If a webhook already exists for that GVK, extend the existing implementation instead of creating a second webhook.

## Required first step

Before making code changes, ask the user for the target resource's:

- group
- version
- kind

If the webhook type is not already clear, also ask whether they want any of these:

- conversion
- defaulting
- programmatic validation

Wait for that information before proceeding.

## Workflow

### 1. Check whether this GVK already has a webhook

Use the provided group/version/kind to inspect the existing code before scaffolding anything.

At minimum, check these places:

- `PROJECT` for an existing resource entry and webhook metadata
- `cmd/main.go` for `Setup*WebhookWithManager` registration
- `internal/webhook/` for an existing `<kind>_webhook.go` or related webhook file
- `internal/webhook/` for `+kubebuilder:webhook` markers matching the same resource
- `internal/webhook/*/webhook_suite_test.go` for webhook setup in tests
- `config/webhook/manifests.yaml` for an already-generated webhook manifest

If the same GVK already exists anywhere in the repository, treat that as an existing webhook and extend it.

### 2. If the webhook already exists for that GVK

- Do **not** scaffold a second webhook.
- Add the new logic to the existing webhook implementation.
- If the user is adding another webhook type for the same GVK, keep it in the same resource-specific webhook area rather than creating a duplicate resource webhook.
- Update or add tests next to the existing webhook tests.

### 3. If the webhook does not exist for that GVK

Create it with `operator-sdk create webhook`.

Always pass these flags:

- `--group`
- `--version`
- `--kind`

Then add one or more of these optional flags depending on the requested webhook types:

- `--conversion`
- `--defaulting`
- `--programmatic-validation`

Examples:

```bash
operator-sdk create webhook --group <group> --version <version> --kind <kind> --programmatic-validation
```

```bash
operator-sdk create webhook --group <group> --version <version> --kind <kind> --defaulting --programmatic-validation
```

```bash
operator-sdk create webhook --group <group> --version <version> --kind <kind> --conversion
```

Only include the optional flags that the user actually wants.

### 4. After scaffolding or locating the existing webhook

Prepare the codebase for the follow-up implementation details that the user will provide later.

When implementing webhook logic, keep the entry methods minimal:

- `ValidateCreate`
- `ValidateUpdate`
- `ValidateDelete`

These methods should only delegate to helper methods that return `(admission.Warnings, error)`.

In the entry methods:

- call the helper method(s)
- append returned warnings
- if `err` is not `nil`, return the error

Keep business logic in helper methods instead of growing the entry methods.

Review the generated or existing changes in:

- `PROJECT`
- `cmd/main.go`
- `internal/webhook/`
- `config/webhook/`
- `config/default/kustomization.yaml`
- `charts/kubernetes-webhooks/templates/`

If the scaffold introduced or requires new webhook manifests, regenerate and verify generated artifacts as needed.

## Follow-up rule

Do not invent the webhook business logic.

Once the existing webhook has been identified or the new scaffold has been created, pause for the user's next message with the implementation details for the webhook behavior.

## Validation checklist

Before finishing the task, verify:

- the repository still has only one webhook implementation per GVK
- any new scaffolded files are wired into the manager and webhook test suite
- generated manifests are up to date
- tests and formatting have been run when code changes were made
- after writing tests, corner cases were reviewed and the most important ones are covered

Useful commands:

```bash
make manifests
make generate
make fmt
make test
```

If Helm templates need to reflect regenerated manifests, also run:

```bash
IMG=ghcr.io/snapp-incubator/kubernetes-webhooks:<new-tag> make helm
```
