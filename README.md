# Terraform Provider: Redis

A [Terraform](https://www.terraform.io) provider for populating a [Redis](https://redis.io) instance with data. It is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework).

## Resources

| Resource       | Redis type | Description                                                               |
| -------------- | ---------- | ------------------------------------------------------------------------- |
| `redis_string` | String     | Manages a key-value string pair (`SET` / `GET` / `DEL`)                   |
| `redis_hash`   | Hash       | Manages a hash with arbitrary string fields (`HSET` / `HGETALL` / `HDEL`) |
| `redis_set`    | Set        | Manages an unordered set of string members (`SADD` / `SMEMBERS` / `SREM`) |

All resources support `terraform import` using the Redis key as the import ID.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.13
- [Go](https://golang.org/doc/install) >= 1.24
- A running Redis instance (>= 6.0)

## Using the Provider

```hcl
terraform {
  required_providers {
    redis = {
      source = "jamespic/redis"
    }
  }
}

provider "redis" {
  addr = "localhost:6379" # or set REDIS_ADDR
}

resource "redis_string" "example" {
  key   = "app:version"
  value = "1.2.3"
}

resource "redis_hash" "example" {
  key = "user:42"
  fields = {
    name  = "Alice"
    email = "alice@example.com"
  }
}

resource "redis_set" "example" {
  key     = "article:1:tags"
  members = ["terraform", "redis", "infrastructure"]
}
```

Provider configuration attributes (`addr`, `password`, `username`, `db`) can all be omitted and supplied via environment variables instead — see the provider schema docs.

## Building the Provider

```shell
git clone <this repo>
cd redis-terraform-provider
go install
```

## Developing the Provider

To run unit tests:

```shell
go test ./...
```

To run acceptance tests (requires a running Redis instance):

```shell
REDIS_ADDR=localhost:6379 TF_ACC=1 go test -v ./internal/provider/
```

To regenerate documentation from schema and examples:

```shell
make generate
```

To run linters:

```shell
make lint
```

## Known Limitations

- **No TTL support.** None of the resources support setting a Redis key expiry. Keys created by this provider persist indefinitely.
- **String values only.** Hash fields and set members are always stored and read back as strings. This matches Redis's own behaviour for these types.
- **Ownership on create.** When `Create` is called for a `redis_hash` or `redis_set` resource, the provider issues a `DEL` on the target key before writing. This ensures Terraform fully owns the key's contents. If you want to adopt a key that already exists in Redis without losing its current data, use `terraform import` instead.
