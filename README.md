# Cloudflare Origin Certificate Terraform Provider

A Terraform provider for managing Cloudflare Origin Certificates imported into AWS ACM.

## Overview

This provider automates the process of:
1. Generating an EC P-256 private key
2. Creating a CSR for a domain
3. Requesting a Cloudflare Origin Certificate via their API
4. Importing the certificate into AWS ACM

If an existing certificate for the domain already exists in ACM, it will be reused instead of creating a new one.

## Requirements

- Go 1.21+ (for building)
- Terraform 1.0+
- AWS credentials configured
- Cloudflare API token with Origin CA permissions

## Building

```bash
cd certificate-provider
go mod tidy
go build -o terraform-provider-cfcert
```

## Installation

For local development, add to your `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "envato/cfcert" = "/path/to/certificate-provider"
  }
  direct {}
}
```

## Usage

### Provider Configuration

```hcl
provider "cfcert" {
  region             = "ap-southeast-2"     # Optional, defaults to AWS_REGION
  cloudflare_api_token = "your-api-token"   # Optional, defaults to CLOUDFLARE_API_TOKEN
}
```

### Resource: `cfcert_origin_certificate`

Creates or imports a Cloudflare Origin Certificate into AWS ACM.

```hcl
resource "cfcert_origin_certificate" "example" {
  domain_name = "example.com"
}

output "certificate_arn" {
  value = cfcert_origin_certificate.example.certificate_arn
}
```

#### Arguments

- `domain_name` - (Required) The domain name for the certificate. Changing this forces a new resource.

#### Attributes

- `certificate_arn` - The ARN of the ACM certificate.
- `id` - Same as `certificate_arn`.

### Data Source: `cfcert_origin_certificate`

Look up an existing certificate by domain name.

```hcl
data "cfcert_origin_certificate" "example" {
  domain_name = "example.com"
}

output "certificate_arn" {
  value = data.cfcert_origin_certificate.example.certificate_arn
}
```

#### Arguments

- `domain_name` - (Required) The domain name to search for.

#### Attributes

- `certificate_arn` - The ARN of the ACM certificate.
- `id` - Same as `certificate_arn`.

## Environment Variables

- `AWS_REGION` - AWS region (can be overridden by provider config)
- `CLOUDFLARE_API_TOKEN` - Cloudflare API token (can be overridden by provider config)
- Standard AWS credential environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, etc.)

## Notes

- The resource will reuse an existing certificate if one with the same domain name already exists in ACM (with `EC_prime256v1` key type and ISSUED status)
- Certificates are requested with a 15-year (5475 days) validity period from Cloudflare
- Deleting the resource will delete the certificate from ACM
