package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/passbolt/go-passbolt/api"
	"github.com/passbolt/go-passbolt/helper"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &PasswordResource{}
	_ resource.ResourceWithConfigure = &PasswordResource{}
)

// NewPasswordResource is a helper function to simplify the provider implementation.
func NewPasswordResource() resource.Resource {
	return &PasswordResource{}
}

// PasswordResource is the resource implementation.
type PasswordResource struct {
	client *api.Client
}

// PasswordResourceModel describes the resource data model.
type PasswordResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Username     types.String `tfsdk:"username"`
	URI          types.String `tfsdk:"uri"`
	Password     types.String `tfsdk:"password"`
	FolderParent types.String `tfsdk:"folder_parent"`
	ShareGroup   types.String `tfsdk:"share_group"`
}

// Configure adds the provider configured client to the resource.
func (r *PasswordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *PasswordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_password"
}

// Schema defines the schema for the resource.
func (r *PasswordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the password resource",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the password resource",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "The description of the password resource",
			},
			"username": schema.StringAttribute{
				Required:    true,
				Description: "The username for the password resource",
			},
			"uri": schema.StringAttribute{
				Required:    true,
				Description: "The URI for the password resource",
			},
			"password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "The password for the resource",
			},
			"folder_parent": schema.StringAttribute{
				Optional:    true,
				Description: "The name of the parent folder",
			},
			"share_group": schema.StringAttribute{
				Optional:    true,
				Description: "The name of the group to share the resource with",
			},
		},
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *PasswordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PasswordResourceModel
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
	if plan.Username.ValueString() == "" {
		resp.Diagnostics.AddError("Validation Error", "Username cannot be empty")
		return
	}
	if plan.URI.ValueString() == "" {
		resp.Diagnostics.AddError("Validation Error", "URI cannot be empty")
		return
	}
	if plan.Password.ValueString() == "" {
		resp.Diagnostics.AddError("Validation Error", "Password cannot be empty")
		return
	}

	// Validate URI format
	uri := plan.URI.ValueString()
	if !regexp.MustCompile(`^https?://.*`).MatchString(uri) {
		resp.Diagnostics.AddError("Validation Error", "URI must be a valid HTTP or HTTPS URL")
		return
	}

	// Get folder ID if specified
	var folderID string
	if !plan.FolderParent.IsNull() && !plan.FolderParent.IsUnknown() {
		folders, err := r.client.GetFolders(ctx, nil)
		if err != nil {
			resp.Diagnostics.AddError("Cannot get folders", err.Error())
			return
		}

		for _, folder := range folders {
			if folder.Name == plan.FolderParent.ValueString() {
				folderID = folder.ID
				break
			}
		}

		if folderID == "" {
			resp.Diagnostics.AddError("Validation Error", fmt.Sprintf("Folder '%s' not found", plan.FolderParent.ValueString()))
			return
		}
	}

	// Create the resource using the helper
	resourceID, err := helper.CreateResource(
		ctx,
		r.client,
		folderID,
		plan.Name.ValueString(),
		plan.Username.ValueString(),
		plan.URI.ValueString(),
		plan.Password.ValueString(),
		plan.Description.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Cannot create resource", err.Error())
		return
	}

	// Share with group if specified
	if !plan.ShareGroup.IsNull() && !plan.ShareGroup.IsUnknown() {
		groups, err := r.client.GetGroups(ctx, nil)
		if err != nil {
			resp.Diagnostics.AddError("Cannot get groups", err.Error())
			return
		}

		var groupID string
		for _, group := range groups {
			if group.Name == plan.ShareGroup.ValueString() {
				groupID = group.ID
				break
			}
		}

		if groupID != "" {
			shares := []helper.ShareOperation{
				{
					Type:  7, // Read permission
					ARO:   "Group",
					AROID: groupID,
				},
			}

			err = helper.ShareResource(ctx, r.client, resourceID, shares)
			if err != nil {
				resp.Diagnostics.AddError("Cannot share resource", err.Error())
				return
			}
		}
	}

	// Set the computed values
	plan.ID = types.StringValue(resourceID)

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *PasswordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PasswordResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the resource from Passbolt
	resource, err := r.client.GetResource(ctx, state.ID.ValueString())
	if err != nil {
		// Check if the resource doesn't exist (was deleted outside of Terraform)
		if isResourceNotFoundError(err) {
			// Resource no longer exists, remove it from state
			// This allows Terraform to recreate it if needed
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error reading password",
			"Could not read password, unexpected error: "+err.Error(),
		)
		return
	}

	// Update the state with the current values from Passbolt
	state.Name = types.StringValue(resource.Name)
	state.Description = types.StringValue(resource.Description)
	state.Username = types.StringValue(resource.Username)
	state.URI = types.StringValue(resource.URI)

	// Note: Passwords cannot be read back from Passbolt for security reasons
	// We keep the password from the state to avoid losing it

	// Get folder information if available
	if resource.FolderParentID != "" {
		folders, err := r.client.GetFolders(ctx, nil)
		if err == nil {
			for _, folder := range folders {
				if folder.ID == resource.FolderParentID {
					state.FolderParent = types.StringValue(folder.Name)
					break
				}
			}
		}
	}

	// Set the updated state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *PasswordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PasswordResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state PasswordResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current resource to check what needs to be updated
	currentResource, err := r.client.GetResource(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading current resource",
			"Could not read current resource, unexpected error: "+err.Error(),
		)
		return
	}

	// Check if we need to recreate the resource
	needsRecreation := false
	if plan.Name.ValueString() != currentResource.Name {
		needsRecreation = true

	}
	if plan.Description.ValueString() != currentResource.Description {
		needsRecreation = true

	}
	if plan.Username.ValueString() != currentResource.Username {
		needsRecreation = true

	}
	if plan.URI.ValueString() != currentResource.URI {
		needsRecreation = true

	}
	if plan.Password.ValueString() != state.Password.ValueString() {
		needsRecreation = true

	}

	// Check folder parent changes
	if plan.FolderParent.ValueString() != state.FolderParent.ValueString() {
		needsRecreation = true

	}

	// If we need to recreate, delete and create new resource
	if needsRecreation {
		// Delete the old resource
		err = r.client.DeleteResource(ctx, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error deleting old resource",
				"Could not delete old resource, unexpected error: "+err.Error(),
			)
			return
		}

		// Get folder ID if specified
		var folderID string
		if !plan.FolderParent.IsNull() && !plan.FolderParent.IsUnknown() {
			folders, err := r.client.GetFolders(ctx, nil)
			if err != nil {
				resp.Diagnostics.AddError("Cannot get folders", err.Error())
				return
			}

			for _, folder := range folders {
				if folder.Name == plan.FolderParent.ValueString() {
					folderID = folder.ID
					break
				}
			}
		}

		// Create the new resource
		resourceID, err := helper.CreateResource(
			ctx,
			r.client,
			folderID,
			plan.Name.ValueString(),
			plan.Username.ValueString(),
			plan.URI.ValueString(),
			plan.Password.ValueString(),
			plan.Description.ValueString(),
		)
		if err != nil {
			resp.Diagnostics.AddError("Cannot recreate resource", err.Error())
			return
		}

		// Share with group if specified
		if !plan.ShareGroup.IsNull() && !plan.ShareGroup.IsUnknown() {
			groups, err := r.client.GetGroups(ctx, nil)
			if err != nil {
				resp.Diagnostics.AddError("Cannot get groups", err.Error())
				return
			}

			var groupID string
			for _, group := range groups {
				if group.Name == plan.ShareGroup.ValueString() {
					groupID = group.ID
					break
				}
			}

			if groupID != "" {
				shares := []helper.ShareOperation{
					{
						Type:  7, // Read permission
						ARO:   "Group",
						AROID: groupID,
					},
				}

				err = helper.ShareResource(ctx, r.client, resourceID, shares)
				if err != nil {
					resp.Diagnostics.AddError("Cannot share resource", err.Error())
					return
				}
			}
		}

		// Update the state ID
		state.ID = types.StringValue(resourceID)
	}

	// Update state with the new values from the plan
	state.Name = plan.Name
	state.Description = plan.Description
	state.Username = plan.Username
	state.URI = plan.URI
	state.Password = plan.Password
	state.FolderParent = plan.FolderParent
	state.ShareGroup = plan.ShareGroup

	// Set the updated state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *PasswordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PasswordResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the resource
	err := r.client.DeleteResource(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting password",
			"Could not delete password, unexpected error: "+err.Error(),
		)
		return
	}
}
