package provider

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &CertificateProvider{}

type CertificateProvider struct {
	version string
}

type CertificateProviderModel struct {
	Region                    types.String `tfsdk:"region"`
	CloudflareAPIToken        types.String `tfsdk:"cloudflare_api_token"`
	CloudflareServiceAPIToken types.String `tfsdk:"cloudflare_service_api_token"`
}

type ProviderClients struct {
	ACMClient                 *acm.Client
	CloudflareAPIToken        string
	CloudflareServiceAPIToken string
	Region                    string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CertificateProvider{
			version: version,
		}
	}
}

func (p *CertificateProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cfcert"
	resp.Version = p.version
}

func (p *CertificateProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provider for managing Cloudflare Origin Certificates imported into AWS ACM.",
		Attributes: map[string]schema.Attribute{
			"region": schema.StringAttribute{
				Description: "AWS region. Can also be set via AWS_REGION environment variable.",
				Optional:    true,
			},
			"cloudflare_api_token": schema.StringAttribute{
				Description: "Cloudflare API token with Origin CA permissions. Can also be set via CLOUDFLARE_API_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"cloudflare_service_api_token": schema.StringAttribute{
				Description: "Cloudflare Service API token with Origin CA permissions. Can also be set via CLOUDFLARE_SERVICE_API_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (p *CertificateProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data CertificateProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := os.Getenv("AWS_REGION")
	if !data.Region.IsNull() && data.Region.ValueString() != "" {
		region = data.Region.ValueString()
	}

	cloudflareToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	if !data.CloudflareAPIToken.IsNull() && data.CloudflareAPIToken.ValueString() != "" {
		cloudflareToken = data.CloudflareAPIToken.ValueString()
	}

	cloudflareServiceToken := os.Getenv("CLOUDFLARE_SERVICE_API_TOKEN")
	if !data.CloudflareServiceAPIToken.IsNull() && data.CloudflareServiceAPIToken.ValueString() != "" {
		cloudflareServiceToken = data.CloudflareServiceAPIToken.ValueString()
	}

	if region == "" {
		resp.Diagnostics.AddError(
			"Missing AWS Region",
			"AWS region must be set via the region attribute or AWS_REGION environment variable.",
		)
	}

	if cloudflareToken == "" && cloudflareServiceToken == "" {
		resp.Diagnostics.AddError(
			"Missing Cloudflare API or Service Token",
			"Cloudflare API or Service token must be set via the cloudflare_api_token or cloudflare_service_api_token attributes or CLOUDFLARE_API_TOKEN or CLOUDFLARE_SERVICE_API_TOKEN environment variables.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create AWS Config",
			"An error occurred while creating the AWS configuration: "+err.Error(),
		)
		return
	}

	clients := &ProviderClients{
		ACMClient:                 acm.NewFromConfig(cfg),
		CloudflareAPIToken:        cloudflareToken,
		CloudflareServiceAPIToken: cloudflareServiceToken,
		Region:                    region,
	}

	resp.DataSourceData = clients
	resp.ResourceData = clients
}

func (p *CertificateProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewCertificateResource,
	}
}

func (p *CertificateProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewCertificateDataSource,
	}
}
