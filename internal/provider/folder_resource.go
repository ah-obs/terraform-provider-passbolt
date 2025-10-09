package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/passbolt/go-passbolt/api"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &FolderResource{}
	_ resource.ResourceWithConfigure = &FolderResource{}
)

// NewFolderResource is a helper function to simplify the provider implementation.
func NewFolderResource() resource.Resource {
	return &FolderResource{}
}

// FolderResource is the resource implementation.
type FolderResource struct {
	client *api.Client
}

// FolderResourceModel describes the resource data model.
type FolderResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Personal     types.Bool   `tfsdk:"personal"`
	FolderParent types.String `tfsdk:"folder_parent"`
}

// Configure adds the provider configured client to the resource.
func (r *FolderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *api.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

// Metadata returns the resource type name.
func (r *FolderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_folder"
}

// Schema defines the schema for the resource.
func (r *FolderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the folder",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the folder",
			},
			"personal": schema.BoolAttribute{
				Computed:    true,
				Optional:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Whether the folder is personal",
			},
			"folder_parent": schema.StringAttribute{
				Optional:    true,
				Description: "The name of the parent folder",
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *FolderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FolderResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate input
	if plan.Name.ValueString() == "" {
		resp.Diagnostics.AddError("Validation Error", "Name cannot be empty")
		return
	}

	// Get parent folder ID if specified
	var parentFolderID string
	if !plan.FolderParent.IsNull() && !plan.FolderParent.IsUnknown() {
		folders, err := r.client.GetFolders(ctx, nil)
		if err != nil {
			resp.Diagnostics.AddError("Cannot get folders", err.Error())
			return
		}

		for _, folder := range folders {
			if folder.Name == plan.FolderParent.ValueString() {
				parentFolderID = folder.ID
				break
			}
		}

		if parentFolderID == "" {
			resp.Diagnostics.AddError("Validation Error", fmt.Sprintf("Parent folder '%s' not found", plan.FolderParent.ValueString()))
			return
		}
	}

	// Create the folder
	folder := api.Folder{
		FolderParentID: parentFolderID,
		Name:           plan.Name.ValueString(),
	}

	createdFolder, err := r.client.CreateFolder(ctx, folder)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating folder",
			"Could not create folder, unexpected error: "+err.Error(),
		)
		return
	}

	// Set the computed values
	plan.ID = types.StringValue(createdFolder.ID)
	plan.Personal = types.BoolValue(createdFolder.Personal)

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *FolderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FolderResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the folder from Passbolt
	folder, err := r.client.GetFolder(ctx, state.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading folder",
			"Could not read folder, unexpected error: "+err.Error(),
		)
		return
	}

	// Update the state with the current values from Passbolt
	state.Name = types.StringValue(folder.Name)
	state.Personal = types.BoolValue(folder.Personal)

	// Get parent folder information if available
	if folder.FolderParentID != "" {
		parentFolder, err := r.client.GetFolder(ctx, folder.FolderParentID, nil)
		if err == nil {
			state.FolderParent = types.StringValue(parentFolder.Name)
		}
	} else {
		state.FolderParent = types.StringNull()
	}

	// Set the updated state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *FolderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FolderResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state FolderResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current folder to check what needs to be updated
	currentFolder, err := r.client.GetFolder(ctx, state.ID.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading current folder",
			"Could not read current folder, unexpected error: "+err.Error(),
		)
		return
	}

	// Check if we need to recreate the folder
	needsRecreation := false
	if plan.Name.ValueString() != currentFolder.Name {
		needsRecreation = true

	}

	// Check folder parent changes
	if plan.FolderParent.ValueString() != state.FolderParent.ValueString() {
		needsRecreation = true

	}

	// If we need to recreate, delete and create new folder
	if needsRecreation {
		// Delete the old folder
		err = r.client.DeleteFolder(ctx, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error deleting old folder",
				"Could not delete old folder, unexpected error: "+err.Error(),
			)
			return
		}

		// Get parent folder ID if specified
		var parentFolderID string
		if !plan.FolderParent.IsNull() && !plan.FolderParent.IsUnknown() {
			folders, err := r.client.GetFolders(ctx, nil)
			if err != nil {
				resp.Diagnostics.AddError("Cannot get folders", err.Error())
				return
			}

			for _, folder := range folders {
				if folder.Name == plan.FolderParent.ValueString() {
					parentFolderID = folder.ID
					break
				}
			}
		}

		// Create the new folder
		folder := api.Folder{
			FolderParentID: parentFolderID,
			Name:           plan.Name.ValueString(),
		}

		createdFolder, err := r.client.CreateFolder(ctx, folder)
		if err != nil {
			resp.Diagnostics.AddError("Cannot recreate folder", err.Error())
			return
		}

		// Update the state ID
		state.ID = types.StringValue(createdFolder.ID)
		state.Personal = types.BoolValue(createdFolder.Personal)
	}

	// Update state with the new values from the plan
	state.Name = plan.Name
	state.FolderParent = plan.FolderParent

	// Set the updated state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *FolderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FolderResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the folder
	err := r.client.DeleteFolder(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting folder",
			"Could not delete folder, unexpected error: "+err.Error(),
		)
		return
	}
}
