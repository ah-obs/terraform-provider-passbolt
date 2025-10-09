package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/passbolt/go-passbolt/api"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ provider.Provider = &PassboltProvider{}
)

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &PassboltProvider{
			version: version,
		}
	}
}

// PassboltProvider is the provider implementation.
type PassboltProvider struct {
	version string
}

// PassboltProviderModel describes the provider data model.
type PassboltProviderModel struct {
	BaseURL    types.String `tfsdk:"base_url"`
	PrivateKey types.String `tfsdk:"private_key"`
	Passphrase types.String `tfsdk:"passphrase"`
}

// Metadata returns the provider type name.
func (p *PassboltProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "passbolt"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *PassboltProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"base_url": schema.StringAttribute{
				Required:    true,
				Description: "The base URL of the Passbolt instance (e.g., https://passbolt.example.com)",
			},
			"private_key": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "The private key for Passbolt authentication",
			},
			"passphrase": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "The passphrase for the private key",
			},
		},
	}
}

// Configure prepares a Passbolt API client for data sources and resources.
func (p *PassboltProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config PassboltProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate provider configuration
	if config.BaseURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("base_url"),
			"Unknown Passbolt Base URL",
			"The provider cannot create the Passbolt API client as there is an unknown configuration value for the Passbolt base URL. "+
				"Either target apply the source of the value first, or set the value statically in the configuration.",
		)
	}

	if config.PrivateKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("private_key"),
			"Unknown Passbolt Private Key",
			"The provider cannot create the Passbolt API client as there is an unknown configuration value for the Passbolt private key. "+
				"Either target apply the source of the value first, or set the value statically in the configuration.",
		)
	}

	if config.Passphrase.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("passphrase"),
			"Unknown Passbolt Passphrase",
			"The provider cannot create the Passbolt API client as there is an unknown configuration value for the Passbolt passphrase. "+
				"Either target apply the source of the value first, or set the value statically in the configuration.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override with Terraform configuration value if set.
	baseURL := os.Getenv("PASSBOLT_BASE_URL")
	privateKey := os.Getenv("PASSBOLT_PRIVATE_KEY")
	passphrase := os.Getenv("PASSBOLT_PASSPHRASE")

	if !config.BaseURL.IsNull() {
		baseURL = config.BaseURL.ValueString()
	}

	if !config.PrivateKey.IsNull() {
		privateKey = config.PrivateKey.ValueString()
	}

	if !config.Passphrase.IsNull() {
		passphrase = config.Passphrase.ValueString()
	}

	// If any of the expected configurations are missing, return errors with provider-specific guidance.
	if baseURL == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("base_url"),
			"Missing Passbolt Base URL",
			"The provider cannot create the Passbolt API client as there is a missing or empty value for the Passbolt base URL. "+
				"Set the base_url value in the configuration or use the PASSBOLT_BASE_URL environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if privateKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("private_key"),
			"Missing Passbolt Private Key",
			"The provider cannot create the Passbolt API client as there is a missing or empty value for the Passbolt private key. "+
				"Set the private_key value in the configuration or use the PASSBOLT_PRIVATE_KEY environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if passphrase == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("passphrase"),
			"Missing Passbolt Passphrase",
			"The provider cannot create the Passbolt API client as there is a missing or empty value for the Passbolt passphrase. "+
				"Set the passphrase value in the configuration or use the PASSBOLT_PASSPHRASE environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Create the Passbolt API client
	client, err := api.NewClient(nil, "", baseURL, privateKey, passphrase)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Passbolt API client",
			fmt.Sprintf("Cannot create the Passbolt API client: %s", err.Error()),
		)
		return
	}

	// Login to Passbolt
	err = client.Login(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to login to Passbolt",
			fmt.Sprintf("Cannot login to Passbolt: %s", err.Error()),
		)
		return
	}

	// Make the client available during DataSource and Resource type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

// DataSources defines the data sources implemented in the provider.
func (p *PassboltProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPasswordsDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *PassboltProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPasswordResource,
		NewFolderResource,
	}
}
