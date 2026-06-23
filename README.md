# cosmonaut-plugin-trino

Cosmonaut plugin for [Apache Trino](https://trino.io).

Implements the [cosmonaut-sdk](https://github.com/galileostd/cosmonaut-sdk) `PluginService` gRPC interface.

## What it does

- **Health check** — calls `GET /v1/info` on Trino and reports healthy/degraded/unhealthy
- **Execute query** — submits SQL via `POST /v1/statement` and returns a query ID
- **Get job** — returns the current state of a running query
- **Cancel job** — cancels a running query via `DELETE /v1/query/{id}`

## How it works

This plugin is a gRPC server. The Cosmonaut control-plane discovers it automatically via the Kubernetes Service label:

```
cosmonaut.galileostd.io/plugin: "true"
```

No manual registration needed.

## Deploy

```bash
helm install cosmonaut-plugin-trino ./charts/cosmonaut-plugin-trino \
  --namespace cosmonaut \
  --create-namespace
```

After deployment, register the component with the control-plane:

```yaml
apiVersion: cosmonaut.galileostd.io/v1
kind: CosmoComponent
metadata:
  name: trino
  namespace: cosmonaut
spec:
  plugin: trino
  type: query-engine
  endpoint: http://trino.trino.svc.cluster.local:8080
  healthCheckIntervalSeconds: 30
  config:
    user: cosmonaut
```

## Execute a query

```bash
curl -X POST http://cosmonaut:8080/api/v1/components/cosmonaut/trino/exec \
  -H "Content-Type: application/json" \
  -d '{"action": "query", "payload": {"sql": "SELECT * FROM bronze.flights LIMIT 10"}}'
```

## Configuration

The plugin itself has no configuration — it is stateless. All connection details are passed by the control-plane at runtime via `CosmoComponent.spec.endpoint` and `CosmoComponent.spec.config`.

| Config key | Description | Default |
|---|---|---|
| `user` | Trino user for query attribution | `cosmonaut` |

## License

Apache 2.0. Part of the [Cosmonaut](https://cosmonaut.galileostd.io) project.
