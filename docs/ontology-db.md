# AIS Ontology DB

AIS maintains a Ontology DB to store canonical API payloads (called `Entities`). These entities are stored
as JSON-like documents within Redis DB (with Persistence). This is done so that to ensure less latency and
minimize overhead which would otherwise has potential to slow down the requests.

## Keys

All keys are namespaced under `ais:ontology:*`

| Key | Type | Value |
|---|---|---|
| `ais:ontology:entity:<name>:<version>` | String | JSON-encoded entity definition |
| `ais:ontology:active:<name>` | String | Active version for the entity |
| `ais:ontology:entities` | Set | All registered entity names |

Example for `Item` version `v1`:

```text
ais:ontology:entity:Item:v1
ais:ontology:active:Item
ais:ontology:entities
```

## Entity value

The versioned entity key contains JSON equivalent to:

```json
{
  "name": "Item",
  "version": "v1",
  "fields": {
    "id": {
      "type": "string",
      "identity": true
    },
    "name": {
      "type": "string",
      "required": true,
      "description": "Display name of the item"
    }
  }
}
```

The active pointer contains only the version string:

```text
GET ais:ontology:active:Item
=> v1
```

## Write and lookup behavior

When an entity is saved, AIS:

1. Writes the immutable versioned entity JSON.
2. Adds the entity name to `ais:ontology:entities`.
3. Updates `ais:ontology:active:<name>` to the new version.

Lookup with an empty version reads the active pointer first. Supplying a version reads that exact version, allowing older plans to remain available.

These keys are separate from authentication session keys (`ais:auth:session:<id>`). The Redis database can also be separated with `AIS_ONTOLOGY_REDIS_DB`.
