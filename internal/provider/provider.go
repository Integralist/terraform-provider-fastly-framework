// NOTE: Refer to https://developer.hashicorp.com/terraform/plugin/framework

package provider

import (
	"context"

	"github.com/fastly/fastly-go/fastly"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/datasources"
	"github.com/integralist/terraform-provider-fastly-framework/internal/provider/resources/servicevcl"
)

// Ensure FastlyProvider satisfies various provider interfaces.
var _ provider.Provider = &FastlyProvider{}

// FastlyProvider defines the provider implementation.
type FastlyProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// FastlyProviderModel describes the provider data model.
type FastlyProviderModel struct{}

func (p *FastlyProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "fastly"
	resp.Version = p.version
}

func (p *FastlyProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			// N/A
		},
	}
}

func (p *FastlyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data FastlyProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Client configuration for data sources and resources
	cfg := fastly.NewConfiguration()
	client := fastly.NewAPIClient(cfg)

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *FastlyProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		servicevcl.NewResource(),
	}
}

func (p *FastlyProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewExample,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &FastlyProvider{
			version: version,
		}
	}
}
