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

var _ resource.Resource = &SetResource{}
var _ resource.ResourceWithImportState = &SetResource{}

// NewSetResource creates a new redis_set resource.
func NewSetResource() resource.Resource {
	return &SetResource{}
}

// SetResource manages a Redis set.
type SetResource struct {
	providerCfg *providerConfig
}

// SetResourceModel describes the resource data model.
type SetResourceModel struct {
	Key        types.String             `tfsdk:"key"`
	Members    types.Set                `tfsdk:"members"`
	ID         types.String             `tfsdk:"id"`
	Connection *ConnectionOverrideModel `tfsdk:"redis_connection"`
}

func (r *SetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_set"
}

func (r *SetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Redis set (`SADD`). On update, members removed from the configuration are deleted with `SREM`.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				MarkdownDescription: "The Redis key.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"members": schema.SetAttribute{
				MarkdownDescription: "The set of string members to store.",
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

func (r *SetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members := make([]string, 0)
	resp.Diagnostics.Append(data.Members.ElementsAs(ctx, &members, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(members) == 0 {
		resp.Diagnostics.AddError("Invalid Configuration", "members must contain at least one entry.")
		return
	}

	client := r.providerCfg.clientFor(data.Connection)
	defer client.Close()

	// DEL first so that pre-existing members in the key do not bleed into Terraform state.
	if err := client.Del(ctx, data.Key.ValueString()).Err(); err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to DEL key %q before creating set: %s", data.Key.ValueString(), err))
		return
	}

	args := make([]interface{}, len(members))
	for i, m := range members {
		args[i] = m
	}

	if err := client.SAdd(ctx, data.Key.ValueString(), args...).Err(); err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to SADD key %q: %s", data.Key.ValueString(), err))
		return
	}

	data.ID = data.Key
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.providerCfg.clientFor(data.Connection)
	defer client.Close()

	result, err := client.SMembers(ctx, data.Key.ValueString()).Result()
	if err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to SMEMBERS key %q: %s", data.Key.ValueString(), err))
		return
	}
	if len(result) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	elems := make([]attr.Value, len(result))
	for i, m := range result {
		elems[i] = types.StringValue(m)
	}
	membersSet, diags := types.SetValue(types.StringType, elems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Members = membersSet
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planMembers := make([]string, 0)
	resp.Diagnostics.Append(plan.Members.ElementsAs(ctx, &planMembers, false)...)
	stateMembers := make([]string, 0)
	resp.Diagnostics.Append(state.Members.ElementsAs(ctx, &stateMembers, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.providerCfg.clientFor(plan.Connection)
	defer client.Close()

	planSet := make(map[string]bool, len(planMembers))
	for _, m := range planMembers {
		planSet[m] = true
	}
	stateSet := make(map[string]bool, len(stateMembers))
	for _, m := range stateMembers {
		stateSet[m] = true
	}

	// Remove members no longer in the plan.
	var toRemove []interface{}
	for m := range stateSet {
		if !planSet[m] {
			toRemove = append(toRemove, m)
		}
	}
	if len(toRemove) > 0 {
		if err := client.SRem(ctx, plan.Key.ValueString(), toRemove...).Err(); err != nil {
			resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to SREM members from key %q: %s", plan.Key.ValueString(), err))
			return
		}
	}

	// Add new members.
	var toAdd []interface{}
	for m := range planSet {
		if !stateSet[m] {
			toAdd = append(toAdd, m)
		}
	}
	if len(toAdd) > 0 {
		if err := client.SAdd(ctx, plan.Key.ValueString(), toAdd...).Err(); err != nil {
			resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to SADD members to key %q: %s", plan.Key.ValueString(), err))
			return
		}
	}

	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SetResourceModel
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

func (r *SetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	key := req.ID

	client := r.providerCfg.clientFor(nil)
	defer client.Close()

	result, err := client.SMembers(ctx, key).Result()
	if err != nil {
		resp.Diagnostics.AddError("Redis Error", fmt.Sprintf("Unable to SMEMBERS key %q: %s", key, err))
		return
	}
	if len(result) == 0 {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Set key %q does not exist in Redis.", key))
		return
	}

	elems := make([]attr.Value, len(result))
	for i, m := range result {
		elems[i] = types.StringValue(m)
	}
	membersSet, diags := types.SetValue(types.StringType, elems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &SetResourceModel{
		Key:     types.StringValue(key),
		Members: membersSet,
		ID:      types.StringValue(key),
	})...)
}
