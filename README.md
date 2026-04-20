# kubernetes-webhooks

Snappcloud Kubernetes webhooks

## Description

We mainly use Kyverno for our Kubernetes webhooks.
This project contains those webhooks that can't be implmented with Kyverno.

## Contributing

### Generating Helm Chart

To re-generate the Helm chart, run the following command.
Publising the helm chart is done through the gitlab CI pipeline.

```bash
IMG=github.com/library/kubernetes-webhooks:<new-tag> make helm
```

### Creating a new webhook

When creating a new webhook, please follow these guidelines:

- We only generate a new webhook for a new GVK(group, version, kind) that we want to validate, default or mutate. If a webhook already exists for the
  GVK, we add the logic to the existing webhook.

- Use the following command to generate the boilerplate code for a new validating webhook. For more details checkout
  the [operatork-sdk documentation](https://sdk.operatorframework.io/docs/building-operators/golang/webhook/#create-validation-webhook).

```bash
operator-sdk create webhook --group <DesiredGroup> --version <DesiredVersion> --kind <DesiredKind> --defaulting --programmatic-validation
```

- The helm chart can be published through a manual GitHub workflow. Run the workflow
  from [GitHub actions](https://github.com/snapp-incubator/kubernetes-webhooks/actions/workflows/release-helm-chart.yaml).
