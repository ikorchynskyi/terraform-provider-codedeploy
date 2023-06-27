// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/codedeploy"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"region": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("AWS_REGION", nil),
				Description: "AWS region",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"codedeploy_deployment": resourceDeployment(),
		},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
	region := d.Get("region").(string)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(region),
		config.WithRetryMode(aws.RetryModeAdaptive),
	)
	if err != nil {
		return nil, diag.FromErr(err)
	}

	return codedeploy.NewFromConfig(cfg), diags
}
