package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &HostResource{}
	_ resource.ResourceWithConfigure = &HostResource{}
)

// NewHostResource is a helper function to simplify the provider implementation.
func NewHostResource() resource.Resource {
	return &HostResource{}
}

// HostResource is the resource implementation.
type HostResource struct {
	client ProviderHTTPClient
}

// Metadata returns the resource type name.
func (r *HostResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_host"
}

// Configure adds the provider configured client to the resource
func (r *HostResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*AAPClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *AAPClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

// Schema defines the schema for the resource.
func (r *HostResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"inventory_id": schema.Int64Attribute{
				Required: true,
			},
			"instance_id": schema.StringAttribute{
				Optional: true,
			},
			"host_url": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"id": schema.Int64Attribute{
				Computed: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"variables": schema.StringAttribute{
				Optional:   true,
				CustomType: jsontypes.NormalizedType{},
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Defaults False.",
			},
			"groups": schema.ListAttribute{
				ElementType: types.Int64Type,
				Optional:    true,
				Description: "The list of groups to assosicate with a host.",
			},
		},
	}
}

// Host AAP API model
type HostAPIModel struct {
	InventoryId int64  `json:"inventory"`
	InstanceId  string `json:"instance_id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Variables   string `json:"variables,omitempty"`
	Enabled     bool   `json:"enabled"`
	Id          int64  `json:"id,omitempty"`
}

// HostResourceModel maps the host resource schema to a Go struct
type HostResourceModel struct {
	InventoryId types.Int64          `tfsdk:"inventory_id"`
	InstanceId  types.String         `tfsdk:"instance_id"`
	Name        types.String         `tfsdk:"name"`
	URL         types.String         `tfsdk:"host_url"`
	Description types.String         `tfsdk:"description"`
	Variables   jsontypes.Normalized `tfsdk:"variables"`
	Groups      types.List           `tfsdk:"groups"`
	Enabled     types.Bool           `tfsdk:"enabled"`
	Id          types.Int64          `tfsdk:"id"`
}

const pathGroup = "/groups/"

// Create creates the host resource and sets the Terraform state on success.
func (r *HostResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data HostResourceModel
	var diags diag.Diagnostics

	// Read Terraform plan data into host resource model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// create request body from host data
	createRequestBody, diags := data.CreateRequestBody()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestData := bytes.NewReader(createRequestBody)

	// Create new host in AAP
	createResponseBody, diags := r.client.Create("/api/v2/hosts/", requestData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save new host data into host resource model
	diags = data.ParseHttpResponse(createResponseBody)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Groups.IsNull() {
		elements := make([]int64, 0, len(data.Groups.Elements()))
		resp.Diagnostics.Append(data.Groups.ElementsAs(ctx, &elements, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(r.AssociateGroups(ctx, elements, data.URL.ValueString()+pathGroup)...)
		if resp.Diagnostics.HasError() {
			return
		}

		groups, diags := r.ReadAssociatedGroups(data)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		resp.Diagnostics.Append(data.UpdateStateWithGroups(ctx, groups)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest host data.
func (r *HostResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data HostResourceModel
	var diags diag.Diagnostics

	// Read current Terraform state data into host resource model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get latest host data from AAP
	readResponseBody, diags := r.client.Get(data.URL.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save latest host data into host resource model
	diags = data.ParseHttpResponse(readResponseBody)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	groups, diags := r.ReadAssociatedGroups(data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(data.UpdateStateWithGroups(ctx, groups)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the host resource and sets the updated Terraform state on success.
func (r *HostResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data HostResourceModel
	var diags diag.Diagnostics

	// Read Terraform plan data into host resource model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// create request body from host data
	updateRequestBody, diags := data.CreateRequestBody()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestData := bytes.NewReader(updateRequestBody)

	// Update host in AAP
	updateResponseBody, diags := r.client.Update(data.URL.ValueString(), requestData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated host data into host resource model
	diags = data.ParseHttpResponse(updateResponseBody)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Groups.IsNull() {
		resp.Diagnostics.Append(r.HandleGroupAssociation(ctx, data)...)
		if resp.Diagnostics.HasError() {
			return
		}
		groups, diagReadGroups := r.ReadAssociatedGroups(data)
		diags.Append(diagReadGroups...)
		if diags.HasError() {
			return
		}

		resp.Diagnostics.Append(data.UpdateStateWithGroups(ctx, groups)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (d *HostResourceModel) UpdateStateWithGroups(ctx context.Context, groups []int64) diag.Diagnostics {
	var diags diag.Diagnostics

	convertedGroups, diagConvertToInt64 := types.ListValueFrom(ctx, types.Int64Type, groups)
	diags.Append(diagConvertToInt64...)
	if diags.HasError() {
		return diags
	}
	d.Groups = convertedGroups
	return diags
}

// Delete deletes the host resource.
func (r *HostResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data HostResourceModel
	var diags diag.Diagnostics

	// Read current Terraform state data into host resource model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Delete host from AAP
	_, diags = r.client.Delete(data.URL.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func extractIDs(data map[string]interface{}) []int64 {
	var ids []int64

	if value, ok := data["results"]; ok {
		for _, v := range value.([]interface{}) {
			group := v.(map[string]interface{})
			if id, ok := group["id"]; ok {
				ids = append(ids, int64(id.(float64)))
			}
		}
	}

	return ids
}

func sliceDifference(slice1 []int64, slice2 []int64) []int64 {
	// Create a map to store the unique elements from slice2
	seen := make(map[int]struct{})
	for _, v := range slice2 {
		seen[int(v)] = struct{}{}
	}

	// Append elements from slice1 that are not in the seen map
	var difference []int64
	for _, v := range slice1 {
		if _, ok := seen[int(v)]; !ok {
			difference = append(difference, v)
		}
	}

	return difference
}

func (r *HostResource) HandleGroupAssociation(ctx context.Context, data HostResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	elements := make([]int64, 0, len(data.Groups.Elements()))
	diags.Append(data.Groups.ElementsAs(ctx, &elements, false)...)
	if diags.HasError() {
		return diags
	}

	groups, diagReadgroups := r.ReadAssociatedGroups(data)
	diags.Append(diagReadgroups...)
	if diags.HasError() {
		return diags
	}

	toBeAdded := sliceDifference(elements, groups)
	toBeRemoved := sliceDifference(groups, elements)
	url := data.URL.ValueString() + pathGroup

	if len(toBeAdded) > 0 {
		diags.Append(r.AssociateGroups(ctx, toBeAdded, url)...)
		if diags.HasError() {
			return diags
		}
	}

	if len(toBeRemoved) > 0 {
		diags.Append(r.AssociateGroups(ctx, toBeRemoved, url, true)...)
		if diags.HasError() {
			return diags
		}
	}

	return diags
}

func (r *HostResource) ReadAssociatedGroups(data HostResourceModel) ([]int64, diag.Diagnostics) {
	var diags diag.Diagnostics
	var result map[string]interface{}

	// Get latest host data from AAP
	readResponseBody, diagsGetGroups := r.client.Get(data.URL.ValueString() + pathGroup)
	diags.Append(diagsGetGroups...)
	if diags.HasError() {
		return nil, diags
	}

	/* Unmarshal the json string */
	err := json.Unmarshal(readResponseBody, &result)
	if err != nil {
		diags.AddError("Error parsing JSON response from AAP", err.Error())
		return nil, diags
	}

	return extractIDs(result), diags
}

func (r *HostResource) AssociateGroups(ctx context.Context, data []int64, url string, args ...bool) diag.Diagnostics {
	var diags diag.Diagnostics
	var wg sync.WaitGroup
	disassociate := false

	// If disassociate is not provided (zero value), use default value (false)
	if len(args) > 0 {
		disassociate = args[0]
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Make sure it's called to release resources even if no errors
	defer cancel()

	for _, elem := range data {
		wg.Add(1)
		go func(elem int64) {
			defer wg.Done()

			// Check if any error occurred in any other gorouties
			select {
			case <-ctx.Done():
				// Error somewhere, terminate
				return
			default: // Default is must to avoid blocking
			}

			body := make(map[string]int64)
			body["id"] = elem
			if disassociate {
				body["disassociate"] = 1
			}
			json_raw, err := json.Marshal(body)
			if err != nil {
				diags.Append(diag.NewErrorDiagnostic("Body JSON Marshal Error", err.Error()))
				cancel()
				return
			}
			req_data := bytes.NewReader(json_raw)

			resp, _, err := r.client.doRequest(http.MethodPost, url, req_data)
			if err != nil {
				diags.AddError("Body JSON Marshal Error", err.Error())
				cancel()
				return
			}
			if resp == nil {
				diags.AddError("HTTP response Error", "no HTTP response from server")
				cancel()
				return
			}
			if resp.StatusCode != http.StatusNoContent {
				diags.AddError("Unexpected HTTP Status code",
					fmt.Sprintf("expected (%d) got (%s)", http.StatusNoContent, resp.Status))
				cancel()
				return
			}
		}(elem)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	if diags.HasError() {
		return diags
	}

	return diags
}

// CreateRequestBody creates a JSON encoded request body from the host resource data
func (r *HostResourceModel) CreateRequestBody() ([]byte, diag.Diagnostics) {
	// Convert host resource data to API data model

	host := HostAPIModel{
		InventoryId: r.InventoryId.ValueInt64(),
		InstanceId:  r.InstanceId.ValueString(),
		Name:        r.Name.ValueString(),
		Description: r.Description.ValueString(),
		Variables:   r.Variables.ValueString(),
		Enabled:     r.Enabled.ValueBool(),
	}

	// create JSON encoded request body
	jsonBody, err := json.Marshal(host)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError(
			"Error marshaling request body",
			fmt.Sprintf("Could not create request body for host resource, unexpected error: %s", err.Error()),
		)
		return nil, diags
	}

	return jsonBody, nil
}

// ParseHttpResponse updates the host resource data from an AAP API response
func (r *HostResourceModel) ParseHttpResponse(body []byte) diag.Diagnostics {
	var diags diag.Diagnostics

	// Unmarshal the JSON response
	var resultApiHost HostAPIModel
	err := json.Unmarshal(body, &resultApiHost)
	if err != nil {
		diags.AddError("Error parsing JSON response from AAP", err.Error())
		return diags
	}

	// Map response to the host resource schema and update attribute values
	r.InventoryId = types.Int64Value(resultApiHost.InventoryId)
	r.URL = types.StringValue(resultApiHost.URL)
	r.Id = types.Int64Value(resultApiHost.Id)
	r.Name = types.StringValue(resultApiHost.Name)
	r.Enabled = basetypes.NewBoolValue(resultApiHost.Enabled)
	if resultApiHost.Description != "" {
		r.Description = types.StringValue(resultApiHost.Description)
	} else {
		r.Description = types.StringNull()
	}
	if resultApiHost.InstanceId != "" {
		r.InstanceId = types.StringValue(resultApiHost.InstanceId)
	} else {
		r.InstanceId = types.StringNull()
	}
	if resultApiHost.Variables != "" {
		r.Variables = jsontypes.NewNormalizedValue(resultApiHost.Variables)
	} else {
		r.Variables = jsontypes.NewNormalizedNull()
	}

	return diags
}
