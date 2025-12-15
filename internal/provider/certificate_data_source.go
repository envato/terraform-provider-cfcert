package provider

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	tfTypes "github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &CertificateDataSource{}
var _ datasource.DataSourceWithConfigure = &CertificateDataSource{}

type CertificateDataSource struct {
	clients *ProviderClients
}

type CertificateDataSourceModel struct {
	DomainName     tfTypes.String `tfsdk:"domain_name"`
	CertificateArn tfTypes.String `tfsdk:"certificate_arn"`
	ID             tfTypes.String `tfsdk:"id"`
}

func NewCertificateDataSource() datasource.DataSource {
	return &CertificateDataSource{}
}

func (d *CertificateDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_origin_certificate"
}

func (d *CertificateDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing Cloudflare Origin Certificate in AWS ACM by domain name.",
		Attributes: map[string]schema.Attribute{
			"domain_name": schema.StringAttribute{
				Description: "The domain name to search for.",
				Required:    true,
			},
			"certificate_arn": schema.StringAttribute{
				Description: "The ARN of the ACM certificate, if found.",
				Computed:    true,
			},
			"id": schema.StringAttribute{
				Description: "Data source identifier.",
				Computed:    true,
			},
		},
	}
}

func (d *CertificateDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	clients, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData),
		)
		return
	}
	d.clients = clients
}

func (d *CertificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CertificateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainName := data.DomainName.ValueString()

	arn, err := d.findExistingCertificate(ctx, domainName)
	if err != nil {
		resp.Diagnostics.AddError("Failed to search for certificates", err.Error())
		return
	}

	if arn == "" {
		resp.Diagnostics.AddError(
			"Certificate Not Found",
			fmt.Sprintf("No issued EC_prime256v1 certificate found for domain: %s", domainName),
		)
		return
	}

	data.CertificateArn = tfTypes.StringValue(arn)
	data.ID = tfTypes.StringValue(arn)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *CertificateDataSource) findExistingCertificate(ctx context.Context, domainName string) (string, error) {
	paginator := acm.NewListCertificatesPaginator(d.clients.ACMClient, &acm.ListCertificatesInput{
		CertificateStatuses: []types.CertificateStatus{types.CertificateStatusIssued},
		Includes: &types.Filters{
			KeyTypes: []types.KeyAlgorithm{types.KeyAlgorithmEcPrime256v1},
		},
		SortBy:    types.SortByCreatedAt,
		SortOrder: types.SortOrderDescending,
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return "", err
		}
		for _, cert := range page.CertificateSummaryList {
			if aws.ToString(cert.DomainName) == domainName {
				return aws.ToString(cert.CertificateArn), nil
			}
		}
	}
	return "", nil
}
