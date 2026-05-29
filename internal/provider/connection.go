// Copyright (c) 2026 James Pickering
// SPDX-License-Identifier: MIT

package provider

import (
	"crypto/tls"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/redis/go-redis/v9"
)

// providerConfig holds the fully-resolved provider-level connection settings
// after environment variable defaults have been applied. A *providerConfig is
// passed to resources via ResourceData so each resource can apply its own
// per-resource overrides on top.
type providerConfig struct {
	addr                  string
	password              string
	username              string
	db                    int
	tls                   bool
	tlsInsecureSkipVerify bool
}

// clientFor creates a *redis.Client by merging the provider-level config with
// any per-resource overrides. override may be nil when no connection block is
// present in the resource configuration.
func (c *providerConfig) clientFor(override *ConnectionOverrideModel) *redis.Client {
	addr := c.addr
	password := c.password
	username := c.username
	db := c.db
	useTLS := c.tls
	skipVerify := c.tlsInsecureSkipVerify

	if override != nil {
		if !override.Addr.IsNull() && !override.Addr.IsUnknown() {
			addr = override.Addr.ValueString()
		}
		if !override.Password.IsNull() && !override.Password.IsUnknown() {
			password = override.Password.ValueString()
		}
		if !override.Username.IsNull() && !override.Username.IsUnknown() {
			username = override.Username.ValueString()
		}
		if !override.DB.IsNull() && !override.DB.IsUnknown() {
			db = int(override.DB.ValueInt64())
		}
		if !override.TLS.IsNull() && !override.TLS.IsUnknown() {
			useTLS = override.TLS.ValueBool()
		}
		if !override.TLSInsecureSkipVerify.IsNull() && !override.TLSInsecureSkipVerify.IsUnknown() {
			skipVerify = override.TLSInsecureSkipVerify.ValueBool()
		}
	}

	opts := &redis.Options{
		Addr:     addr,
		Password: password,
		Username: username,
		DB:       db,
	}
	if useTLS {
		opts.TLSConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: skipVerify, //nolint:gosec // controlled by explicit provider option
		}
	}
	return redis.NewClient(opts)
}

// ConnectionOverrideModel holds optional per-resource connection settings.
type ConnectionOverrideModel struct {
	Addr                  types.String `tfsdk:"addr"`
	Password              types.String `tfsdk:"password"`
	Username              types.String `tfsdk:"username"`
	DB                    types.Int64  `tfsdk:"db"`
	TLS                   types.Bool   `tfsdk:"tls"`
	TLSInsecureSkipVerify types.Bool   `tfsdk:"tls_insecure_skip_verify"`
}

// connectionBlock returns the schema for the optional nested connection block
// that can appear on any resource. The attribute is named "redis_connection"
// (not "connection", which is reserved by Terraform for provisioner blocks).
func connectionBlock() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Per-resource connection settings (`redis_connection`). Each attribute is optional and overrides the matching provider-level setting for this resource only. If omitted, all provider-level settings are used. Changing this block to point at a different Redis server will not clean up the key on the original server — use `terraform import` to adopt existing keys.",
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"addr": schema.StringAttribute{
				MarkdownDescription: "Redis server address in `host:port` form. Overrides the provider `addr`.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Redis password. Overrides the provider `password`.",
				Optional:            true,
				Sensitive:           true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Redis username. Overrides the provider `username`.",
				Optional:            true,
			},
			"db": schema.Int64Attribute{
				MarkdownDescription: "Redis database index (0–15). Overrides the provider `db`.",
				Optional:            true,
			},
			"tls": schema.BoolAttribute{
				MarkdownDescription: "Enable TLS. Overrides the provider `tls`.",
				Optional:            true,
			},
			"tls_insecure_skip_verify": schema.BoolAttribute{
				MarkdownDescription: "Disable TLS certificate verification. Overrides the provider `tls_insecure_skip_verify`. **Not recommended in production.**",
				Optional:            true,
			},
		},
	}
}
