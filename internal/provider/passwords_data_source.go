package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/passbolt/go-passbolt/api"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &PasswordsDataSource{}
	_ datasource.DataSourceWithConfigure = &PasswordsDataSource{}
)

// NewPasswordsDataSource is a helper function to simplify the provider implementation.
func NewPasswordsDataSource() datasource.DataSource {
	return &PasswordsDataSource{}
}

// PasswordsDataSource is the data source implementation.
type PasswordsDataSource struct {
	client *api.Client
}

// PasswordsDataSourceModel describes the data source data model.
type PasswordsDataSourceModel struct {
	Passwords []PasswordModel `tfsdk:"passwords"`
}

// PasswordModel describes a single password resource.
type PasswordModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Username     types.String `tfsdk:"username"`
	URI          types.String `tfsdk:"uri"`
	FolderParent types.String `tfsdk:"folder_parent"`
}

// Configure adds the provider configured client to the data source.
func (d *PasswordsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			"Expected *api.Client, got: %T. Please report this issue to the provider developers.",
		)
		return
	}

	d.client = client
}

// Metadata returns the data source type name.
func (d *PasswordsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_passwords"
}

// Schema defines the schema for the data source.
func (d *PasswordsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"passwords": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of password resources",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "The unique identifier of the password resource",
						},
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The name of the password resource",
						},
						"description": schema.StringAttribute{
							Computed:    true,
							Description: "The description of the password resource",
						},
						"username": schema.StringAttribute{
							Computed:    true,
							Description: "The username for the password resource",
						},
						"uri": schema.StringAttribute{
							Computed:    true,
							Description: "The URI for the password resource",
						},
						"folder_parent": schema.StringAttribute{
							Computed:    true,
							Description: "The name of the parent folder",
						},
					},
				},
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *PasswordsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state PasswordsDataSourceModel

	// Get all resources from Passbolt
	resources, err := d.client.GetResources(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading passwords",
			"Could not read passwords, unexpected error: "+err.Error(),
		)
		return
	}

	// Get all folders for parent folder mapping
	folders, err := d.client.GetFolders(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading folders",
			"Could not read folders, unexpected error: "+err.Error(),
		)
		return
	}

	// Create a map of folder IDs to names
	folderMap := make(map[string]string)
	for _, folder := range folders {
		folderMap[folder.ID] = folder.Name
	}

	// Convert resources to our model
	passwords := make([]PasswordModel, 0, len(resources))
	for _, resource := range resources {
		password := PasswordModel{
			ID:          types.StringValue(resource.ID),
			Name:        types.StringValue(resource.Name),
			Description: types.StringValue(resource.Description),
			Username:    types.StringValue(resource.Username),
			URI:         types.StringValue(resource.URI),
		}

		// Set folder parent if available
		if resource.FolderParentID != "" {
			if folderName, exists := folderMap[resource.FolderParentID]; exists {
				password.FolderParent = types.StringValue(folderName)
			}
		}

		passwords = append(passwords, password)
	}

	state.Passwords = passwords

	// Set state
	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
