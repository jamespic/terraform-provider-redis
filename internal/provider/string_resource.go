// Copyright (c) 2026 James Pickering
// SPDX-License-Identifier: MIT

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/redis/go-redis/v9" // for redis.Nil
)

var _ resource.Resource = &StringResource{}
var _ resource.ResourceWithImportState = &StringResource{}

// NewStringResource creates a new redis_string resource.
func NewStringResource() resource.Resource {
	return &StringResource{}
}

// StringResource manages a Redis string key-value pair.
type StringResource struct {
	providerCfg *providerConfig
}

// StringResourceModel describes the resource data model.
type StringResourceModel struct {
	Key        types.String             `tfsdk:"key"`
	Value      types.String             `tfsdk:"value"`
	ID         types.String             `tfsdk:"id"`
	Connection *ConnectionOverrideModel `tfsdk:"redis_connection"`
}

func (r *StringResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_string"
}

func (r *StringResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Redis string key-value pair (`SET`).",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				MarkdownDescription: "The Redis key.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The string value to store.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The Redis key, used as the resource identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"redis_connection": connectionBlock(),
		},
	}
}

func (r *StringResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cfg, ok := req.ProviderData.(*providerConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *providerConfig, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.providerCfg = cfg
}

func (r *StringResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data StringResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.providerCfg.clientFor(data.Connection)
	defer client.Close()

	if err := client.Set(ctx, data.Key.ValueString(), data.Value.ValueString(), 0).Err(); err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to SET key %q: %s", data.Key.ValueString(), err))
		return
	}

	data.ID = data.Key
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *StringResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data StringResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.providerCfg.clientFor(data.Connection)
	defer client.Close()

	val, err := client.Get(ctx, data.Key.ValueString()).Result()
	if err == redis.Nil {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to GET key %q: %s", data.Key.ValueString(), err))
		return
	}

	data.Value = types.StringValue(val)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *StringResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data StringResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.providerCfg.clientFor(data.Connection)
	defer client.Close()

	if err := client.Set(ctx, data.Key.ValueString(), data.Value.ValueString(), 0).Err(); err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to SET key %q: %s", data.Key.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *StringResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data StringResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.providerCfg.clientFor(data.Connection)
	defer client.Close()

	if err := client.Del(ctx, data.Key.ValueString()).Err(); err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to DEL key %q: %s", data.Key.ValueString(), err))
	}
}

func (r *StringResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	key := req.ID

	client := r.providerCfg.clientFor(nil)
	defer client.Close()

	val, err := client.Get(ctx, key).Result()
	if err == redis.Nil {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Key %q does not exist in Redis.", key))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to GET key %q: %s", key, err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &StringResourceModel{
		Key:   types.StringValue(key),
		Value: types.StringValue(val),
		ID:    types.StringValue(key),
	})...)
}
