package provider

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tfTypes "github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &CertificateResource{}
var _ resource.ResourceWithConfigure = &CertificateResource{}

type CertificateResource struct {
	clients *ProviderClients
}

type CertificateResourceModel struct {
	DomainName     tfTypes.String `tfsdk:"domain_name"`
	CertificateArn tfTypes.String `tfsdk:"certificate_arn"`
	ID             tfTypes.String `tfsdk:"id"`
}

func NewCertificateResource() resource.Resource {
	return &CertificateResource{}
}

func (r *CertificateResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_origin_certificate"
}

func (r *CertificateResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Cloudflare Origin Certificate imported into AWS ACM.",
		Attributes: map[string]schema.Attribute{
			"domain_name": schema.StringAttribute{
				Description: "The domain name for the certificate.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"certificate_arn": schema.StringAttribute{
				Description: "The ARN of the ACM certificate.",
				Computed:    true,
			},
			"id": schema.StringAttribute{
				Description: "Resource identifier (same as certificate_arn).",
				Computed:    true,
			},
		},
	}
}

func (r *CertificateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	clients, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData),
		)
		return
	}
	r.clients = clients
}

func (r *CertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainName := data.DomainName.ValueString()

	existingArn, err := r.findExistingCertificate(ctx, domainName)
	if err != nil {
		resp.Diagnostics.AddError("Failed to check existing certificates", err.Error())
		return
	}

	if existingArn != "" {
		data.CertificateArn = tfTypes.StringValue(existingArn)
		data.ID = tfTypes.StringValue(existingArn)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		resp.Diagnostics.AddError("Failed to generate private key", err.Error())
		return
	}

	csrTemplate := x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domainName},
		DNSNames: []string{domainName},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, privateKey)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create CSR", err.Error())
		return
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	certPEM, err := r.requestCloudflareOriginCert(domainName, string(csrPEM))
	if err != nil {
		resp.Diagnostics.AddError("Failed to request Cloudflare Origin Certificate", err.Error())
		return
	}

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		resp.Diagnostics.AddError("Failed to marshal private key", err.Error())
		return
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	importOutput, err := r.clients.ACMClient.ImportCertificate(ctx, &acm.ImportCertificateInput{
		Certificate: []byte(certPEM),
		PrivateKey:  keyPEM,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to import certificate to ACM", err.Error())
		return
	}

	arn := aws.ToString(importOutput.CertificateArn)
	data.CertificateArn = tfTypes.StringValue(arn)
	data.ID = tfTypes.StringValue(arn)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	arn := data.CertificateArn.ValueString()
	if arn == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	_, err := r.clients.ACMClient.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
		CertificateArn: aws.String(arn),
	})
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// domain_name forces replacement, so Update is a no-op
	var data CertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	arn := data.CertificateArn.ValueString()
	if arn == "" {
		return
	}

	_, err := r.clients.ACMClient.DeleteCertificate(ctx, &acm.DeleteCertificateInput{
		CertificateArn: aws.String(arn),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete certificate", err.Error())
		return
	}
}

func (r *CertificateResource) findExistingCertificate(ctx context.Context, domainName string) (string, error) {
	paginator := acm.NewListCertificatesPaginator(r.clients.ACMClient, &acm.ListCertificatesInput{
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

type cloudflareOriginCertRequest struct {
	CSR               string   `json:"csr"`
	Hostnames         []string `json:"hostnames"`
	RequestType       string   `json:"request_type"`
	RequestedValidity int      `json:"requested_validity"`
}

type cloudflareOriginCertResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Certificate string `json:"certificate"`
	} `json:"result"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (r *CertificateResource) requestCloudflareOriginCert(domainName, csrPEM string) (string, error) {
	reqBody := cloudflareOriginCertRequest{
		CSR:               csrPEM,
		Hostnames:         []string{domainName},
		RequestType:       "origin-ecc",
		RequestedValidity: 5475,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/certificates", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+r.clients.CloudflareAPIToken)

	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var cfResp cloudflareOriginCertResponse
	if err := json.Unmarshal(body, &cfResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !cfResp.Success {
		errMsg := "unknown error"
		if len(cfResp.Errors) > 0 {
			errMsg = cfResp.Errors[0].Message
		}
		return "", fmt.Errorf("cloudflare API error: %s", errMsg)
	}

	return cfResp.Result.Certificate, nil
}
