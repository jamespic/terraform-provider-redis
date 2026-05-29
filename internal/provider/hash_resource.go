// Copyright (c) 2026 James Pickering
// SPDX-License-Identifier: MIT

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &HashResource{}
var _ resource.ResourceWithImportState = &HashResource{}

// NewHashResource creates a new redis_hash resource.
func NewHashResource() resource.Resource {
	return &HashResource{}
}

// HashResource manages a Redis hash.
type HashResource struct {
	providerCfg *providerConfig
}

// HashResourceModel describes the resource data model.
type HashResourceModel struct {
	Key        types.String             `tfsdk:"key"`
	Fields     types.Map                `tfsdk:"fields"`
	ID         types.String             `tfsdk:"id"`
	Connection *ConnectionOverrideModel `tfsdk:"redis_connection"`
}

func (r *HashResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hash"
}

func (r *HashResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Redis hash (`HSET`). On update, fields removed from the configuration are deleted with `HDEL`.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				MarkdownDescription: "The Redis key.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"fields": schema.MapAttribute{
				MarkdownDescription: "A map of field names to string values for the hash.",
				Required:            true,
				ElementType:         types.StringType,
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

func (r *HashResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *HashResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data HashResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	fields := make(map[string]string)
	resp.Diagnostics.Append(data.Fields.ElementsAs(ctx, &fields, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(fields) == 0 {
		resp.Diagnostics.AddError("Invalid Configuration", "fields must contain at least one entry.")
		return
	}

	client := r.providerCfg.clientFor(data.Connection)
	defer client.Close()

	// DEL first so that pre-existing fields in the key do not bleed into Terraform state.
	if err := client.Del(ctx, data.Key.ValueString()).Err(); err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to DEL key %q before creating hash: %s", data.Key.ValueString(), err))
		return
	}

	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}

	if err := client.HSet(ctx, data.Key.ValueString(), args...).Err(); err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to HSET key %q: %s", data.Key.ValueString(), err))
		return
	}

	data.ID = data.Key
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HashResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data HashResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.providerCfg.clientFor(data.Connection)
	defer client.Close()

	result, err := client.HGetAll(ctx, data.Key.ValueString()).Result()
	if err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to HGETALL key %q: %s", data.Key.ValueString(), err))
		return
	}
	if len(result) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	elems := make(map[string]attr.Value, len(result))
	for k, v := range result {
		elems[k] = types.StringValue(v)
	}
	fieldsMap, diags := types.MapValue(types.StringType, elems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Fields = fieldsMap
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HashResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state HashResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planFields := make(map[string]string)
	resp.Diagnostics.Append(plan.Fields.ElementsAs(ctx, &planFields, false)...)
	stateFields := make(map[string]string)
	resp.Diagnostics.Append(state.Fields.ElementsAs(ctx, &stateFields, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.providerCfg.clientFor(plan.Connection)
	defer client.Close()

	// Remove fields that are no longer present in the plan.
	var toDelete []string
	for k := range stateFields {
		if _, ok := planFields[k]; !ok {
			toDelete = append(toDelete, k)
		}
	}
	if len(toDelete) > 0 {
		if err := client.HDel(ctx, plan.Key.ValueString(), toDelete...).Err(); err != nil {
			resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to HDEL fields from key %q: %s", plan.Key.ValueString(), err))
			return
		}
	}

	// Set all planned fields (new and updated).
	args := make([]interface{}, 0, len(planFields)*2)
	for k, v := range planFields {
		args = append(args, k, v)
	}
	if len(args) > 0 {
		if err := client.HSet(ctx, plan.Key.ValueString(), args...).Err(); err != nil {
			resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to HSET key %q: %s", plan.Key.ValueString(), err))
			return
		}
	}

	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *HashResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data HashResourceModel
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

func (r *HashResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	key := req.ID

	client := r.providerCfg.clientFor(nil)
	defer client.Close()

	result, err := client.HGetAll(ctx, key).Result()
	if err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to HGETALL key %q: %s", key, err))
		return
	}
	if len(result) == 0 {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Hash key %q does not exist in Redis.", key))
		return
	}

	elems := make(map[string]attr.Value, len(result))
	for k, v := range result {
		elems[k] = types.StringValue(v)
	}
	fieldsMap, diags := types.MapValue(types.StringType, elems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &HashResourceModel{
		Key:    types.StringValue(key),
		Fields: fieldsMap,
		ID:     types.StringValue(key),
	})...)
}
